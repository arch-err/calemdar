package store

import (
	"path/filepath"
	"testing"

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
