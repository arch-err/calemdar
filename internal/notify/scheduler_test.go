package notify

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/store"
	"github.com/arch-err/calemdar/internal/vault"
)

// stubBackend records every Send call. Tests register one to assert
// scheduler dispatch without going over a real network or libnotify.
type stubBackend struct {
	name string
	mu   sync.Mutex
	got  []Notification
}

func (s *stubBackend) Name() string { return s.name }
func (s *stubBackend) Send(_ context.Context, n Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.got = append(s.got, n)
	return nil
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	root := t.TempDir()
	v := &vault.Vault{Root: root}
	s, err := store.Open(v)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSchedulerFiresInsideWindowOnce(t *testing.T) {
	Reset()
	stub := &stubBackend{name: "stub"}
	Register(stub)

	s := openStore(t)

	// Event at 10:00 with a 5m lead; "now" of 09:55 should fire.
	loc := model.Location()
	when := time.Date(2026, 5, 4, 10, 0, 0, 0, loc)
	evt := &model.Event{
		Path:      "/v/events/work/2026-05-04-standup.md",
		Title:     "Standup",
		Date:      "2026-05-04",
		StartTime: "10:00",
		Notify:    []model.NotifyEntry{{Lead: "5m", Via: []string{"stub"}}},
	}
	if err := s.UpsertOccurrence(evt, "work"); err != nil {
		t.Fatal(err)
	}

	sc := NewScheduler(s, nil, SchedulerConfig{
		TickInterval:        time.Minute,
		MaxLead:             time.Hour,
		MaxConcurrentSpawns: 1,
	})
	// Pretend the daemon last ticked a minute before fire_at.
	sc.lastTick = when.Add(-6 * time.Minute)

	// First tick at 09:55 — fires.
	sc.tick(context.Background(), when.Add(-5*time.Minute))
	stub.mu.Lock()
	if len(stub.got) != 1 {
		t.Fatalf("got %d notifications, want 1", len(stub.got))
	}
	if stub.got[0].Title != "calemdar: Standup" {
		t.Errorf("title = %q", stub.got[0].Title)
	}
	stub.mu.Unlock()

	// Second tick at 09:56 — already fired (dedupe via store).
	sc.tick(context.Background(), when.Add(-4*time.Minute))
	stub.mu.Lock()
	if len(stub.got) != 1 {
		t.Errorf("dedupe broken: got %d, want 1", len(stub.got))
	}
	stub.mu.Unlock()
}

func TestSchedulerDoesNotReplayHistoryOnRestart(t *testing.T) {
	Reset()
	stub := &stubBackend{name: "stub"}
	Register(stub)

	s := openStore(t)
	loc := model.Location()
	// Event with start time well in the future (10:00) and a 5m lead.
	// fire_at = 09:55. "now" = 11:00 → fire_at is firmly in the past.
	evt := &model.Event{
		Path:      "/v/events/work/2026-05-04-standup.md",
		Title:     "Standup",
		Date:      "2026-05-04",
		StartTime: "10:00",
		Notify:    []model.NotifyEntry{{Lead: "5m", Via: []string{"stub"}}},
	}
	_ = s.UpsertOccurrence(evt, "work")

	sc := NewScheduler(s, nil, SchedulerConfig{
		TickInterval:        time.Minute,
		MaxLead:             time.Hour,
		MaxConcurrentSpawns: 1,
	})
	now := time.Date(2026, 5, 4, 11, 0, 0, 0, loc)
	// Run() would set lastTick to now - 2*tickInterval. Simulate that.
	sc.lastTick = now.Add(-2 * time.Minute)

	sc.tick(context.Background(), now)

	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.got) != 0 {
		t.Errorf("expected no replay of past notif, got %d", len(stub.got))
	}
}

func TestSchedulerHandlesMultipleNotifyRules(t *testing.T) {
	Reset()
	stub := &stubBackend{name: "stub"}
	Register(stub)

	s := openStore(t)
	loc := model.Location()
	when := time.Date(2026, 5, 4, 10, 0, 0, 0, loc)
	evt := &model.Event{
		Path:      "/v/events/work/2026-05-04-standup.md",
		Title:     "Standup",
		Date:      "2026-05-04",
		StartTime: "10:00",
		Notify: []model.NotifyEntry{
			{Lead: "5m", Via: []string{"stub"}},
			{Lead: "0", Via: []string{"stub"}},
		},
	}
	_ = s.UpsertOccurrence(evt, "work")

	sc := NewScheduler(s, nil, SchedulerConfig{
		TickInterval:        time.Minute,
		MaxLead:             time.Hour,
		MaxConcurrentSpawns: 1,
	})

	// Tick at 09:55 — fires the 5m lead only.
	sc.lastTick = when.Add(-6 * time.Minute)
	sc.tick(context.Background(), when.Add(-5*time.Minute))
	// Tick at 10:00 — fires the 0 lead.
	sc.tick(context.Background(), when)

	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.got) != 2 {
		t.Errorf("got %d, want 2", len(stub.got))
	}
}

// 0-lead rule whose start time falls between two ticks must still
// surface — before this test was added, the lookahead window started
// at `now` and a 0-lead start that drifted past `now` between the
// query and the fire check got filtered out by ListUpcomingWithNotify.
func TestSchedulerFiresZeroLeadEventThatStartedInLastTickWindow(t *testing.T) {
	Reset()
	stub := &stubBackend{name: "stub"}
	Register(stub)

	s := openStore(t)
	loc := model.Location()

	// Event start = 15:19:00. Tick at 15:19:32 with last_tick = 15:18:32
	// → fire-at (15:19:00 - 0) is in (15:18:32, 15:19:32], should fire.
	startsAt := time.Date(2026, 5, 4, 15, 19, 0, 0, loc)
	evt := &model.Event{
		Path:      "/v/events/work/2026-05-04-smoke.md",
		Title:     "smoke",
		Date:      "2026-05-04",
		StartTime: "15:19",
		Notify:    []model.NotifyEntry{{Lead: "0", Via: []string{"stub"}}},
	}
	if err := s.UpsertOccurrence(evt, "work"); err != nil {
		t.Fatal(err)
	}

	sc := NewScheduler(s, nil, SchedulerConfig{
		TickInterval:        time.Minute,
		MaxLead:             time.Hour,
		MaxConcurrentSpawns: 1,
	})
	sc.lastTick = startsAt.Add(-32 * time.Second).Add(-time.Minute) // 15:18:32 - 1m drift
	tickAt := startsAt.Add(32 * time.Second)                        // 15:19:32

	sc.tick(context.Background(), tickAt)

	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.got) != 1 {
		t.Fatalf("expected 1 fire, got %d", len(stub.got))
	}
}

func TestFormatBodyZeroLead(t *testing.T) {
	e := &model.Event{Title: "Ping", StartTime: "10:00", EndTime: "10:30"}
	got := formatBody(e, "0")
	if !contains(got, "Ping") || !contains(got, "now") {
		t.Errorf("body = %q", got)
	}
}

func TestFormatBodyWithLead(t *testing.T) {
	e := &model.Event{Title: "Ping", StartTime: "10:00", EndTime: "10:30"}
	got := formatBody(e, "5m")
	if !contains(got, "Ping") || !contains(got, "5m") || !contains(got, "10:00") {
		t.Errorf("body = %q", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
