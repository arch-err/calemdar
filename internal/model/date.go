package model

import (
	"fmt"
	"time"
)

// DateLayout is the canonical on-disk date format (YYYY-MM-DD).
const DateLayout = "2006-01-02"

// ParseDate parses a date in YYYY-MM-DD form at midnight in the given location.
// Empty input returns the zero time.
func ParseDate(s string, loc *time.Location) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.ParseInLocation(DateLayout, s, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse date %q: %w", s, err)
	}
	return t, nil
}

// FormatDate formats t as YYYY-MM-DD.
func FormatDate(t time.Time) string {
	return t.Format(DateLayout)
}

// Stockholm is the default timezone for v1.
func Stockholm() *time.Location {
	loc, err := time.LoadLocation("Europe/Stockholm")
	if err != nil {
		// Fall back to UTC if tzdata is missing; date math is tz-insensitive so
		// this only affects "today" resolution at the local-midnight boundary.
		return time.UTC
	}
	return loc
}

// Today returns today's date at 00:00 in the given location.
func Today(loc *time.Location) time.Time {
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
}
