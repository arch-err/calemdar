package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/arch-err/calemdar/internal/fcparse"
	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/reconcile"
	"github.com/arch-err/calemdar/internal/series"
	"github.com/arch-err/calemdar/internal/writer"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func runSeriesNew(cmd *cobra.Command, args []string) error {
	v, err := resolveVault(cmd)
	if err != nil {
		return err
	}
	p := newPrompter()

	title, err := p.askRequired("title")
	if err != nil {
		return err
	}

	calendar, err := p.askChoice("calendar", model.Calendars)
	if err != nil {
		return err
	}

	freq, err := p.askChoice("freq", []string{model.FreqDaily, model.FreqWeekly, model.FreqMonthly})
	if err != nil {
		return err
	}

	interval, err := p.askInt("interval (every N periods)", 1)
	if err != nil {
		return err
	}
	if interval < 1 {
		return fmt.Errorf("interval must be >= 1")
	}

	var byday []string
	var bymonthday []int
	switch freq {
	case model.FreqWeekly:
		s, err := p.askRequired("byday (comma-separated: mon,tue,wed,thu,fri,sat,sun)")
		if err != nil {
			return err
		}
		byday, err = parseByDay(s)
		if err != nil {
			return err
		}
	case model.FreqMonthly:
		s, err := p.askRequired("bymonthday (comma-separated: 1,15,28)")
		if err != nil {
			return err
		}
		bymonthday, err = parseByMonthDay(s)
		if err != nil {
			return err
		}
	}

	startDate, err := p.askRequired("start-date (YYYY-MM-DD)")
	if err != nil {
		return err
	}
	if _, err := model.ParseDate(startDate, time.UTC); err != nil {
		return err
	}

	until, err := p.ask("until (YYYY-MM-DD, blank for none): ")
	if err != nil {
		return err
	}
	if until != "" {
		if _, err := model.ParseDate(until, time.UTC); err != nil {
			return err
		}
	}

	allDay, err := p.askYN("all-day", false)
	if err != nil {
		return err
	}
	var startTime, endTime string
	if !allDay {
		startTime, err = p.askRequired("start-time (HH:MM)")
		if err != nil {
			return err
		}
		endTime, err = p.askRequired("end-time (HH:MM)")
		if err != nil {
			return err
		}
	}

	slug := fcparse.Slugify(title)
	if slug == "" {
		return fmt.Errorf("title slugifies to empty")
	}

	// Collision check.
	target := filepath.Join(v.RecurringDir(), slug+".md")
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("slug collision: %s exists", target)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return err
	}

	r := &model.Root{
		ID:         id.String(),
		Calendar:   calendar,
		Title:      title,
		StartDate:  startDate,
		Until:      until,
		StartTime:  startTime,
		EndTime:    endTime,
		AllDay:     allDay,
		Freq:       freq,
		Interval:   interval,
		ByDay:      byday,
		ByMonthDay: bymonthday,
		Path:       target,
		Slug:       slug,
	}
	if err := writer.WriteRoot(r); err != nil {
		return err
	}
	fmt.Printf("wrote %s (id %s)\n", target, r.ID)

	reconcileNow, err := p.askYN("reconcile now", true)
	if err != nil {
		return err
	}
	if reconcileNow {
		rep, err := reconcile.Series(v, r)
		if err != nil {
			return err
		}
		printReport(r, rep)
	}
	return nil
}

func runSeriesExcept(cmd *cobra.Command, args []string) error {
	v, err := resolveVault(cmd)
	if err != nil {
		return err
	}
	r, err := series.FindByIDOrSlug(v, args[0])
	if err != nil {
		return err
	}
	if r == nil {
		return fmt.Errorf("series %q not found", args[0])
	}
	date := args[1]
	loc := model.Location()
	d, err := model.ParseDate(date, loc)
	if err != nil {
		return fmt.Errorf("bad date: %w", err)
	}
	if d.Before(model.Today(loc)) {
		fmt.Fprintf(os.Stderr,
			"note: %s is in the past — past events are never rewritten, so the existing file (if any) stays put. The exception only affects future reconciles.\n",
			date)
	}

	// Dedup: only append if not already present.
	for _, e := range r.Exceptions {
		if e == date {
			fmt.Printf("%s already in exceptions for %s\n", date, r.Slug)
			return nil
		}
	}
	r.Exceptions = append(r.Exceptions, date)

	if err := writer.WriteRoot(r); err != nil {
		return err
	}
	fmt.Printf("added %s to exceptions for %s\n", date, r.Slug)

	rep, err := reconcile.Series(v, r)
	if err != nil {
		return err
	}
	printReport(r, rep)
	return nil
}

// parseByDay splits a comma-separated list of weekday tokens and validates each.
func parseByDay(s string) ([]string, error) {
	out := []string{}
	for _, part := range strings.Split(s, ",") {
		p := strings.ToLower(strings.TrimSpace(part))
		if p == "" {
			continue
		}
		if !model.ValidWeekday(p) {
			return nil, fmt.Errorf("invalid weekday %q (use mon|tue|wed|thu|fri|sat|sun)", p)
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("byday empty")
	}
	return out, nil
}

func parseByMonthDay(s string) ([]int, error) {
	out := []int{}
	for _, part := range strings.Split(s, ",") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 31 {
			return nil, fmt.Errorf("invalid month-day %q (1..31)", p)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("bymonthday empty")
	}
	return out, nil
}
