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

// defaultLocation is the tz used before SetTimezone is called. Matches
// config's default so code that runs before config.LoadAndApply (including
// all unit tests) sees a reasonable value.
var defaultLocation = func() *time.Location {
	if loc, err := time.LoadLocation("Europe/Stockholm"); err == nil {
		return loc
	}
	return time.UTC
}()

// Location returns the configured timezone. Runtime-settable via SetTimezone.
func Location() *time.Location { return defaultLocation }

// SetTimezone updates the package-global timezone. Call once at startup
// after config.LoadAndApply.
func SetTimezone(loc *time.Location) {
	if loc != nil {
		defaultLocation = loc
	}
}

// ResolvedLocation loads a timezone by IANA name, falling back to UTC on
// failure. Returns the loaded location and nil on success.
func ResolvedLocation(name string) (*time.Location, error) {
	return time.LoadLocation(name)
}

// Stockholm is kept as an alias for backward compatibility with v1 callers.
// Prefer Location() in new code.
//
// Deprecated: use Location().
func Stockholm() *time.Location { return Location() }

// Today returns today's date at 00:00 in the given location.
func Today(loc *time.Location) time.Time {
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
}
