package serve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arch-err/calemdar/internal/backup"
	"github.com/arch-err/calemdar/internal/config"
	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/store"
	"github.com/arch-err/calemdar/internal/vault"
	"github.com/arch-err/calemdar/internal/watch"
)

// setup creates a tempdir vault scaffold + open store. Returns both.
func setup(t *testing.T) (*vault.Vault, *store.Store) {
	t.Helper()
	root := t.TempDir()
	v := &vault.Vault{Root: root}
	for _, sub := range []string{"recurring", "events/health", "archive", ".calemdar"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	config.Active = config.Defaults()

	s, err := store.Open(v)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return v, s
}

// snapshotRoot writes a recurring root file and seeds the snapshot in
// sqlite. Mirrors what dispatchRecurring(Changed) would do, but without
// requiring a full reconcile.
func snapshotRoot(t *testing.T, v *vault.Vault, s *store.Store, slug string, content string) *model.Root {
	t.Helper()
	target := filepath.Join(v.RecurringDir(), slug+".md")
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := model.ParseRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertSeries(r); err != nil {
		t.Fatal(err)
	}
	return r
}

const sampleRoot = `---
id: 11111111-1111-7111-8111-111111111111
calendar: health
title: Workout
start-date: 2026-05-01
all-day: false
freq: weekly
interval: 1
byday: [mon]
start-time: "10:00"
end-time: "11:00"
---

body content
`

func TestExternalDeleteTriggersAutoRestore(t *testing.T) {
	v, s := setup(t)
	r := snapshotRoot(t, v, s, "workout", sampleRoot)

	// Confirm sqlite captured the snapshot.
	got, err := s.GetSeriesByPath(r.Path)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.RawSource == "" {
		t.Fatal("no snapshot in sqlite")
	}

	// Simulate an external delete.
	if err := os.Remove(r.Path); err != nil {
		t.Fatal(err)
	}

	// Hand the watcher Event to dispatch.
	ev := watch.Event{Kind: watch.Deleted, Source: watch.SourceRecurring, Path: r.Path}
	if err := dispatch(Options{Vault: v, Store: s}, ev); err != nil {
		t.Fatalf("dispatch returned error: %v", err)
	}

	// File restored.
	restored, err := os.ReadFile(r.Path)
	if err != nil {
		t.Fatalf("file not restored: %v", err)
	}
	if string(restored) != sampleRoot {
		t.Errorf("restored contents differ:\nwant: %q\ngot:  %q", sampleRoot, restored)
	}

	// Backup mirror exists.
	all, err := backup.List(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Slug != "workout" {
		t.Fatalf("expected exactly 1 backup for workout, got %+v", all)
	}
	bk, err := os.ReadFile(all[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(bk) != sampleRoot {
		t.Errorf("backup contents differ:\nwant: %q\ngot:  %q", sampleRoot, bk)
	}
}

func TestExternalDeleteWithoutSnapshotIsNoop(t *testing.T) {
	v, s := setup(t)
	// File on disk but no sqlite row at all.
	target := filepath.Join(v.RecurringDir(), "ghost.md")
	if err := os.WriteFile(target, []byte(sampleRoot), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}

	ev := watch.Event{Kind: watch.Deleted, Source: watch.SourceRecurring, Path: target}
	if err := dispatch(Options{Vault: v, Store: s}, ev); err != nil {
		t.Fatalf("dispatch returned error: %v", err)
	}

	// File still gone.
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("expected file to remain deleted, got err=%v", err)
	}
	// No backup written.
	all, _ := backup.List(v)
	if len(all) != 0 {
		t.Errorf("expected no backup, got %+v", all)
	}
}

// TestStickyDeleteAppendsExceptionToRoot exercises the sticky-delete
// path: a series-bound event file is removed, dispatch picks up the
// DELETE, and the root's `exceptions:` list grows to include the date.
// Future reconciles must skip the date.
func TestStickyDeleteAppendsExceptionToRoot(t *testing.T) {
	v, s := setup(t)
	r := snapshotRoot(t, v, s, "workout", sampleRoot)

	// Seed an expanded occurrence in the store + on disk.
	loc := model.Location()
	date := model.Today(loc).AddDate(0, 0, 14).Format("2006-01-02")
	eventPath := v.EventPath("health", date, "workout")
	if err := os.MkdirAll(filepath.Dir(eventPath), 0o755); err != nil {
		t.Fatal(err)
	}
	occContent := `---
title: Workout
date: "` + date + `"
startTime: "10:00"
endTime: "11:00"
allDay: false
type: single
series-id: 11111111-1111-7111-8111-111111111111
user-owned: false
---

body
`
	if err := os.WriteFile(eventPath, []byte(occContent), 0o644); err != nil {
		t.Fatal(err)
	}
	occ, err := model.ParseEvent(eventPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertOccurrence(occ, "health"); err != nil {
		t.Fatal(err)
	}

	// User deletes the file (e.g. obsidian on phone, syncthing pushes here).
	if err := os.Remove(eventPath); err != nil {
		t.Fatal(err)
	}

	ev := watch.Event{Kind: watch.Deleted, Source: watch.SourceEvents, Path: eventPath}
	if err := dispatch(Options{Vault: v, Store: s}, ev); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	// Store row dropped.
	got, err := s.GetOccurrenceByPath(eventPath)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected store row gone, got %+v", got)
	}

	// Root file now carries the exception.
	rerolled, err := model.ParseRoot(r.Path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ex := range rerolled.Exceptions {
		if ex == date {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("date %s not in exceptions: %v", date, rerolled.Exceptions)
	}
}

// TestStickyDeleteIdempotent runs the delete dispatch twice on the same
// path. Second run must not duplicate the date in exceptions.
func TestStickyDeleteIdempotent(t *testing.T) {
	v, s := setup(t)
	_ = snapshotRoot(t, v, s, "workout", sampleRoot)

	loc := model.Location()
	date := model.Today(loc).AddDate(0, 0, 21).Format("2006-01-02")
	eventPath := v.EventPath("health", date, "workout")
	if err := os.MkdirAll(filepath.Dir(eventPath), 0o755); err != nil {
		t.Fatal(err)
	}
	occContent := `---
title: Workout
date: "` + date + `"
startTime: "10:00"
endTime: "11:00"
allDay: false
type: single
series-id: 11111111-1111-7111-8111-111111111111
user-owned: false
---

body
`
	if err := os.WriteFile(eventPath, []byte(occContent), 0o644); err != nil {
		t.Fatal(err)
	}
	occ, _ := model.ParseEvent(eventPath)
	_ = s.UpsertOccurrence(occ, "health")

	if err := os.Remove(eventPath); err != nil {
		t.Fatal(err)
	}
	ev := watch.Event{Kind: watch.Deleted, Source: watch.SourceEvents, Path: eventPath}
	for i := 0; i < 3; i++ {
		_ = dispatch(Options{Vault: v, Store: s}, ev) // second/third are no-ops
	}

	rerolled, err := model.ParseRoot(filepath.Join(v.RecurringDir(), "workout.md"))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, ex := range rerolled.Exceptions {
		if ex == date {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 occurrence of %s in exceptions, got %d (full list: %v)",
			date, count, rerolled.Exceptions)
	}
}

// TestStickyDeleteOneOffNoExceptionWritten exercises a delete on an
// event with no series_id (true one-off). The store row drops, no root
// is touched.
func TestStickyDeleteOneOffNoExceptionWritten(t *testing.T) {
	v, s := setup(t)
	eventPath := filepath.Join(v.EventsDir(), "health", "2026-05-04-oneoff.md")
	if err := os.MkdirAll(filepath.Dir(eventPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(eventPath, []byte(`---
title: oneoff
date: "2026-05-04"
startTime: "10:00"
endTime: "11:00"
allDay: false
type: single
user-owned: true
---
`), 0o644); err != nil {
		t.Fatal(err)
	}
	occ, _ := model.ParseEvent(eventPath)
	if err := s.UpsertOccurrence(occ, "health"); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(eventPath); err != nil {
		t.Fatal(err)
	}
	ev := watch.Event{Kind: watch.Deleted, Source: watch.SourceEvents, Path: eventPath}
	if err := dispatch(Options{Vault: v, Store: s}, ev); err != nil {
		t.Fatal(err)
	}
	// Just confirm nothing exploded; the only side effect is the row drop.
	got, _ := s.GetOccurrenceByPath(eventPath)
	if got != nil {
		t.Errorf("expected store row dropped, got %+v", got)
	}
}

func TestRestoreSurfacesBodyVerbatim(t *testing.T) {
	// The whole file is the snapshot, including frontmatter quirks.
	v, s := setup(t)
	weird := strings.Replace(sampleRoot, "body content", "body with a # heading\n\n[[wikilink]]", 1)
	r := snapshotRoot(t, v, s, "workout", weird)

	if err := os.Remove(r.Path); err != nil {
		t.Fatal(err)
	}
	ev := watch.Event{Kind: watch.Deleted, Source: watch.SourceRecurring, Path: r.Path}
	if err := dispatch(Options{Vault: v, Store: s}, ev); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(r.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != weird {
		t.Errorf("restore not byte-exact:\nwant: %q\ngot:  %q", weird, got)
	}
}
