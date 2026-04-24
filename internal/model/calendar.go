package model

// Calendars is the active list. Each maps to a top-level folder under
// events/ and to a Full Calendar source with its own color. Runtime-settable
// via SetCalendars (config-driven at startup).
var Calendars = []string{
	"health",
	"tech",
	"work",
	"life",
	"friends-family",
	"special",
}

// SetCalendars replaces the active calendar list. Call once at startup
// after config.LoadAndApply.
func SetCalendars(cs []string) {
	if len(cs) == 0 {
		return
	}
	Calendars = append([]string(nil), cs...)
}

// ValidCalendar reports whether s is a recognized calendar.
func ValidCalendar(s string) bool {
	for _, c := range Calendars {
		if c == s {
			return true
		}
	}
	return false
}
