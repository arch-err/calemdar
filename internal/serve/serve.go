// Package serve is the long-running daemon. It wires the fsnotify watcher
// to the reactor, reconcile, archive, and store layers.
package serve

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/arch-err/calemdar/internal/config"
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
	// Make sure the vault tree exists before the watcher tries to attach.
	if rep, err := vault.Scaffold(opts.Vault, config.Active.Calendars); err != nil {
		return err
	} else if len(rep.Created) > 0 {
		for _, p := range rep.Created {
			log.Printf("serve: created %s", p)
		}
	}

	debounce := time.Duration(config.Active.DebounceMs) * time.Millisecond
	w, err := watch.StartWithDebounce(opts.Vault, debounce)
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

// runNightlyLoop fires runNightly at the configured nightly_at in the
// configured timezone, every day, until ctx is cancelled.
func runNightlyLoop(ctx context.Context, opts Options) {
	loc := model.Location()
	hh, mm, err := parseHHMM(config.Active.NightlyAt)
	if err != nil {
		log.Printf("serve: bad nightly_at %q, skipping nightly: %v", config.Active.NightlyAt, err)
		return
	}
	for {
		next := nextNightly(loc, time.Now(), hh, mm)
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

// nextNightly returns the next HH:MM in loc strictly after now.
func nextNightly(loc *time.Location, now time.Time, hh, mm int) time.Time {
	now = now.In(loc)
	t := time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, loc)
	if !t.After(now) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

func parseHHMM(s string) (int, int, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, 0, fmt.Errorf("not HH:MM: %w", err)
	}
	return t.Hour(), t.Minute(), nil
}
