package notify

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/arch-err/calemdar/internal/actions"
	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/store"
)

// SchedulerConfig is the resolved-and-bounded knob set the scheduler runs on.
type SchedulerConfig struct {
	// TickInterval is how often the loop wakes. 1m is the default; the
	// store query and dedupe are cheap enough that going lower buys us
	// nothing while raising load.
	TickInterval time.Duration
	// MaxLead caps the longest lead we will scan for. Events with a lead
	// longer than MaxLead are rejected at validation time, so this is a
	// belt-and-suspenders safety value, not a configurable feature.
	MaxLead time.Duration
	// Calendars filters which calendars the scheduler considers. Empty
	// means all configured calendars.
	Calendars []string
	// MaxConcurrentSpawns caps action-runner parallelism so a flurry of
	// simultaneous fires can't fork-bomb the daemon.
	MaxConcurrentSpawns int
	// PruneOlderThan controls how long fired-records are retained. The
	// nightly loop calls PruneFired with `now - PruneOlderThan`.
	PruneOlderThan time.Duration
}

// DefaultSchedulerConfig returns the v1 defaults.
func DefaultSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		TickInterval:        time.Minute,
		MaxLead:             23 * time.Hour,
		MaxConcurrentSpawns: 4,
		PruneOlderThan:      14 * 24 * time.Hour,
	}
}

// Scheduler is the per-event notif tick loop.
type Scheduler struct {
	cfg     SchedulerConfig
	store   *store.Store
	actions *actions.Runner

	// startedAt is captured once at Run-time so the missed-notif window
	// stays stable; using time.Now() inside tick would let a paused
	// daemon "catch up" on a long history.
	startedAt time.Time
	// lastTick is the previous tick's wall time. The scheduler fires
	// any rule whose fire_at lands in (lastTick, now]. Initialised to
	// startedAt - tickInterval*2 so a fresh process picks up rules in
	// the very recent past but not anything older.
	lastTick time.Time

	mu sync.Mutex
}

// NewScheduler returns a configured scheduler. r may be nil if no
// actions runner is wired (then any rule with `action:` is logged and
// skipped).
func NewScheduler(s *store.Store, r *actions.Runner, cfg SchedulerConfig) *Scheduler {
	if cfg.TickInterval <= 0 {
		cfg.TickInterval = time.Minute
	}
	if cfg.MaxLead <= 0 {
		cfg.MaxLead = 23 * time.Hour
	}
	if cfg.MaxConcurrentSpawns <= 0 {
		cfg.MaxConcurrentSpawns = 4
	}
	if cfg.PruneOlderThan <= 0 {
		cfg.PruneOlderThan = 14 * 24 * time.Hour
	}
	return &Scheduler{cfg: cfg, store: s, actions: r}
}

// Run blocks until ctx is cancelled. Ticks every cfg.TickInterval.
//
// Restart-safety: lastTick is initialised to now - 2*tickInterval, so a
// freshly-started daemon picks up notifs whose fire_at fell in the past
// 2 minutes but does NOT replay a longer history (which would be spammy
// when the laptop wakes from a long sleep).
func (sc *Scheduler) Run(ctx context.Context) {
	now := time.Now()
	sc.startedAt = now
	sc.lastTick = now.Add(-2 * sc.cfg.TickInterval)

	// Tick immediately so the first-run experience isn't "wait a minute".
	sc.tick(ctx, now)

	t := time.NewTicker(sc.cfg.TickInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case n := <-t.C:
			sc.tick(ctx, n)
		}
	}
}

// tick runs one iteration. Queries the store for events with notify
// rules in the lookahead window, computes per-rule fire_at, fires the
// rules whose fire_at is in (lastTick, now], records dedupe.
func (sc *Scheduler) tick(ctx context.Context, now time.Time) {
	sc.mu.Lock()
	last := sc.lastTick
	sc.mu.Unlock()

	// Lookahead window: events whose start lands in [last, now+maxLead].
	// We include the (last, now] portion of the past so a 0-lead rule
	// whose fire-at IS the start time still surfaces — without this,
	// the start has already drifted into "the past" relative to `now`
	// by the time we tick, and the event is filtered out before we
	// ever reach the per-rule fire check.
	queryFrom := last
	if queryFrom.After(now) {
		queryFrom = now
	}
	rows, err := sc.store.ListUpcomingWithNotify(queryFrom, now.Add(sc.cfg.MaxLead), sc.cfg.Calendars)
	if err != nil {
		log.Printf("notify: list upcoming: %v", err)
		return
	}

	loc := model.Location()
	for _, row := range rows {
		startTs, ok := parseEventStart(row.Event, loc)
		if !ok {
			continue
		}
		for i, rule := range row.Event.Notify {
			lead, err := rule.LeadDuration()
			if err != nil {
				log.Printf("notify: %s: bad lead %q: %v", row.Event.Path, rule.Lead, err)
				continue
			}
			fireAt := startTs.Add(-lead)
			// Fire if (last, now]
			if !fireAt.After(last) || fireAt.After(now) {
				continue
			}
			// Persistent dedupe: skip if recorded.
			already, err := sc.store.IsFired(row.Event.Path, i, fireAt)
			if err != nil {
				log.Printf("notify: dedupe lookup %s[%d]: %v", row.Event.Path, i, err)
				continue
			}
			if already {
				continue
			}
			sc.fire(ctx, row, rule, i, fireAt, now)
		}
	}

	sc.mu.Lock()
	sc.lastTick = now
	sc.mu.Unlock()
}

// fire delivers a single rule: dispatches to each `via:` backend and, if
// configured, runs the named action. Records the fire in notify_fired
// before returning so a crash mid-fire still prevents replay.
func (sc *Scheduler) fire(ctx context.Context, row store.UpcomingRow, rule model.NotifyEntry, idx int, fireAt, now time.Time) {
	// Record first — better to suppress a future replay than to double-fire
	// after a crash. Failures here are logged but not blocking: a rare
	// double-fire on a single restart is preferable to silent loss.
	if err := sc.store.RecordFired(row.Event.Path, idx, fireAt, now); err != nil {
		log.Printf("notify: record fired %s[%d]: %v", row.Event.Path, idx, err)
	}

	msg := buildNotification(row, rule)
	for _, via := range rule.Via {
		b, err := Get(via)
		if err != nil {
			log.Printf("notify: %s: %v", row.Event.Path, err)
			continue
		}
		if err := b.Send(ctx, msg); err != nil {
			log.Printf("notify: backend %s send %s: %v", via, row.Event.Path, err)
			continue
		}
		log.Printf("notify: %s → %s (lead=%s)", row.Event.Title, via, rule.Lead)
	}

	if rule.Action != "" {
		if sc.actions == nil {
			log.Printf("notify: %s: action %q skipped — no actions runner configured", row.Event.Path, rule.Action)
			return
		}
		env := buildActionEnv(row, rule)
		// Dispatch async — the action may shell out to a slow GUI launcher
		// (gtk-launch waits for the spawned program to start, not exit, but
		// some apps still take 10-30s to come up). Blocking the scheduler
		// tick on that risks missing the next 1m tick. The runner already
		// caps concurrency via its own semaphore.
		title := row.Event.Title
		path := row.Event.Path
		action := rule.Action
		go func() {
			if err := sc.actions.Run(ctx, action, env); err != nil {
				log.Printf("notify: action %q for %s: %v", action, path, err)
				return
			}
			log.Printf("notify: %s → action %q", title, action)
		}()
	}
}

// buildNotification renders the body, title, and tags shared across
// backends for a single fire.
func buildNotification(row store.UpcomingRow, rule model.NotifyEntry) Notification {
	body := formatBody(row.Event, rule.Lead)
	tags := []string{"calendar"}
	if row.Calendar != "" {
		tags = append(tags, row.Calendar)
	}
	return Notification{
		Title:      "calemdar: " + row.Event.Title,
		Body:       body,
		Tags:       tags,
		EventTitle: row.Event.Title,
		EventDate:  row.Event.Date,
		EventStart: row.Event.StartTime,
		EventEnd:   row.Event.EndTime,
		EventPath:  row.Event.Path,
		Calendar:   row.Calendar,
		Lead:       rule.Lead,
	}
}

// buildActionEnv returns the env map handed to actions.Runner. Keys are
// uppercase CALEMDAR_* so scripts can grep for them in one go.
func buildActionEnv(row store.UpcomingRow, rule model.NotifyEntry) map[string]string {
	return map[string]string{
		"CALEMDAR_TITLE":    row.Event.Title,
		"CALEMDAR_DATE":     row.Event.Date,
		"CALEMDAR_START":    row.Event.StartTime,
		"CALEMDAR_END":      row.Event.EndTime,
		"CALEMDAR_PATH":     row.Event.Path,
		"CALEMDAR_CALENDAR": row.Calendar,
		"CALEMDAR_LEAD":     rule.Lead,
	}
}

// formatBody renders the human-readable line. Keep it short — phones
// truncate.
func formatBody(e *model.Event, lead string) string {
	var sb strings.Builder
	sb.WriteString(e.Title)
	if lead == "0" || lead == "" {
		sb.WriteString(" — now")
	} else {
		sb.WriteString(" — in ")
		sb.WriteString(lead)
	}
	if e.StartTime != "" {
		sb.WriteString(" @ ")
		sb.WriteString(e.StartTime)
		if e.EndTime != "" {
			sb.WriteString("–")
			sb.WriteString(e.EndTime)
		}
	}
	return sb.String()
}

// parseEventStart turns Date+StartTime in the configured tz into a
// concrete time.Time. Returns (zero, false) on malformed input.
func parseEventStart(e *model.Event, loc *time.Location) (time.Time, bool) {
	if e.StartTime == "" {
		return time.Time{}, false
	}
	ts, err := time.ParseInLocation("2006-01-02 15:04", fmt.Sprintf("%s %s", e.Date, e.StartTime), loc)
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}
