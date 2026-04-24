package expand

import (
	"testing"
	"time"

	"github.com/arch-err/calemdar/internal/model"
)

var (
	loc     = time.UTC
	mkDate  = func(s string) time.Time { t, _ := model.ParseDate(s, loc); return t }
	fixedAt = time.Date(2026, 4, 24, 11, 0, 0, 0, time.UTC)
)

func datesOf(events []*model.Event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.Date
	}
	return out
}

func TestExpandDaily(t *testing.T) {
	r := &model.Root{
		ID:        "id",
		Calendar:  "health",
		Title:     "Meds",
		StartDate: "2026-05-01",
		Freq:      model.FreqDaily,
		Interval:  1,
	}
	got, err := Expand(r, mkDate("2026-05-01"), mkDate("2026-05-05"), fixedAt)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2026-05-01", "2026-05-02", "2026-05-03", "2026-05-04", "2026-05-05"}
	if got := datesOf(got); !equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExpandDailyInterval2(t *testing.T) {
	r := &model.Root{
		ID: "id", Calendar: "health", Title: "x",
		StartDate: "2026-05-01", Freq: model.FreqDaily, Interval: 2,
	}
	got, _ := Expand(r, mkDate("2026-05-01"), mkDate("2026-05-07"), fixedAt)
	want := []string{"2026-05-01", "2026-05-03", "2026-05-05", "2026-05-07"}
	if g := datesOf(got); !equal(g, want) {
		t.Errorf("got %v, want %v", g, want)
	}
}

func TestExpandWeeklyByday(t *testing.T) {
	r := &model.Root{
		ID: "id", Calendar: "health", Title: "x",
		StartDate: "2026-05-01", // Fri
		Freq:      model.FreqWeekly,
		Interval:  1,
		ByDay:     []string{"mon", "wed", "fri"},
	}
	got, _ := Expand(r, mkDate("2026-05-01"), mkDate("2026-05-17"), fixedAt)
	want := []string{
		"2026-05-01", // Fri
		"2026-05-04", // Mon
		"2026-05-06", // Wed
		"2026-05-08", // Fri
		"2026-05-11", "2026-05-13", "2026-05-15",
	}
	if g := datesOf(got); !equal(g, want) {
		t.Errorf("got %v, want %v", g, want)
	}
}

func TestExpandWeeklyInterval2(t *testing.T) {
	r := &model.Root{
		ID: "id", Calendar: "health", Title: "x",
		StartDate: "2026-05-04", // Mon
		Freq:      model.FreqWeekly,
		Interval:  2,
		ByDay:     []string{"mon"},
	}
	got, _ := Expand(r, mkDate("2026-05-01"), mkDate("2026-06-30"), fixedAt)
	want := []string{"2026-05-04", "2026-05-18", "2026-06-01", "2026-06-15", "2026-06-29"}
	if g := datesOf(got); !equal(g, want) {
		t.Errorf("got %v, want %v", g, want)
	}
}

func TestExpandMonthlyByMonthday(t *testing.T) {
	r := &model.Root{
		ID: "id", Calendar: "health", Title: "x",
		StartDate: "2026-01-01", Freq: model.FreqMonthly, Interval: 1,
		ByMonthDay: []int{1, 15},
	}
	got, _ := Expand(r, mkDate("2026-01-01"), mkDate("2026-03-31"), fixedAt)
	want := []string{"2026-01-01", "2026-01-15", "2026-02-01", "2026-02-15", "2026-03-01", "2026-03-15"}
	if g := datesOf(got); !equal(g, want) {
		t.Errorf("got %v, want %v", g, want)
	}
}

func TestExpandMonthlyInvalidDayOfMonth(t *testing.T) {
	r := &model.Root{
		ID: "id", Calendar: "health", Title: "x",
		StartDate: "2026-01-31", Freq: model.FreqMonthly, Interval: 1,
		ByMonthDay: []int{31},
	}
	got, _ := Expand(r, mkDate("2026-01-01"), mkDate("2026-04-30"), fixedAt)
	// Feb has no 31st → skipped. Apr has 30 days → no 31st.
	want := []string{"2026-01-31", "2026-03-31"}
	if g := datesOf(got); !equal(g, want) {
		t.Errorf("got %v, want %v", g, want)
	}
}

func TestExpandUntil(t *testing.T) {
	r := &model.Root{
		ID: "id", Calendar: "health", Title: "x",
		StartDate: "2026-05-01", Until: "2026-05-03",
		Freq: model.FreqDaily, Interval: 1,
	}
	got, _ := Expand(r, mkDate("2026-05-01"), mkDate("2026-05-31"), fixedAt)
	want := []string{"2026-05-01", "2026-05-02", "2026-05-03"}
	if g := datesOf(got); !equal(g, want) {
		t.Errorf("got %v, want %v", g, want)
	}
}

func TestExpandExceptions(t *testing.T) {
	r := &model.Root{
		ID: "id", Calendar: "health", Title: "x",
		StartDate: "2026-05-01", Freq: model.FreqDaily, Interval: 1,
		Exceptions: []string{"2026-05-02", "2026-05-04"},
	}
	got, _ := Expand(r, mkDate("2026-05-01"), mkDate("2026-05-05"), fixedAt)
	want := []string{"2026-05-01", "2026-05-03", "2026-05-05"}
	if g := datesOf(got); !equal(g, want) {
		t.Errorf("got %v, want %v", g, want)
	}
}

func TestExpandWindowClipsBeforeStart(t *testing.T) {
	r := &model.Root{
		ID: "id", Calendar: "health", Title: "x",
		StartDate: "2026-05-10", Freq: model.FreqDaily, Interval: 1,
	}
	// Window starts before StartDate; should emit only from StartDate onward.
	got, _ := Expand(r, mkDate("2026-05-01"), mkDate("2026-05-12"), fixedAt)
	want := []string{"2026-05-10", "2026-05-11", "2026-05-12"}
	if g := datesOf(got); !equal(g, want) {
		t.Errorf("got %v, want %v", g, want)
	}
}

func TestExpandBuildsEventFields(t *testing.T) {
	r := &model.Root{
		ID: "sid", Calendar: "health", Title: "Workout",
		StartDate: "2026-05-01", Freq: model.FreqDaily, Interval: 1,
		StartTime: "10:00", EndTime: "11:00",
		Slug: "workout", Body: "body text",
	}
	got, _ := Expand(r, mkDate("2026-05-01"), mkDate("2026-05-01"), fixedAt)
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	e := got[0]
	if e.SeriesID != "sid" {
		t.Errorf("SeriesID = %q", e.SeriesID)
	}
	if e.Title != "Workout" {
		t.Errorf("Title = %q", e.Title)
	}
	if e.Type != "single" {
		t.Errorf("Type = %q", e.Type)
	}
	if e.UserOwned {
		t.Error("UserOwned = true")
	}
	if e.StartTime != "10:00" || e.EndTime != "11:00" {
		t.Errorf("times = %q/%q", e.StartTime, e.EndTime)
	}
	if e.SeriesExpandedAt == "" {
		t.Error("SeriesExpandedAt empty")
	}
	if e.Body != "[[workout]]\n\nbody text" {
		t.Errorf("Body = %q", e.Body)
	}
}

func TestExpandUnknownFreq(t *testing.T) {
	r := &model.Root{ID: "id", Calendar: "health", Title: "x", StartDate: "2026-05-01", Freq: "yearly", Interval: 1}
	_, err := Expand(r, mkDate("2026-05-01"), mkDate("2026-05-31"), fixedAt)
	if err == nil {
		t.Error("want error, got nil")
	}
}

func TestExpandUnknownCalendar(t *testing.T) {
	r := &model.Root{ID: "id", Calendar: "bogus", Title: "x", StartDate: "2026-05-01", Freq: model.FreqDaily, Interval: 1}
	_, err := Expand(r, mkDate("2026-05-01"), mkDate("2026-05-31"), fixedAt)
	if err == nil {
		t.Error("want error, got nil")
	}
}

func equal(a, b []string) bool {
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
