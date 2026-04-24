package serve

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/arch-err/calemdar/internal/autoown"
	"github.com/arch-err/calemdar/internal/fcparse"
	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/reactor"
	"github.com/arch-err/calemdar/internal/reconcile"
	"github.com/arch-err/calemdar/internal/series"
	"github.com/arch-err/calemdar/internal/vault"
	"github.com/arch-err/calemdar/internal/watch"
)

func dispatch(opts Options, ev watch.Event) error {
	switch ev.Source {
	case watch.SourceRecurring:
		return dispatchRecurring(opts, ev)
	case watch.SourceEvents:
		return dispatchEvents(opts, ev)
	}
	return nil
}

// dispatchRecurring handles changes to roots in recurring/.
//   - Changed: parse, reconcile, upsert series + occurrences in store.
//   - Deleted: leave events alone (past may matter); purge series from store.
//     Caller can `calemdar reindex` to GC dangling occurrences if desired.
func dispatchRecurring(opts Options, ev watch.Event) error {
	if ev.Kind == watch.Deleted {
		log.Printf("serve: recurring deleted: %s (events left in place; run `calemdar reindex` to clean store)", ev.Path)
		return nil
	}
	r, err := model.ParseRoot(ev.Path)
	if err != nil {
		// Non-FC or malformed file landed in recurring/: log and skip.
		log.Printf("serve: ignoring unparseable recurring %s: %v", ev.Path, err)
		return nil
	}
	rep, err := reconcile.Series(opts.Vault, r)
	if err != nil {
		return err
	}
	log.Printf("serve: reconciled %s — in-plan=%d created=%d updated=%d skipped=%d swept=%d",
		r.Slug, rep.InPlan, rep.Created, rep.Updated, rep.Skipped, rep.Swept)

	if err := opts.Store.UpsertSeries(r); err != nil {
		return fmt.Errorf("store upsert series: %w", err)
	}
	// Refresh store occurrences for this series.
	existing, err := series.LoadEventsForSeries(opts.Vault, r)
	if err != nil {
		return err
	}
	for _, e := range existing {
		cal := calendarFromPath(opts.Vault, e.Path)
		if err := opts.Store.UpsertOccurrence(e, cal); err != nil {
			return fmt.Errorf("store upsert occurrence: %w", err)
		}
	}
	return nil
}

// dispatchEvents handles changes to files in events/.
//   - Deleted: remove from store.
//   - Changed + FC recurring/rrule: migrate via reactor.
//   - Changed + other: auto-flip user-owned, upsert in store.
func dispatchEvents(opts Options, ev watch.Event) error {
	if ev.Kind == watch.Deleted {
		return opts.Store.DeleteOccurrence(ev.Path)
	}

	kind, err := fcparse.Detect(ev.Path)
	if err != nil {
		// File might be transiently gone (rapid save); log and ignore.
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if kind == fcparse.TypeRecurring || kind == fcparse.TypeRRule {
		return handleFCRecurring(opts, ev.Path)
	}

	flipped, err := autoown.FlipIfNeeded(ev.Path)
	if err != nil {
		return err
	}
	if flipped {
		log.Printf("serve: user-owned flipped: %s", ev.Path)
	}
	e, err := model.ParseEvent(ev.Path)
	if err != nil {
		return err
	}
	cal := calendarFromPath(opts.Vault, ev.Path)
	return opts.Store.UpsertOccurrence(e, cal)
}

// handleFCRecurring runs the reactor on the whole events/ tree. Cheap enough
// for now and always idempotent; avoids duplicating migration logic.
func handleFCRecurring(opts Options, path string) error {
	log.Printf("serve: FC recurring detected at %s — running reactor", path)
	migrations, err := reactor.Run(opts.Vault)
	if err != nil {
		return err
	}
	for _, m := range migrations {
		log.Printf("serve: migrated %s → %s (%d events)", m.FromPath, m.ToPath, m.Report.InPlan)
		if err := opts.Store.UpsertSeries(m.Series); err != nil {
			log.Printf("serve: store upsert after migration: %v", err)
		}
		existing, err := series.LoadEventsForSeries(opts.Vault, m.Series)
		if err == nil {
			for _, e := range existing {
				cal := calendarFromPath(opts.Vault, e.Path)
				_ = opts.Store.UpsertOccurrence(e, cal)
			}
		}
	}
	return nil
}

// calendarFromPath returns the first path component under events/.
func calendarFromPath(v *vault.Vault, path string) string {
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
