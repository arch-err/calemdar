package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/arch-err/calemdar/internal/backup"
	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/series"
	"github.com/arch-err/calemdar/internal/store"
	"github.com/arch-err/calemdar/internal/writer"
	"github.com/spf13/cobra"
)

var recurringCmd = &cobra.Command{
	Use:   "recurring",
	Short: "Recurring root deletion safeguards (delete / restore / backup-list)",
}

var recurringDeleteCmd = &cobra.Command{
	Use:   "delete <id-or-slug>",
	Short: "Delete a recurring root with proper bookkeeping (snapshot, optional event purge)",
	Args:  cobra.ExactArgs(1),
	RunE:  runRecurringDelete,
}

var recurringRestoreCmd = &cobra.Command{
	Use:   "restore <slug>",
	Short: "Restore the most recent backup for slug into recurring/<slug>.md",
	Args:  cobra.ExactArgs(1),
	RunE:  runRecurringRestore,
}

var recurringBackupListCmd = &cobra.Command{
	Use:   "backup-list",
	Short: "List recurring-root backups, grouped by slug",
	RunE:  runRecurringBackupList,
}

func init() {
	recurringDeleteCmd.Flags().Bool("purge-events", false,
		"also delete future non-user-owned expanded events for this series")
	recurringCmd.AddCommand(recurringDeleteCmd, recurringRestoreCmd, recurringBackupListCmd)
}

// runRecurringDelete is the safe-delete CLI. Order:
//  1. Resolve series via series.FindByIDOrSlug.
//  2. Drop a backup copy of the current root file.
//  3. Optionally cascade-delete future non-user-owned events on disk.
//  4. Mark the root path with the self-delete flag and remove it.
//  5. Clean sqlite (DeleteSeries; per-purged-path DeleteOccurrence).
//
// The self-delete flag prevents the watcher's auto-restore from kicking
// in. Sqlite cleanup runs after the file delete so that — if the daemon
// crashes between steps — a reindex can rebuild from disk truth.
func runRecurringDelete(cmd *cobra.Command, args []string) error {
	v, err := resolveVault(cmd)
	if err != nil {
		return err
	}
	purgeEvents, _ := cmd.Flags().GetBool("purge-events")

	r, err := series.FindByIDOrSlug(v, args[0])
	if err != nil {
		return err
	}
	if r == nil {
		return fmt.Errorf("series %q not found", args[0])
	}

	// Snapshot the current root before we touch anything.
	if _, berr := backup.WriteFromFile(v, r.Slug, r.Path, time.Now()); berr != nil {
		// Don't fail the whole delete on backup failure — the sqlite
		// snapshot already covers the recovery case. Surface it loudly.
		fmt.Fprintf(os.Stderr, "warn: backup of %s failed: %v\n", r.Path, berr)
	}

	loc := model.Location()
	today := model.Today(loc)

	purged := 0
	preservedUserOwned := 0
	purgedPaths := []string{}
	if purgeEvents {
		existing, err := series.LoadEventsForSeries(v, r)
		if err != nil {
			return err
		}
		for _, e := range existing {
			d, err := model.ParseDate(e.Date, loc)
			if err != nil {
				continue
			}
			if d.Before(today) {
				// Past stays put — archive-bound, not ours to wipe.
				continue
			}
			if e.UserOwned {
				preservedUserOwned++
				continue
			}
			writer.NotifySelfDelete(e.Path)
			if err := os.Remove(e.Path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("purge event %s: %w", e.Path, err)
			}
			purgedPaths = append(purgedPaths, e.Path)
			purged++
		}
	}

	// File delete: heads-up the watcher first so the daemon (if running)
	// doesn't auto-restore.
	writer.NotifySelfDelete(r.Path)
	if err := os.Remove(r.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove root: %w", err)
	}

	// Sqlite cleanup: drop the series row + any occurrence rows we just
	// removed from disk. If a fresh reindex runs, it'll match this state.
	s, err := store.Open(v)
	if err != nil {
		// Non-fatal — file delete already succeeded; user can `reindex`.
		fmt.Fprintf(os.Stderr, "warn: store open failed: %v (run `calemdar reindex`)\n", err)
	} else {
		defer s.Close()
		if err := s.DeleteSeries(r.ID); err != nil {
			fmt.Fprintf(os.Stderr, "warn: store delete series: %v\n", err)
		}
		for _, p := range purgedPaths {
			if err := s.DeleteOccurrence(p); err != nil {
				fmt.Fprintf(os.Stderr, "warn: store delete occurrence %s: %v\n", p, err)
			}
		}
	}

	fmt.Printf("deleted recurring root %s (id %s)\n", r.Slug, r.ID)
	if purgeEvents {
		fmt.Printf("purged %d future events; preserved %d user-owned\n", purged, preservedUserOwned)
	}
	return nil
}

// runRecurringRestore copies the most recent backup file matching slug
// back to <vault>/recurring/<slug>.md. Refuses to overwrite an existing
// file — the user must move it out of the way first (or use `recurring
// delete <slug>` if they really want to replace it).
func runRecurringRestore(cmd *cobra.Command, args []string) error {
	v, err := resolveVault(cmd)
	if err != nil {
		return err
	}
	slug := args[0]

	target := filepath.Join(v.RecurringDir(), slug+".md")
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("recurring root already exists at %s — refusing to overwrite", target)
	}

	entry, err := backup.LatestForSlug(v, slug)
	if err != nil {
		return err
	}
	if entry == nil {
		return fmt.Errorf("no backup found for slug %q (looked in %s)", slug, backup.Dir(v))
	}

	raw, err := os.ReadFile(entry.Path)
	if err != nil {
		return fmt.Errorf("read backup: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(target, raw, 0o644); err != nil {
		return err
	}
	fmt.Printf("restored %s ← %s\n", target, entry.Path)
	fmt.Printf("(daemon will reconcile on the next watcher tick; run `calemdar reindex` if standalone)\n")
	return nil
}

// runRecurringBackupList prints every backup grouped by slug, newest first.
func runRecurringBackupList(cmd *cobra.Command, args []string) error {
	v, err := resolveVault(cmd)
	if err != nil {
		return err
	}
	all, err := backup.List(v)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		fmt.Printf("no backups in %s\n", backup.Dir(v))
		return nil
	}

	bySlug := map[string][]backup.Entry{}
	var slugs []string
	for _, e := range all {
		if _, ok := bySlug[e.Slug]; !ok {
			slugs = append(slugs, e.Slug)
		}
		bySlug[e.Slug] = append(bySlug[e.Slug], e)
	}
	sort.Strings(slugs)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SLUG\tWHEN (UTC)\tFILE")
	for _, slug := range slugs {
		// Already sorted newest-first within slug by backup.List.
		for _, e := range bySlug[slug] {
			fmt.Fprintf(w, "%s\t%s\t%s\n",
				slug, e.When.UTC().Format(time.RFC3339), e.Filename)
		}
	}
	return w.Flush()
}
