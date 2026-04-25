package reconcile

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arch-err/calemdar/internal/config"
	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/vault"
	"github.com/arch-err/calemdar/internal/writer"
)

// setup returns a tempdir-backed Vault with scaffolded subfolders, plus a
// reset config.Active with defaults. Call at the top of every test.
func setup(t *testing.T) *vault.Vault {
	t.Helper()
	root := t.TempDir()
	v := &vault.Vault{Root: root}
	for _, sub := range []string{"recurring", "events/health", "archive"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	config.Active = config.Defaults()
	return v
}

// dateStr renders today + days in YYYY-MM-DD.
func dateStr(days int) string {
	return time.Now().In(model.Location()).AddDate(0, 0, days).Format("2006-01-02")
}

// writeRaw drops a file with arbitrary Event frontmatter at the canonical
// calendar path. Uses writer so paths and formatting match production.
func writeRaw(t *testing.T, v *vault.Vault, e *model.Event, calendar string) {
	t.Helper()
	if e.Path == "" {
		e.Path = v.EventPath(calendar, e.Date, "test-series")
	}
	if e.Type == "" {
		e.Type = "single"
	}
	// Clear the SelfWriteNotifier so tests don't try to hit a nil watcher.
	writer.SelfWriteNotifier = nil
	if err := writer.WriteEvent(e); err != nil {
		t.Fatal(err)
	}
}

func baseRoot() *model.Root {
	return &model.Root{
		ID:        "test-series-id",
		Slug:      "test-series",
		Calendar:  "health",
		Title:     "Test",
		StartDate: dateStr(-30), // 30 days ago
		Freq:      model.FreqDaily,
		Interval:  1,
		AllDay:    true,
		Path:      "", // not needed for tests; reconcile doesn't touch root file
	}
}

// ---------- tests ----------

func TestFreshSeriesBackfillsAndCreatesForward(t *testing.T) {
	v := setup(t)
	r := baseRoot()

	rep, err := Series(v, r, nil)
	if err != nil {
		t.Fatal(err)
	}
	// 30 past + today + 12 months * ~30.42 days. Exact count is noisy; just
	// check both bands exist.
	if rep.Created < 30 {
		t.Errorf("Created = %d, want >= 30 (backfill past)", rep.Created)
	}
	pastPath := v.EventPath("health", dateStr(-15), "test-series")
	if _, err := os.Stat(pastPath); err != nil {
		t.Errorf("past event missing: %v", err)
	}
	futurePath := v.EventPath("health", dateStr(30), "test-series")
	if _, err := os.Stat(futurePath); err != nil {
		t.Errorf("future event missing: %v", err)
	}
}

func TestPreservesUserOwned(t *testing.T) {
	v := setup(t)
	r := baseRoot()

	// Drop a user-owned file in the future position.
	future := dateStr(5)
	writeRaw(t, v, &model.Event{
		Title:     "Custom",
		Date:      future,
		SeriesID:  r.ID,
		UserOwned: true,
		Body:      "hand-crafted",
	}, "health")

	_, err := Series(v, r, nil)
	if err != nil {
		t.Fatal(err)
	}
	e, err := model.ParseEvent(v.EventPath("health", future, r.Slug))
	if err != nil {
		t.Fatal(err)
	}
	if e.Title != "Custom" {
		t.Errorf("title = %q, want preserved 'Custom'", e.Title)
	}
	if !e.UserOwned {
		t.Error("user-owned got flipped off")
	}
}

func TestPastEventsNotRewritten(t *testing.T) {
	v := setup(t)
	r := baseRoot()

	// Hand-write a past event with an oddball title (not what reconcile
	// would produce). reconcile should leave it alone.
	past := dateStr(-5)
	writeRaw(t, v, &model.Event{
		Title:     "Happened",
		Date:      past,
		SeriesID:  r.ID,
		UserOwned: false,
		Body:      "done & done",
	}, "health")

	_, err := Series(v, r, nil)
	if err != nil {
		t.Fatal(err)
	}
	e, err := model.ParseEvent(v.EventPath("health", past, r.Slug))
	if err != nil {
		t.Fatal(err)
	}
	if e.Title != "Happened" {
		t.Errorf("past event rewritten: title = %q, want 'Happened'", e.Title)
	}
}

func TestArchivedPastNotRecreated(t *testing.T) {
	v := setup(t)
	r := baseRoot()

	past := dateStr(-10)
	year := past[:4]
	archiveDir := filepath.Join(v.ArchiveDir(), year, "health")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(archiveDir, past+"-test-series.md")
	if err := os.WriteFile(archivePath, []byte("---\ntitle: archived\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Series(v, r, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The archived date should NOT re-appear in events/.
	eventsPath := v.EventPath("health", past, r.Slug)
	if _, err := os.Stat(eventsPath); err == nil {
		t.Errorf("un-archived: %s was recreated", eventsPath)
	}
}

func TestRenamedEventNoDuplicate(t *testing.T) {
	v := setup(t)
	r := baseRoot()

	// First pass: full population.
	if _, err := Series(v, r, nil); err != nil {
		t.Fatal(err)
	}

	// User renames a future event. Keep the series-id in frontmatter.
	future := dateStr(5)
	origPath := v.EventPath("health", future, r.Slug)
	renamed := filepath.Join(v.EventsDir(), "health", future+" Something different.md")
	if err := os.Rename(origPath, renamed); err != nil {
		t.Fatal(err)
	}
	// Flip user-owned to match what the daemon would have done.
	e, _ := model.ParseEvent(renamed)
	e.UserOwned = true
	writer.SelfWriteNotifier = nil
	if err := writer.WriteEvent(e); err != nil {
		t.Fatal(err)
	}

	// Second reconcile — should NOT create a duplicate at origPath.
	if _, err := Series(v, r, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(origPath); err == nil {
		t.Errorf("duplicate created at %s after rename", origPath)
	}
	if _, err := os.Stat(renamed); err != nil {
		t.Errorf("renamed file got removed: %v", err)
	}
}

func TestOrphanSweepFutureNonUserOwned(t *testing.T) {
	v := setup(t)
	r := baseRoot()

	// Seed a non-user-owned event at a future date that's NOT in plan.
	// We fabricate by using an interval-2 daily rule so, say, today+5 isn't
	// in plan. Simpler: add an exception covering today+5.
	r.Exceptions = []string{dateStr(5)}

	// Pre-seed an event on today+5 with series-id, user-owned=false.
	strayDate := dateStr(5)
	writeRaw(t, v, &model.Event{
		Title:     "Stale",
		Date:      strayDate,
		SeriesID:  r.ID,
		UserOwned: false,
	}, "health")

	rep, err := Series(v, r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Swept != 1 {
		t.Errorf("Swept = %d, want 1", rep.Swept)
	}
	if _, err := os.Stat(v.EventPath("health", strayDate, r.Slug)); err == nil {
		t.Error("stray event not swept")
	}
}

func TestOrphanSweepSkipsUserOwned(t *testing.T) {
	v := setup(t)
	r := baseRoot()
	r.Exceptions = []string{dateStr(7)}

	userDate := dateStr(7)
	writeRaw(t, v, &model.Event{
		Title:     "Kept",
		Date:      userDate,
		SeriesID:  r.ID,
		UserOwned: true, // should NOT be swept
	}, "health")

	rep, err := Series(v, r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Swept != 0 {
		t.Errorf("Swept = %d, want 0 (user-owned preserved)", rep.Swept)
	}
	if _, err := os.Stat(v.EventPath("health", userDate, r.Slug)); err != nil {
		t.Errorf("user-owned orphan got swept: %v", err)
	}
}

func TestOrphanSweepSkipsPast(t *testing.T) {
	v := setup(t)
	r := baseRoot()
	r.Exceptions = []string{dateStr(-3)}

	pastDate := dateStr(-3)
	writeRaw(t, v, &model.Event{
		Title:     "History",
		Date:      pastDate,
		SeriesID:  r.ID,
		UserOwned: false,
	}, "health")

	rep, err := Series(v, r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Swept != 0 {
		t.Errorf("Swept = %d, want 0 (past immutable)", rep.Swept)
	}
	if _, err := os.Stat(v.EventPath("health", pastDate, r.Slug)); err != nil {
		t.Errorf("past event got swept: %v", err)
	}
}

func TestUpdatesNonUserOwnedFuture(t *testing.T) {
	v := setup(t)
	r := baseRoot()
	r.Title = "Updated Title"

	future := dateStr(5)
	writeRaw(t, v, &model.Event{
		Title:     "Stale Title",
		Date:      future,
		SeriesID:  r.ID,
		UserOwned: false,
	}, "health")

	if _, err := Series(v, r, nil); err != nil {
		t.Fatal(err)
	}
	e, err := model.ParseEvent(v.EventPath("health", future, r.Slug))
	if err != nil {
		t.Fatal(err)
	}
	if e.Title != "Updated Title" {
		t.Errorf("title = %q, want 'Updated Title' (overwrite)", e.Title)
	}
}

func TestUntilHonored(t *testing.T) {
	v := setup(t)
	r := baseRoot()
	r.StartDate = dateStr(-2)
	r.Until = dateStr(3)

	if _, err := Series(v, r, nil); err != nil {
		t.Fatal(err)
	}
	// Should have events for dates -2, -1, 0, 1, 2, 3 = 6 events.
	entries, err := os.ReadDir(filepath.Join(v.EventsDir(), "health"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 6 {
		t.Errorf("got %d files, want 6", len(entries))
	}
	// Nothing on dateStr(4) or beyond.
	tooFar := v.EventPath("health", dateStr(4), r.Slug)
	if _, err := os.Stat(tooFar); err == nil {
		t.Errorf("event past until: %s exists", tooFar)
	}
}
