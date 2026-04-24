// Package reconcile applies a computed expansion plan to disk: writes
// planned events, preserves user-owned occurrences, sweeps future orphans.
package reconcile

import (
	"fmt"
	"os"
	"time"

	"github.com/arch-err/calemdar/internal/config"
	"github.com/arch-err/calemdar/internal/expand"
	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/series"
	"github.com/arch-err/calemdar/internal/vault"
	"github.com/arch-err/calemdar/internal/writer"
)

// Report summarises a reconcile run for a single series.
type Report struct {
	InPlan  int
	Created int
	Updated int
	Skipped int // user-owned preserved
	Swept   int // orphan future events deleted
}

// Series reconciles r against disk over the configured horizon window
// starting today (in the configured timezone). Past events are immutable.
func Series(v *vault.Vault, r *model.Root) (*Report, error) {
	loc := model.Location()
	today := model.Today(loc)
	end := today.AddDate(0, config.Active.HorizonMonths, 0)

	events, err := expand.Expand(r, today, end, time.Now())
	if err != nil {
		return nil, err
	}

	planned := make(map[string]bool, len(events))
	for _, e := range events {
		e.Path = v.EventPath(r.Calendar, e.Date, r.Slug)
		planned[e.Path] = true
	}

	rep := &Report{InPlan: len(events)}
	for _, e := range events {
		if existing, err := model.ParseEvent(e.Path); err == nil {
			if existing.UserOwned {
				rep.Skipped++
				continue
			}
			rep.Updated++
		} else {
			rep.Created++
		}
		if err := writer.WriteEvent(e); err != nil {
			return rep, fmt.Errorf("write %s: %w", e.Path, err)
		}
	}

	existing, err := series.LoadEventsForSeries(v, r)
	if err != nil {
		return rep, err
	}
	for _, ex := range existing {
		if planned[ex.Path] || ex.UserOwned {
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
