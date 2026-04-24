package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/vault"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	root := t.TempDir()
	v := &vault.Vault{Root: root}
	s, err := Open(v)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestUpsertAndGetSeries(t *testing.T) {
	s := openTemp(t)
	r := &model.Root{
		ID: "abc", Slug: "workout", Calendar: "health", Title: "Workout",
		Freq: "weekly", Interval: 1, ByDay: []string{"mon", "wed"},
		StartDate: "2026-05-01", StartTime: "10:00", EndTime: "11:00",
		Path: "/vault/recurring/workout.md",
	}
	if err := s.UpsertSeries(r); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSeries("abc")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("nil")
	}
	if got.Title != "Workout" {
		t.Errorf("Title = %q", got.Title)
	}
	if len(got.ByDay) != 2 || got.ByDay[0] != "mon" {
		t.Errorf("ByDay = %v", got.ByDay)
	}
}

func TestUpsertOccurrenceAndList(t *testing.T) {
	s := openTemp(t)
	e1 := &model.Event{
		Path: "/vault/events/health/2026/2026-05-04-workout.md",
		Date: "2026-05-04", Title: "Workout", StartTime: "10:00", EndTime: "11:00",
		SeriesID: "abc", UserOwned: false,
	}
	e2 := &model.Event{
		Path: "/vault/events/health/2026/2026-05-06-workout.md",
		Date: "2026-05-06", Title: "Workout", StartTime: "10:00", EndTime: "11:00",
		SeriesID: "abc", UserOwned: true,
	}
	if err := s.UpsertOccurrence(e1, "health"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertOccurrence(e2, "health"); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListOccurrencesInRange("2026-05-01", "2026-05-07")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	if got[0].Date != "2026-05-04" || got[1].Date != "2026-05-06" {
		t.Errorf("dates = %q, %q", got[0].Date, got[1].Date)
	}
}

func TestListUpcomingFiltersByTimeAndCalendar(t *testing.T) {
	s := openTemp(t)
	loc := model.Location()

	// Two events at 10:00 on 2026-05-04, different calendars.
	e1 := &model.Event{
		Path: "/vault/events/health/2026-05-04-workout.md",
		Date: "2026-05-04", Title: "Workout", StartTime: "10:00", EndTime: "11:00",
	}
	e2 := &model.Event{
		Path: "/vault/events/work/2026-05-04-standup.md",
		Date: "2026-05-04", Title: "Standup", StartTime: "10:00", EndTime: "10:15",
	}
	// All-day event — should always be skipped.
	e3 := &model.Event{
		Path: "/vault/events/life/2026-05-04-birthday.md",
		Date: "2026-05-04", Title: "Birthday", AllDay: true,
	}
	// Out-of-window event.
	e4 := &model.Event{
		Path: "/vault/events/health/2026-05-04-evening.md",
		Date: "2026-05-04", Title: "Evening", StartTime: "20:00", EndTime: "21:00",
	}
	for _, e := range []*model.Event{e1, e2, e3, e4} {
		cal := "health"
		switch {
		case e.Path == e2.Path:
			cal = "work"
		case e.Path == e3.Path:
			cal = "life"
		}
		if err := s.UpsertOccurrence(e, cal); err != nil {
			t.Fatal(err)
		}
	}

	// Window: 09:55–10:05 on 2026-05-04 → should match both 10:00 events.
	from := time.Date(2026, 5, 4, 9, 55, 0, 0, loc)
	to := time.Date(2026, 5, 4, 10, 5, 0, 0, loc)

	got, err := s.ListUpcoming(from, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d, want 2 (10:00 events only): %+v", len(got), got)
	}

	// Restrict to calendar = work → only standup.
	gotWork, err := s.ListUpcoming(from, to, []string{"work"})
	if err != nil {
		t.Fatal(err)
	}
	if len(gotWork) != 1 {
		t.Fatalf("got %d, want 1 work event", len(gotWork))
	}
	if gotWork[0].Title != "Standup" {
		t.Errorf("title = %q", gotWork[0].Title)
	}

	// Evening event with window far away → empty.
	earlyFrom := time.Date(2026, 5, 4, 8, 0, 0, 0, loc)
	earlyTo := time.Date(2026, 5, 4, 8, 5, 0, 0, loc)
	early, err := s.ListUpcoming(earlyFrom, earlyTo, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(early) != 0 {
		t.Errorf("expected 0 for early window, got %d", len(early))
	}
}

func TestWipe(t *testing.T) {
	s := openTemp(t)
	r := &model.Root{ID: "abc", Slug: "w", Calendar: "health", Title: "t",
		Freq: "daily", Interval: 1, StartDate: "2026-05-01", Path: "/p"}
	if err := s.UpsertSeries(r); err != nil {
		t.Fatal(err)
	}
	if err := s.Wipe(); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetSeries("abc")
	if got != nil {
		t.Error("expected nil after Wipe")
	}
}

func TestDBFilePath(t *testing.T) {
	root := t.TempDir()
	v := &vault.Vault{Root: root}
	s, err := Open(v)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	want := filepath.Join(root, DBFile)
	if _, err := filepathStat(want); err != nil {
		t.Errorf("db not at expected path %s: %v", want, err)
	}
}

// filepathStat is a tiny wrapper so we don't need to import os in the test
// file's imports list (keeps the signal above obvious).
func filepathStat(p string) (any, error) {
	return stat(p)
}
