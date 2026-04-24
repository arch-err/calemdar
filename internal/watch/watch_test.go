package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arch-err/calemdar/internal/vault"
)

func setup(t *testing.T) (*vault.Vault, *Watcher) {
	t.Helper()
	root := t.TempDir()
	for _, sub := range []string{"recurring", "events/health/2026", "events/tech"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	v := &vault.Vault{Root: root}
	w, err := StartWithDebounce(v, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Stop() })
	return v, w
}

func drain(t *testing.T, w *Watcher, timeout time.Duration) []Event {
	t.Helper()
	var out []Event
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case ev, ok := <-w.Events():
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-timer.C:
			return out
		}
	}
}

func TestRecurringCreate(t *testing.T) {
	v, w := setup(t)
	path := filepath.Join(v.RecurringDir(), "workout.md")
	if err := os.WriteFile(path, []byte("---\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	evs := drain(t, w, 300*time.Millisecond)
	if len(evs) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(evs), evs)
	}
	if evs[0].Source != SourceRecurring || evs[0].Kind != Changed {
		t.Errorf("unexpected event: %+v", evs[0])
	}
}

func TestEventsCreateNested(t *testing.T) {
	v, w := setup(t)
	path := filepath.Join(v.EventsDir(), "health", "2026", "2026-05-01-w.md")
	if err := os.WriteFile(path, []byte("---\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	evs := drain(t, w, 300*time.Millisecond)
	if len(evs) != 1 {
		t.Fatalf("got %d events: %+v", len(evs), evs)
	}
	if evs[0].Source != SourceEvents || evs[0].Kind != Changed {
		t.Errorf("unexpected: %+v", evs[0])
	}
}

func TestSelfWriteSuppression(t *testing.T) {
	v, w := setup(t)
	path := filepath.Join(v.RecurringDir(), "silent.md")
	w.NotifySelfWrite(path)
	if err := os.WriteFile(path, []byte("---\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	evs := drain(t, w, 200*time.Millisecond)
	if len(evs) != 0 {
		t.Errorf("expected 0 events (suppressed), got %+v", evs)
	}
}

func TestDebounceCoalesce(t *testing.T) {
	v, w := setup(t)
	path := filepath.Join(v.RecurringDir(), "rapid.md")
	if err := os.WriteFile(path, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Burst of writes within the debounce window.
	for i := 0; i < 5; i++ {
		if err := os.WriteFile(path, []byte("b"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	evs := drain(t, w, 300*time.Millisecond)
	if len(evs) != 1 {
		t.Errorf("got %d events, want 1 (debounced): %+v", len(evs), evs)
	}
}

func TestDeleteEmitsDeleted(t *testing.T) {
	v, w := setup(t)
	path := filepath.Join(v.RecurringDir(), "gone.md")
	if err := os.WriteFile(path, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = drain(t, w, 200*time.Millisecond) // consume Changed
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	evs := drain(t, w, 300*time.Millisecond)
	if len(evs) != 1 || evs[0].Kind != Deleted {
		t.Errorf("expected Deleted, got %+v", evs)
	}
}

func TestIgnoresNonMarkdown(t *testing.T) {
	v, w := setup(t)
	path := filepath.Join(v.RecurringDir(), "junk.txt")
	if err := os.WriteFile(path, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	evs := drain(t, w, 200*time.Millisecond)
	if len(evs) != 0 {
		t.Errorf("expected 0 events for non-md, got %+v", evs)
	}
}

func TestNewSubdirWatched(t *testing.T) {
	v, w := setup(t)
	newDir := filepath.Join(v.EventsDir(), "life", "2026")
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Give the watcher a tick to pick up the new dir.
	time.Sleep(100 * time.Millisecond)

	path := filepath.Join(newDir, "thing.md")
	if err := os.WriteFile(path, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	evs := drain(t, w, 300*time.Millisecond)
	if len(evs) == 0 {
		t.Fatal("expected event in newly-created subdir")
	}
	if evs[len(evs)-1].Source != SourceEvents {
		t.Errorf("source = %v", evs[len(evs)-1].Source)
	}
}
