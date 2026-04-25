package serve

import (
	"log"
	"time"

	"github.com/arch-err/calemdar/internal/archive"
	"github.com/arch-err/calemdar/internal/config"
	"github.com/arch-err/calemdar/internal/reconcile"
	"github.com/arch-err/calemdar/internal/series"
)

// runNightly extends the 12-month horizon for every series (by re-running
// reconcile, which will add new occurrences to events within window) and
// archives events older than the cutoff.
func runNightly(opts Options) {
	log.Printf("serve: nightly run starting")

	roots, err := series.LoadAll(opts.Vault)
	if err != nil {
		log.Printf("serve: nightly LoadAll: %v", err)
		return
	}
	for _, r := range roots {
		rep, err := reconcile.Series(opts.Vault, r)
		if err != nil {
			log.Printf("serve: nightly reconcile %s: %v", r.Slug, err)
			continue
		}
		log.Printf("serve: nightly %s — in-plan=%d created=%d updated=%d skipped=%d swept=%d",
			r.Slug, rep.InPlan, rep.Created, rep.Updated, rep.Skipped, rep.Swept)
		if err := opts.Store.UpsertSeries(r); err != nil {
			log.Printf("serve: nightly store upsert: %v", err)
		}
	}

	arep, err := archive.Run(opts.Vault)
	if err != nil {
		log.Printf("serve: nightly archive: %v", err)
	} else {
		log.Printf("serve: nightly archived %d events", arep.Moved)
	}

	// Prune the notify_fired dedupe table. Only meaningful when notifs
	// are enabled; running it unconditionally is harmless when the
	// table is empty.
	if config.Active.Notifications.Enabled {
		cutoff := time.Now().Add(-14 * 24 * time.Hour)
		n, err := opts.Store.PruneFired(cutoff)
		if err != nil {
			log.Printf("serve: nightly prune notify_fired: %v", err)
		} else if n > 0 {
			log.Printf("serve: nightly pruned %d notify_fired rows", n)
		}
	}
}
