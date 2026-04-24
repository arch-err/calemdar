package fcparse

import "strings"

// Slugify lowercases s and replaces any run of non-alphanumeric runes with a
// single dash. Trims leading/trailing dashes.
func Slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prevDash := true
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
