package model

// Calendars is the fixed v1 list. Each maps to a top-level folder under
// events/ and to a Full Calendar source with its own color.
var Calendars = []string{
	"health",
	"tech",
	"work",
	"life",
	"friends-family",
	"special",
}

// ValidCalendar reports whether s is a recognized v1 calendar.
func ValidCalendar(s string) bool {
	for _, c := range Calendars {
		if c == s {
			return true
		}
	}
	return false
}
