package model

// Recurrence frequencies supported in v1.
const (
	FreqDaily   = "daily"
	FreqWeekly  = "weekly"
	FreqMonthly = "monthly"
)

// Weekdays used in byday lists. Lowercase three-letter.
var Weekdays = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

// ValidFreq reports whether s is a recognized frequency.
func ValidFreq(s string) bool {
	return s == FreqDaily || s == FreqWeekly || s == FreqMonthly
}

// ValidWeekday reports whether s is a recognized weekday token.
func ValidWeekday(s string) bool {
	for _, w := range Weekdays {
		if w == s {
			return true
		}
	}
	return false
}
