// Package notify sends upcoming-event push notifications via ntfy.
//
// It ticks every minute and queries the store for events whose local start
// time falls inside a small window centered on `now + lead`, for each
// configured lead-minute value. Matching events trigger a POST to the
// configured ntfy URL + topic. A dedupe set keyed by <path>|<lead> prevents
// the same event firing twice for the same lead value.
package notify

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/arch-err/calemdar/internal/config"
	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/store"
)

// tickInterval is how often the notifier polls the store. Notifications fire
// with resolution = tickInterval ± windowHalf. One minute is fine for human
// lead-times measured in minutes; we trade precision for cheapness.
const tickInterval = 60 * time.Second

// windowHalf is the half-width of the "about to fire" match window. Picking
// this slightly larger than half tickInterval keeps events from slipping
// between ticks when the daemon tick drifts.
const windowHalf = 30 * time.Second

// Notifier holds the runtime state for the tick loop: a handle to the store,
// a copy of the notifications config (never a pointer to config.Active — we
// don't want a hot reload to change our behavior mid-tick), and an in-memory
// dedupe set.
type Notifier struct {
	store  *store.Store
	cfg    config.Notifications
	client *http.Client
	// seen maps "<path>|<lead>" → occurrence date (YYYY-MM-DD). The date is
	// kept so GC can drop entries older than one day.
	seen map[string]string
}

// New constructs a Notifier. The cfg is copied (value receiver, so config
// changes after this call won't leak in).
func New(s *store.Store, cfg config.Notifications) *Notifier {
	return &Notifier{
		store:  s,
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
		seen:   make(map[string]string),
	}
}

// Run blocks until ctx is cancelled, ticking every tickInterval.
func (n *Notifier) Run(ctx context.Context) {
	// Fire once immediately so the first-run experience isn't "wait a minute".
	n.tick(ctx, time.Now())

	t := time.NewTicker(tickInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			n.tick(ctx, now)
		}
	}
}

// tick scans each configured lead-time and fires a push for any matching,
// not-yet-seen occurrence. GCs the dedupe set as a side effect.
func (n *Notifier) tick(ctx context.Context, now time.Time) {
	n.gc(now)

	for _, lead := range n.cfg.LeadMinutes {
		target := now.Add(time.Duration(lead) * time.Minute)
		from := target.Add(-windowHalf)
		to := target.Add(windowHalf)

		events, err := n.store.ListUpcoming(from, to, n.cfg.Calendars)
		if err != nil {
			log.Printf("notify: list upcoming (lead=%dm): %v", lead, err)
			continue
		}

		for _, e := range events {
			key := dedupeKey(e.Path, lead)
			if _, ok := n.seen[key]; ok {
				continue
			}
			if err := n.send(ctx, e, lead); err != nil {
				log.Printf("notify: send %s (lead=%dm): %v", e.Path, lead, err)
				continue
			}
			n.seen[key] = e.Date
			log.Printf("notify: pushed %q (lead=%dm) → %s/%s", e.Title, lead, n.cfg.NtfyURL, n.cfg.NtfyTopic)
		}
	}
}

// send POSTs a single event to ntfy. Body is the human message; title, tags,
// and priority ride on headers. Ntfy parses these without auth — for private
// topics the user bakes auth into the URL or reverse-proxies.
func (n *Notifier) send(ctx context.Context, e *model.Event, lead int) error {
	body := formatBody(e, lead)

	// Calendar is carried on the event via its parent folder; we stash it on
	// the occurrence row and reuse that here. ListUpcoming scans `calendar`
	// but doesn't surface it on the Event — so we fall back to the best
	// available signal: the full path. For tagging, leave the calendar blank
	// if not set explicitly by the caller chain.
	calendar := extractCalendar(e.Path)

	url := strings.TrimRight(n.cfg.NtfyURL, "/") + "/" + n.cfg.NtfyTopic
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Title", "calemdar: upcoming")
	req.Header.Set("Tags", "calendar,"+calendar)
	req.Header.Set("Content-Type", "text/plain")

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Drain so the connection can be reused.
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy returned %s", resp.Status)
	}
	return nil
}

// SendTest pushes a single canned message. Used by `calemdar notify test`.
func (n *Notifier) SendTest(ctx context.Context) error {
	url := strings.TrimRight(n.cfg.NtfyURL, "/") + "/" + n.cfg.NtfyTopic
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url,
		strings.NewReader("calemdar notify test — if you see this, the wiring is good."))
	if err != nil {
		return err
	}
	req.Header.Set("Title", "calemdar: test")
	req.Header.Set("Tags", "calendar,test")
	req.Header.Set("Content-Type", "text/plain")

	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy returned %s", resp.Status)
	}
	return nil
}

// gc drops dedupe entries whose occurrence date is more than one day behind
// `now`. Keeps the map from growing without bound across long-running days.
func (n *Notifier) gc(now time.Time) {
	cutoff := now.AddDate(0, 0, -1).Format("2006-01-02")
	for k, d := range n.seen {
		if d < cutoff {
			delete(n.seen, k)
		}
	}
}

func dedupeKey(path string, lead int) string {
	return fmt.Sprintf("%s|%d", path, lead)
}

// formatBody renders the human-readable notification body. Keep it short —
// ntfy shows body in the push, and phones truncate fast.
func formatBody(e *model.Event, lead int) string {
	var sb strings.Builder
	sb.WriteString(e.Title)
	sb.WriteString(" — in ")
	sb.WriteString(fmt.Sprintf("%dm", lead))
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

// extractCalendar returns the calendar folder name from an events/<cal>/…
// path. Returns "" when the pattern doesn't match — caller tolerates that.
func extractCalendar(path string) string {
	// Normalise separators for lookup; we're just searching for a segment.
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == "events" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
