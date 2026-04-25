package serve

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/arch-err/calemdar/internal/autoown"
	"github.com/arch-err/calemdar/internal/backup"
	"github.com/arch-err/calemdar/internal/fcparse"
	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/reactor"
	"github.com/arch-err/calemdar/internal/reconcile"
	"github.com/arch-err/calemdar/internal/series"
	"github.com/arch-err/calemdar/internal/vault"
	"github.com/arch-err/calemdar/internal/watch"
	"github.com/arch-err/calemdar/internal/writer"
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
//   - Deleted (external): try to restore from the sqlite snapshot and
//     drop a copy in the laptop-local backup dir. If the snapshot is
//     missing (pre-snapshot DB / never reconciled), log and bail.
func dispatchRecurring(opts Options, ev watch.Event) error {
	if ev.Kind == watch.Deleted {
		return handleRecurringDelete(opts, ev.Path)
	}
	r, err := model.ParseRoot(ev.Path)
	if err != nil {
		// Non-FC or malformed file landed in recurring/: log and skip.
		log.Printf("serve: ignoring unparseable recurring %s: %v", ev.Path, err)
		return nil
	}
	rep, err := reconcile.Series(opts.Vault, r, func() ([]*model.Event, error) { return opts.Store.ListOccurrencesBySeriesID(r.ID) })
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
//   - Deleted: drop the store row and, if the occurrence belonged to a
//     series, append its date to the root's `exceptions:` so reconcile
//     (live + nightly) won't recreate it. Sticky cross-device delete.
//   - Changed + FC recurring/rrule: migrate via reactor.
//   - Changed + other: auto-flip user-owned, upsert in store.
func dispatchEvents(opts Options, ev watch.Event) error {
	if ev.Kind == watch.Deleted {
		// Capture metadata before dropping the row — the file is gone so
		// we can't re-parse it, but the cache still holds series_id + date.
		occ, oerr := opts.Store.GetOccurrenceByPath(ev.Path)
		if oerr != nil {
			log.Printf("serve: lookup before delete %s: %v", ev.Path, oerr)
		}
		if err := opts.Store.DeleteOccurrence(ev.Path); err != nil {
			return err
		}
		if occ != nil && occ.SeriesID != "" {
			if err := stickyDeleteAddException(opts, occ); err != nil {
				log.Printf("serve: sticky-delete %s: %v", ev.Path, err)
			}
		}
		return nil
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

// handleRecurringDelete is the auto-restore path for an externally
// deleted recurring root. Self-deletes are already swallowed by the
// watcher (see writer.NotifySelfDelete + watch.NotifySelfDelete) — by
// the time we get here the delete is, definitionally, not ours.
//
// Restore strategy: look up the series by root_path in the sqlite
// cache, get the body_raw snapshot, mirror it to the backup dir, and
// rewrite the file. Single-shot — if a sync peer immediately deletes
// again, we'll get a fresh DELETE event and try again; we don't loop
// internally.
func handleRecurringDelete(opts Options, path string) error {
	r, err := opts.Store.GetSeriesByPath(path)
	if err != nil {
		return fmt.Errorf("recurring delete: store lookup: %w", err)
	}
	if r == nil {
		log.Printf("serve: recurring deleted (no snapshot): %s — events left in place; run `calemdar reindex` to clean store", path)
		return nil
	}
	if r.RawSource == "" {
		log.Printf("serve: recurring deleted: %s (slug=%s) — snapshot missing, can't restore. run `calemdar reindex` after recreating the file", path, r.Slug)
		return nil
	}

	// Mirror the snapshot to the backup dir BEFORE the rewrite — if the
	// rewrite fails the user still has a copy.
	bkp, berr := backup.WriteFromBytes(opts.Vault, r.Slug, []byte(r.RawSource), time.Now())
	if berr != nil {
		log.Printf("serve: backup write failed for %s: %v", path, berr)
	} else {
		log.Printf("serve: recurring backup written: %s", bkp)
	}

	log.Printf("serve: WARNING — external delete on recurring root %s (slug=%s); auto-restoring from snapshot", path, r.Slug)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("recurring restore: mkdir: %w", err)
	}
	if err := os.WriteFile(path, []byte(r.RawSource), 0o644); err != nil {
		return fmt.Errorf("recurring restore: write: %w", err)
	}
	// The follow-up CREATE will land back here as Changed, which will
	// reconcile + UpsertSeries (refreshing the body_raw snapshot from disk).
	// Tell the watcher this write is ours so we don't loop on the
	// reconcile path either.
	writer.NotifySelf(path)
	return nil
}

// stickyDeleteAddException records the deleted occurrence's date as an
// exception on its series root. Implements sync-resilient delete: an
// occurrence removed in obsidian on any peer ends up here as a DELETE
// event — we update the root once, future reconciles (live + nightly)
// honour the exception, and the deletion sticks across all devices.
//
// No-ops if the series root has been deleted as well, or if the date is
// already in the exceptions list. Writes the root with the self-write
// flag so the watcher doesn't bounce the change back through
// dispatchRecurring; reconcile + store upsert are run inline.
func stickyDeleteAddException(opts Options, occ *model.Event) error {
	r, err := series.FindByIDOrSlug(opts.Vault, occ.SeriesID)
	if err != nil {
		return err
	}
	if r == nil {
		// Series root is gone too — nothing to update. The root-delete
		// path handles its own cleanup.
		return nil
	}
	for _, ex := range r.Exceptions {
		if ex == occ.Date {
			return nil
		}
	}
	r.Exceptions = append(r.Exceptions, occ.Date)
	sort.Strings(r.Exceptions)

	if err := writer.WriteRoot(r); err != nil {
		return fmt.Errorf("write root: %w", err)
	}

	rep, err := reconcile.Series(opts.Vault, r, func() ([]*model.Event, error) { return opts.Store.ListOccurrencesBySeriesID(r.ID) })
	if err != nil {
		return fmt.Errorf("reconcile after exception: %w", err)
	}
	log.Printf("serve: sticky-delete %s on %s — reconciled (in-plan=%d created=%d swept=%d)",
		occ.Date, r.Slug, rep.InPlan, rep.Created, rep.Swept)

	if err := opts.Store.UpsertSeries(r); err != nil {
		return fmt.Errorf("store upsert series: %w", err)
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
