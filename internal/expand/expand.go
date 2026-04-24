// Package expand computes the concrete occurrence events of a recurring
// root within a given date window.
package expand

import (
	"fmt"
	"strings"
	"time"

	"github.com/arch-err/calemdar/internal/model"
)

// Expand returns the list of occurrence events for r within [start, end].
// start and end are treated as date-only; end is inclusive. Both must be in
// the same location; that location is used for all date math.
//
// ExpandedAt stamps the returned events' series-expanded-at field. Pass a
// pre-resolved time so callers can batch multiple expansions with a single
// timestamp.
func Expand(r *model.Root, start, end, expandedAt time.Time) ([]*model.Event, error) {
	if r == nil {
		return nil, fmt.Errorf("expand: nil root")
	}
	if !model.ValidCalendar(r.Calendar) {
		return nil, fmt.Errorf("expand: unknown calendar %q", r.Calendar)
	}
	if !model.ValidFreq(r.Freq) {
		return nil, fmt.Errorf("expand: unknown freq %q", r.Freq)
	}
	if r.Interval < 1 {
		return nil, fmt.Errorf("expand: interval must be >= 1, got %d", r.Interval)
	}

	loc := start.Location()
	startDate, err := model.ParseDate(r.StartDate, loc)
	if err != nil {
		return nil, fmt.Errorf("expand: start-date: %w", err)
	}
	if startDate.IsZero() {
		return nil, fmt.Errorf("expand: missing start-date")
	}
	until, err := model.ParseDate(r.Until, loc)
	if err != nil {
		return nil, fmt.Errorf("expand: until: %w", err)
	}

	exceptions := make(map[string]struct{}, len(r.Exceptions))
	for _, e := range r.Exceptions {
		d, err := model.ParseDate(e, loc)
		if err != nil {
			return nil, fmt.Errorf("expand: exception: %w", err)
		}
		exceptions[model.FormatDate(d)] = struct{}{}
	}

	// Effective window clipped by start-date and until.
	effStart := maxDate(start, startDate)
	effEnd := end
	if !until.IsZero() && until.Before(effEnd) {
		effEnd = until
	}
	if effEnd.Before(effStart) {
		return nil, nil
	}

	var dates []time.Time
	switch r.Freq {
	case model.FreqDaily:
		dates = daily(startDate, effStart, effEnd, r.Interval)
	case model.FreqWeekly:
		if len(r.ByDay) == 0 {
			return nil, fmt.Errorf("expand: weekly freq requires byday")
		}
		days, err := parseWeekdays(r.ByDay)
		if err != nil {
			return nil, fmt.Errorf("expand: %w", err)
		}
		dates = weekly(startDate, effStart, effEnd, r.Interval, days)
	case model.FreqMonthly:
		if len(r.ByMonthDay) == 0 {
			return nil, fmt.Errorf("expand: monthly freq requires bymonthday")
		}
		dates = monthly(startDate, effStart, effEnd, r.Interval, r.ByMonthDay)
	}

	events := make([]*model.Event, 0, len(dates))
	for _, d := range dates {
		key := model.FormatDate(d)
		if _, skip := exceptions[key]; skip {
			continue
		}
		events = append(events, buildEvent(r, d, expandedAt))
	}
	return events, nil
}

// buildEvent constructs a single expanded Event for root r at date d.
// Path is left empty — the writer populates it based on vault location.
func buildEvent(r *model.Root, d, expandedAt time.Time) *model.Event {
	body := "[[" + r.Slug + "]]\n\n" + strings.TrimLeft(r.Body, "\n")
	return &model.Event{
		Title:            r.Title,
		Date:             model.FormatDate(d),
		StartTime:        r.StartTime,
		EndTime:          r.EndTime,
		AllDay:           r.AllDay,
		Type:             "single",
		SeriesID:         r.ID,
		SeriesExpandedAt: expandedAt.UTC().Format(time.RFC3339),
		UserOwned:        false,
		Body:             body,
	}
}

func maxDate(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
