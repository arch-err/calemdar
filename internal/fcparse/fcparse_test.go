package fcparse

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/arch-err/calemdar/internal/model"
)

func TestSlugify(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Workout", "workout"},
		{"My New Event!", "my-new-event"},
		{"  Spaces  ", "spaces"},
		{"emoji 🎉 event", "emoji-event"},
		{"multi---dash", "multi-dash"},
	}
	for _, tt := range tests {
		if got := Slugify(tt.in); got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseRRuleWeekly(t *testing.T) {
	rr, err := ParseRRule("FREQ=WEEKLY;BYDAY=MO,WE,FR")
	if err != nil {
		t.Fatal(err)
	}
	if rr.Freq != "WEEKLY" {
		t.Errorf("Freq = %q", rr.Freq)
	}
	if rr.Interval != 1 {
		t.Errorf("Interval = %d", rr.Interval)
	}
	if got, want := rr.ByDay, []string{"MO", "WE", "FR"}; !eqStr(got, want) {
		t.Errorf("ByDay = %v, want %v", got, want)
	}
}

func TestParseRRuleWithIntervalAndUntil(t *testing.T) {
	rr, err := ParseRRule("FREQ=DAILY;INTERVAL=3;UNTIL=20270501T000000Z")
	if err != nil {
		t.Fatal(err)
	}
	if rr.Interval != 3 {
		t.Errorf("Interval = %d", rr.Interval)
	}
	if rr.Until != "2027-05-01" {
		t.Errorf("Until = %q", rr.Until)
	}
}

func TestParseRRuleRejectsCount(t *testing.T) {
	_, err := ParseRRule("FREQ=DAILY;COUNT=10")
	if err == nil {
		t.Fatal("expected error for COUNT")
	}
}

func TestParseRRuleRejectsPositionalByday(t *testing.T) {
	_, err := ParseRRule("FREQ=MONTHLY;BYDAY=1MO")
	if err == nil {
		t.Fatal("expected error for positional BYDAY")
	}
}

func TestTranslateRecurring(t *testing.T) {
	fc := &Recurring{
		Title:      "Workout",
		Type:       TypeRecurring,
		DaysOfWeek: []string{"M", "W", "F"},
		StartRecur: "2026-05-01",
		EndRecur:   "2027-05-01",
		StartTime:  "10:00",
		EndTime:    "11:00",
	}
	r, err := TranslateRecurring(fc, "health")
	if err != nil {
		t.Fatal(err)
	}
	if r.Freq != model.FreqWeekly {
		t.Errorf("Freq = %q", r.Freq)
	}
	if got, want := r.ByDay, []string{"mon", "wed", "fri"}; !eqStr(got, want) {
		t.Errorf("ByDay = %v, want %v", got, want)
	}
	if r.Calendar != "health" {
		t.Errorf("Calendar = %q", r.Calendar)
	}
	if r.ID == "" {
		t.Error("ID empty")
	}
	if r.StartDate != "2026-05-01" || r.Until != "2027-05-01" {
		t.Errorf("dates: %q .. %q", r.StartDate, r.Until)
	}
}

func TestTranslateRRuleMonthly(t *testing.T) {
	fc := &RRule{
		Title:     "Payday",
		Type:      TypeRRule,
		RRule:     "FREQ=MONTHLY;BYMONTHDAY=25;INTERVAL=1",
		StartDate: "2026-05-25",
	}
	r, err := TranslateRRule(fc, "life")
	if err != nil {
		t.Fatal(err)
	}
	if r.Freq != model.FreqMonthly {
		t.Errorf("Freq = %q", r.Freq)
	}
	if got, want := r.ByMonthDay, []int{25}; !eqInt(got, want) {
		t.Errorf("ByMonthDay = %v, want %v", got, want)
	}
}

func TestDetect(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "e.md")
	content := `---
type: recurring
title: x
daysOfWeek: [M]
startRecur: 2026-05-01
---
body
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Detect(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "recurring" {
		t.Errorf("Detect = %q", got)
	}
}

func eqStr(a, b []string) bool {
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

func eqInt(a, b []int) bool {
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
