package fcparse

import (
	"fmt"
	"strconv"
	"strings"
)

// RRuleFields is a parsed subset of RFC 5545 RRULE that calemdar v1 supports.
type RRuleFields struct {
	Freq       string   // DAILY | WEEKLY | MONTHLY
	Interval   int      // defaults to 1 if absent
	ByDay      []string // two-letter codes: MO, TU, WE, TH, FR, SA, SU
	ByMonthDay []int
	Until      string // YYYY-MM-DD (time part dropped)
}

// ParseRRule parses a subset of RFC 5545 RRULE. Unsupported fields return an
// error so the caller surfaces a clear migration failure rather than silently
// dropping data.
func ParseRRule(s string) (*RRuleFields, error) {
	if s == "" {
		return nil, fmt.Errorf("rrule: empty")
	}
	out := &RRuleFields{Interval: 1}

	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			return nil, fmt.Errorf("rrule: malformed part %q", part)
		}
		k = strings.ToUpper(strings.TrimSpace(k))
		v = strings.TrimSpace(v)

		switch k {
		case "FREQ":
			v = strings.ToUpper(v)
			switch v {
			case "DAILY", "WEEKLY", "MONTHLY":
				out.Freq = v
			default:
				return nil, fmt.Errorf("rrule: unsupported FREQ=%s (v1 supports DAILY|WEEKLY|MONTHLY)", v)
			}

		case "INTERVAL":
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("rrule: bad INTERVAL=%q", v)
			}
			out.Interval = n

		case "BYDAY":
			for _, d := range strings.Split(v, ",") {
				d = strings.TrimSpace(strings.ToUpper(d))
				// Reject positional prefixes like "1MO" (first monday).
				if len(d) != 2 {
					return nil, fmt.Errorf("rrule: positional BYDAY %q not supported in v1", d)
				}
				if !isRRuleWeekday(d) {
					return nil, fmt.Errorf("rrule: unknown weekday %q", d)
				}
				out.ByDay = append(out.ByDay, d)
			}

		case "BYMONTHDAY":
			for _, d := range strings.Split(v, ",") {
				d = strings.TrimSpace(d)
				n, err := strconv.Atoi(d)
				if err != nil || n < 1 || n > 31 {
					return nil, fmt.Errorf("rrule: bad BYMONTHDAY %q", d)
				}
				out.ByMonthDay = append(out.ByMonthDay, n)
			}

		case "UNTIL":
			// UNTIL can be YYYYMMDD or YYYYMMDDTHHMMSSZ. Take the date part.
			if len(v) < 8 {
				return nil, fmt.Errorf("rrule: bad UNTIL %q", v)
			}
			y, m, d := v[0:4], v[4:6], v[6:8]
			out.Until = y + "-" + m + "-" + d

		case "COUNT":
			return nil, fmt.Errorf("rrule: COUNT not supported in v1 (use UNTIL instead)")

		case "WKST":
			// weekday-start: ignored, we anchor to monday.

		case "BYSETPOS", "BYMONTH", "BYWEEKNO", "BYYEARDAY", "BYHOUR", "BYMINUTE", "BYSECOND":
			return nil, fmt.Errorf("rrule: %s not supported in v1", k)

		default:
			return nil, fmt.Errorf("rrule: unknown key %s", k)
		}
	}

	if out.Freq == "" {
		return nil, fmt.Errorf("rrule: missing FREQ")
	}
	return out, nil
}

func isRRuleWeekday(s string) bool {
	switch s {
	case "MO", "TU", "WE", "TH", "FR", "SA", "SU":
		return true
	}
	return false
}
