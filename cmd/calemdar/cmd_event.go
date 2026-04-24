package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/arch-err/calemdar/internal/fcparse"
	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/writer"
	"github.com/spf13/cobra"
)

func runEventNew(cmd *cobra.Command, args []string) error {
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
	date, err := p.askRequired("date (YYYY-MM-DD)")
	if err != nil {
		return err
	}
	if _, err := model.ParseDate(date, time.UTC); err != nil {
		return err
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

	target := v.EventPath(calendar, date, slug)
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("event already exists: %s", target)
	}

	e := &model.Event{
		Title:            title,
		Date:             date,
		StartTime:        startTime,
		EndTime:          endTime,
		AllDay:           allDay,
		Type:             "single",
		SeriesID:         "",
		SeriesExpandedAt: "",
		UserOwned:        true, // human-authored, server won't touch
		Body:             "",
		Path:             target,
	}
	if err := writer.WriteEvent(e); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", target)
	return nil
}

func runEventShow(cmd *cobra.Command, args []string) error {
	// Accept either an absolute path or a path relative to cwd.
	e, err := model.ParseEvent(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("path:       %s\n", e.Path)
	fmt.Printf("title:      %s\n", e.Title)
	fmt.Printf("date:       %s\n", e.Date)
	if e.AllDay {
		fmt.Printf("all-day:    true\n")
	} else {
		fmt.Printf("time:       %s–%s\n", e.StartTime, e.EndTime)
	}
	if e.SeriesID != "" {
		fmt.Printf("series-id:  %s\n", e.SeriesID)
	}
	fmt.Printf("user-owned: %t\n", e.UserOwned)
	if e.SeriesExpandedAt != "" {
		fmt.Printf("expanded:   %s\n", e.SeriesExpandedAt)
	}
	if strings.TrimSpace(e.Body) != "" {
		fmt.Printf("---\n%s", e.Body)
	}
	return nil
}

func runEventList(cmd *cobra.Command, args []string) error {
	v, err := resolveVault(cmd)
	if err != nil {
		return err
	}
	rangeFlag, _ := cmd.Flags().GetString("range")

	loc := model.Location()
	today := model.Today(loc)
	var from, to time.Time
	switch rangeFlag {
	case "today":
		from = today
		to = today
	case "week":
		from = today
		to = today.AddDate(0, 0, 7)
	case "month":
		from = today
		to = today.AddDate(0, 1, 0)
	case "all":
		// Show everything from today forward, no upper bound.
		from = today
		to = today.AddDate(100, 0, 0)
	default:
		return fmt.Errorf("range must be one of: today, week, month, all")
	}

	events, err := walkEvents(v)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "DATE\tTIME\tCALENDAR\tTITLE\tSERIES\tUSER-OWNED")
	printed := 0
	for _, e := range events {
		d, err := model.ParseDate(e.Date, loc)
		if err != nil {
			continue
		}
		if d.Before(from) || d.After(to) {
			continue
		}
		cal := calendarFromEventPath(v, e.Path)
		tstr := "all-day"
		if !e.AllDay {
			tstr = e.StartTime + "–" + e.EndTime
		}
		seriesDisplay := "-"
		if e.SeriesID != "" {
			seriesDisplay = e.SeriesID[:8]
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%t\n",
			e.Date, tstr, cal, e.Title, seriesDisplay, e.UserOwned)
		printed++
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if printed == 0 {
		fmt.Println("no events in range")
	}
	return nil
}

// walkEvents returns every event under events/. Skips archive/ (not a
// subdir of events/ in our layout, but safe belt-and-suspenders).
func walkEvents(v interface{ EventsDir() string }) ([]*model.Event, error) {
	var out []*model.Event
	dir := v.EventsDir()
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		e, err := model.ParseEvent(path)
		if err != nil {
			return fmt.Errorf("parse event: %w", err)
		}
		out = append(out, e)
		return nil
	})
	return out, err
}

// calendarFromEventPath pulls the first path component under events/.
func calendarFromEventPath(v interface{ EventsDir() string }, path string) string {
	rel, err := filepath.Rel(v.EventsDir(), path)
	if err != nil {
		return "?"
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) == 0 {
		return "?"
	}
	return parts[0]
}
