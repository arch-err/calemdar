package model

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleRoot = `---
id: 019073c4-d7e0-7d8f-a1f3-8b2c9e5f4a10
calendar: health
title: "Workout"
start-date: 2026-05-01
until: 2027-05-01
start-time: "10:00"
end-time: "11:00"
all-day: false
freq: weekly
interval: 1
byday: [mon, wed, fri]
exceptions:
  - 2026-05-08
---

Strength session, 45 min.
`

func TestParseRoot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workout.md")
	if err := os.WriteFile(path, []byte(sampleRoot), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := ParseRoot(path)
	if err != nil {
		t.Fatal(err)
	}

	if r.ID != "019073c4-d7e0-7d8f-a1f3-8b2c9e5f4a10" {
		t.Errorf("ID = %q", r.ID)
	}
	if r.Calendar != "health" {
		t.Errorf("Calendar = %q", r.Calendar)
	}
	if r.Title != "Workout" {
		t.Errorf("Title = %q", r.Title)
	}
	if r.StartDate != "2026-05-01" {
		t.Errorf("StartDate = %q", r.StartDate)
	}
	if r.Until != "2027-05-01" {
		t.Errorf("Until = %q", r.Until)
	}
	if r.StartTime != "10:00" {
		t.Errorf("StartTime = %q", r.StartTime)
	}
	if r.Freq != "weekly" {
		t.Errorf("Freq = %q", r.Freq)
	}
	if r.Interval != 1 {
		t.Errorf("Interval = %d", r.Interval)
	}
	if got, want := r.ByDay, []string{"mon", "wed", "fri"}; !equalStrings(got, want) {
		t.Errorf("ByDay = %v, want %v", got, want)
	}
	if len(r.Exceptions) != 1 || r.Exceptions[0] != "2026-05-08" {
		t.Errorf("Exceptions = %v", r.Exceptions)
	}
	if r.Slug != "workout" {
		t.Errorf("Slug = %q", r.Slug)
	}
	if r.Body == "" {
		t.Error("Body empty")
	}
}

const sampleEvent = `---
title: "Workout"
date: 2026-05-03
startTime: "10:00"
endTime: "11:00"
allDay: false
type: single
series-id: 019073c4-d7e0-7d8f-a1f3-8b2c9e5f4a10
series-expanded-at: 2026-04-24T11:00:00Z
user-owned: false
---

[[workout]]

Strength session, 45 min.
`

func TestParseEvent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "2026-05-03-workout.md")
	if err := os.WriteFile(path, []byte(sampleEvent), 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := ParseEvent(path)
	if err != nil {
		t.Fatal(err)
	}

	if e.Title != "Workout" {
		t.Errorf("Title = %q", e.Title)
	}
	if e.Date != "2026-05-03" {
		t.Errorf("Date = %q", e.Date)
	}
	if e.Type != "single" {
		t.Errorf("Type = %q", e.Type)
	}
	if e.SeriesID != "019073c4-d7e0-7d8f-a1f3-8b2c9e5f4a10" {
		t.Errorf("SeriesID = %q", e.SeriesID)
	}
	if e.UserOwned {
		t.Error("UserOwned = true, want false")
	}
	if e.IsOneOff() {
		t.Error("IsOneOff() = true, want false")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
