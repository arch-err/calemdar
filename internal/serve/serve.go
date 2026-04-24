// Package serve is the long-running daemon. It wires the fsnotify watcher
// to the reactor, reconcile, archive, and store layers.
package serve

import (
	"context"
	"log"
	"time"

	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/store"
	"github.com/arch-err/calemdar/internal/vault"
	"github.com/arch-err/calemdar/internal/watch"
	"github.com/arch-err/calemdar/internal/writer"
)

// Options bundles runtime dependencies for Run.
type Options struct {
	Vault *vault.Vault
	Store *store.Store
}

// Run starts the daemon. Returns when ctx is cancelled.
func Run(ctx context.Context, opts Options) error {
	w, err := watch.Start(opts.Vault)
	if err != nil {
		return err
	}
	defer w.Stop()

	// Install self-write notifier for every writer.Write* / NotifySelf call.
	writer.SelfWriteNotifier = w.NotifySelfWrite
	defer func() { writer.SelfWriteNotifier = nil }()

	// Initial sync: fresh reindex so the store matches disk.
	if rep, err := opts.Store.Reindex(opts.Vault); err != nil {
		log.Printf("serve: initial reindex failed: %v", err)
	} else {
		log.Printf("serve: reindexed: %d series, %d occurrences", rep.Series, rep.Occurrences)
	}

	// Nightly timer: extend horizon + archive.
	go runNightlyLoop(ctx, opts)

	log.Printf("serve: watching %s", opts.Vault.Root)

	for {
		select {
		case <-ctx.Done():
			log.Printf("serve: shutting down")
			return nil
		case ev, ok := <-w.Events():
			if !ok {
				return nil
			}
			if err := dispatch(opts, ev); err != nil {
				log.Printf("serve: dispatch %s %s: %v", ev.Source, ev.Path, err)
			}
		}
	}
}

// runNightlyLoop fires runNightly at 03:00 local Stockholm time, every day,
// until ctx is cancelled.
func runNightlyLoop(ctx context.Context, opts Options) {
	loc := model.Stockholm()
	for {
		next := nextNightly(loc, time.Now())
		log.Printf("serve: next nightly run at %s", next.Format(time.RFC3339))
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			runNightly(opts)
		}
	}
}

// nextNightly returns the next 03:00 in loc strictly after now.
func nextNightly(loc *time.Location, now time.Time) time.Time {
	now = now.In(loc)
	t := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, loc)
	if !t.After(now) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}
