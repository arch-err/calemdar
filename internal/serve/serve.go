// Package serve is the long-running daemon. It wires the fsnotify watcher
// to the reactor, reconcile, archive, and store layers.
package serve

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/arch-err/calemdar/internal/actions"
	"github.com/arch-err/calemdar/internal/config"
	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/notify"
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
	writer.SelfDeleteNotifier = w.NotifySelfDelete
	defer func() {
		writer.SelfWriteNotifier = nil
		writer.SelfDeleteNotifier = nil
	}()

	// Initial sync: fresh reindex so the store matches disk.
	if rep, err := opts.Store.Reindex(opts.Vault); err != nil {
		log.Printf("serve: initial reindex failed: %v", err)
	} else {
		log.Printf("serve: reindexed: %d series, %d occurrences", rep.Series, rep.Occurrences)
	}

	// Nightly timer: extend horizon + archive.
	go runNightlyLoop(ctx, opts)

	// Per-event notifications: register configured backends, build the
	// actions runner (if enabled), spawn the scheduler.
	if config.Active.Notifications.Enabled {
		if err := startScheduler(ctx, opts); err != nil {
			log.Printf("serve: notif scheduler not started: %v", err)
		}
	}

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

// startScheduler wires backends + actions runner + scheduler. Each side
// is independently optional: if the ntfy backend is disabled, we just
// don't register it. Returns nil if at least one backend is registered;
// otherwise returns an error so the daemon log surfaces a misconfig.
func startScheduler(ctx context.Context, opts Options) error {
	cfg := config.Active.Notifications

	// Always start fresh — the daemon only registers what's configured.
	notify.Reset()

	registered := 0
	if cfg.Backends.System.Enabled {
		notify.Register(notify.NewSystem(notify.SystemConfig{
			BinaryPath: cfg.Backends.System.BinaryPath,
			Urgency:    cfg.Backends.System.Urgency,
		}))
		log.Printf("serve: notify backend registered — system")
		registered++
	}
	if cfg.Backends.Ntfy.Enabled {
		notify.Register(notify.NewNtfy(notify.NtfyConfig{
			URL:   cfg.Backends.Ntfy.URL,
			Topic: cfg.Backends.Ntfy.Topic,
		}))
		log.Printf("serve: notify backend registered — ntfy → %s/%s",
			notify.RedactURL(cfg.Backends.Ntfy.URL), cfg.Backends.Ntfy.Topic)
		registered++
	}
	if registered == 0 {
		return fmt.Errorf("no backends enabled — set notifications.backends.system.enabled or .ntfy.enabled")
	}

	var runner *actions.Runner
	if cfg.Actions.Enabled {
		path := cfg.Actions.ConfigPath
		if path == "" {
			p, err := actions.Path()
			if err != nil {
				return fmt.Errorf("resolve actions path: %w", err)
			}
			path = p
		}
		acfg, err := actions.Load(path)
		if err != nil {
			return fmt.Errorf("load actions: %w", err)
		}
		runner = actions.NewRunner(acfg, cfg.MaxConcurrentSpawns)
		log.Printf("serve: actions runner loaded — %d action(s) from %s", len(runner.Names()), path)
	}

	sc := notify.NewScheduler(opts.Store, runner, notify.SchedulerConfig{
		TickInterval:        cfg.TickInterval.AsDuration(),
		MaxLead:             cfg.MaxLead.AsDuration(),
		Calendars:           cfg.Calendars,
		MaxConcurrentSpawns: cfg.MaxConcurrentSpawns,
	})
	go sc.Run(ctx)
	log.Printf("serve: notify scheduler started")
	return nil
}
