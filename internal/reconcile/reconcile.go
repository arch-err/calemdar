// Package reconcile applies a computed expansion plan to disk: writes
// planned events, preserves user-owned occurrences, sweeps future orphans.
package reconcile

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/arch-err/calemdar/internal/config"
	"github.com/arch-err/calemdar/internal/expand"
	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/series"
	"github.com/arch-err/calemdar/internal/vault"
	"github.com/arch-err/calemdar/internal/writer"
)

// archivedExists reports whether an event at (calendar, dateStr, slug) has
// already been moved to archive/<year>/<calendar>/. Used to avoid
// un-archiving events during the backfill-from-start-date window.
func archivedExists(v *vault.Vault, calendar, dateStr, slug string) bool {
	year := dateStr[:4]
	p := filepath.Join(v.ArchiveDir(), year, calendar, dateStr+"-"+slug+".md")
	_, err := os.Stat(p)
	return err == nil
}

// Report summarises a reconcile run for a single series.
type Report struct {
	InPlan  int
	Created int
	Updated int
	Skipped int // user-owned preserved
	Swept   int // orphan future events deleted
}

// Series reconciles r against disk. Window: [max(start-date, today - 0),
// today + HorizonMonths]. Past events (date < today) are backfilled on
// first-create only — never rewritten or un-archived.
func Series(v *vault.Vault, r *model.Root) (*Report, error) {
	loc := model.Location()
	today := model.Today(loc)
	end := today.AddDate(0, config.Active.HorizonMonths, 0)

	startDate, err := model.ParseDate(r.StartDate, loc)
	if err != nil {
		return nil, fmt.Errorf("reconcile: start-date: %w", err)
	}
	// Start window at start-date so past occurrences get backfilled on the
	// first reconcile (e.g. when FC's recurring event has startRecur earlier
	// than today). expand.Expand clips to the root's own start-date anyway.
	winStart := startDate

	events, err := expand.Expand(r, winStart, end, time.Now())
	if err != nil {
		return nil, err
	}

	// Index existing occurrences by date. Uses series-id (not filename) so a
	// renamed file still counts as "that date's slot is filled" — prevents
	// duplicates when a user renames an expanded event in the UI.
	existingForSeries, err := series.LoadEventsForSeries(v, r)
	if err != nil {
		return nil, err
	}
	byDate := make(map[string]*model.Event, len(existingForSeries))
	for _, ex := range existingForSeries {
		byDate[ex.Date] = ex
	}

	planned := make(map[string]bool, len(events))
	for _, e := range events {
		e.Path = v.EventPath(r.Calendar, e.Date, r.Slug)
		planned[e.Path] = true
	}

	rep := &Report{InPlan: len(events)}
	for _, e := range events {
		eDate, _ := model.ParseDate(e.Date, loc)
		isPast := eDate.Before(today)

		if ex, filled := byDate[e.Date]; filled {
			if ex.UserOwned {
				rep.Skipped++
				continue
			}
			// Past events, user-owned or not, are never rewritten.
			if isPast {
				rep.Skipped++
				continue
			}
			// Future non-user-owned slot exists. Overwrite in place — keep
			// the existing path in case a rename moved it off the canonical
			// naming (rare for non-user-owned, but be defensive).
			e.Path = ex.Path
			rep.Updated++
		} else {
			// No file with this series-id for this date.
			if isPast && archivedExists(v, r.Calendar, e.Date, r.Slug) {
				rep.Skipped++
				continue
			}
			rep.Created++
		}
		if err := writer.WriteEvent(e); err != nil {
			return rep, fmt.Errorf("write %s: %w", e.Path, err)
		}
	}

	// Orphan sweep: future non-user-owned events whose date isn't in the
	// plan. Matches by date (so renamed/off-canonical files still count as
	// "this date's slot") — prevents accidental deletion of user-renamed
	// events that happen to be non-user-owned.
	plannedDates := make(map[string]bool, len(events))
	for _, e := range events {
		plannedDates[e.Date] = true
	}
	for _, ex := range existingForSeries {
		if plannedDates[ex.Date] || ex.UserOwned {
			continue
		}
		exDate, err := model.ParseDate(ex.Date, loc)
		if err != nil || exDate.Before(today) {
			continue
		}
		if err := os.Remove(ex.Path); err != nil {
			return rep, fmt.Errorf("sweep %s: %w", ex.Path, err)
		}
		writer.NotifySelf(ex.Path)
		rep.Swept++
	}
	return rep, nil
}
