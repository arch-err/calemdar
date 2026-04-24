package expand

import (
	"fmt"
	"time"
)

// daily returns occurrence dates at interval-day steps from startDate,
// clipped to [winStart, winEnd].
func daily(startDate, winStart, winEnd time.Time, interval int) []time.Time {
	// Align to the first on-or-after winStart that matches the interval cadence.
	daysFromStart := int(winStart.Sub(startDate).Hours() / 24)
	if daysFromStart < 0 {
		daysFromStart = 0
	}
	// Round up to the next interval boundary.
	aligned := daysFromStart
	if mod := aligned % interval; mod != 0 {
		aligned += interval - mod
	}

	var out []time.Time
	for d := startDate.AddDate(0, 0, aligned); !d.After(winEnd); d = d.AddDate(0, 0, interval) {
		if d.Before(startDate) {
			continue
		}
		out = append(out, d)
	}
	return out
}

// weekly returns occurrence dates for a weekly+byday+interval rule, clipped
// to [winStart, winEnd]. Weeks are anchored to the Monday of startDate's week.
// A date D is included iff:
//   - weekday(D) ∈ byday
//   - weeks-since-anchor(D) % interval == 0
//   - D ∈ [startDate, winEnd]
func weekly(startDate, winStart, winEnd time.Time, interval int, byday map[time.Weekday]struct{}) []time.Time {
	anchor := mondayOfWeek(startDate)

	// Walk day by day through the window. Cheap — worst case ~1yr = 366 iters.
	var out []time.Time
	cur := winStart
	if cur.Before(startDate) {
		cur = startDate
	}
	for !cur.After(winEnd) {
		if _, ok := byday[cur.Weekday()]; ok {
			weeksSince := daysBetween(anchor, cur) / 7
			if weeksSince%interval == 0 {
				out = append(out, cur)
			}
		}
		cur = cur.AddDate(0, 0, 1)
	}
	return out
}

// monthly returns occurrence dates for a monthly+bymonthday+interval rule,
// clipped to [winStart, winEnd]. Month cadence is anchored to startDate's
// (year, month); each month in the cadence emits the listed days-of-month,
// skipping any that don't exist in that month (e.g. day 31 in February).
func monthly(startDate, winStart, winEnd time.Time, interval int, bymonthday []int) []time.Time {
	loc := startDate.Location()
	anchorYM := startDate.Year()*12 + int(startDate.Month()) - 1

	// Start month = first month in window on or after startDate that matches cadence.
	winYM := winStart.Year()*12 + int(winStart.Month()) - 1
	if winYM < anchorYM {
		winYM = anchorYM
	}
	aligned := winYM
	if mod := (aligned - anchorYM) % interval; mod != 0 {
		aligned += interval - mod
	}

	endYM := winEnd.Year()*12 + int(winEnd.Month()) - 1

	var out []time.Time
	for ym := aligned; ym <= endYM; ym += interval {
		year := ym / 12
		month := time.Month((ym % 12) + 1)
		for _, dom := range bymonthday {
			d := time.Date(year, month, dom, 0, 0, 0, 0, loc)
			// time.Date normalizes invalid days (e.g. Feb 31 → Mar 3); reject.
			if d.Month() != month {
				continue
			}
			if d.Before(startDate) || d.Before(winStart) || d.After(winEnd) {
				continue
			}
			out = append(out, d)
		}
	}
	return out
}

func parseWeekdays(tokens []string) (map[time.Weekday]struct{}, error) {
	m := make(map[time.Weekday]struct{}, len(tokens))
	for _, t := range tokens {
		wd, ok := weekdayFromToken(t)
		if !ok {
			return nil, fmt.Errorf("unknown weekday token %q", t)
		}
		m[wd] = struct{}{}
	}
	return m, nil
}

func weekdayFromToken(s string) (time.Weekday, bool) {
	switch s {
	case "mon":
		return time.Monday, true
	case "tue":
		return time.Tuesday, true
	case "wed":
		return time.Wednesday, true
	case "thu":
		return time.Thursday, true
	case "fri":
		return time.Friday, true
	case "sat":
		return time.Saturday, true
	case "sun":
		return time.Sunday, true
	}
	return 0, false
}

// mondayOfWeek returns the Monday of t's ISO week (midnight, same location).
func mondayOfWeek(t time.Time) time.Time {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7 // Go's Sunday=0 → treat as last day of week
	}
	return t.AddDate(0, 0, -(wd - 1))
}

// daysBetween returns b - a in whole days.
func daysBetween(a, b time.Time) int {
	return int(b.Sub(a).Hours() / 24)
}
