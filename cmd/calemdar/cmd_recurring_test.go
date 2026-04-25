package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arch-err/calemdar/internal/backup"
	"github.com/arch-err/calemdar/internal/config"
	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/store"
	"github.com/arch-err/calemdar/internal/vault"
	"github.com/arch-err/calemdar/internal/writer"
	"github.com/spf13/cobra"
)

// fixture builds a vault scaffold with one recurring root + a few expanded
// events: one past, one future-non-user-owned, one future-user-owned.
type fixture struct {
	v        *vault.Vault
	rootPath string
	pastPath string
	futPath  string
	usrPath  string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	v := &vault.Vault{Root: root}
	for _, sub := range []string{"recurring", "events/health", "archive", ".calemdar"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	config.Active = config.Defaults()

	// Wire env so resolveVault picks up our path.
	t.Setenv(vault.EnvVar, root)

	// Stable root so series.FindByIDOrSlug works.
	rootContent := `---
id: 11111111-1111-7111-8111-111111111111
calendar: health
title: Workout
start-date: 2026-05-01
all-day: false
freq: weekly
interval: 1
byday: [mon]
start-time: "10:00"
end-time: "11:00"
---

body
`
	rootPath := filepath.Join(v.RecurringDir(), "workout.md")
	if err := os.WriteFile(rootPath, []byte(rootContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Seed sqlite snapshot.
	s, err := store.Open(v)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r, err := model.ParseRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertSeries(r); err != nil {
		t.Fatal(err)
	}

	loc := model.Location()
	today := model.Today(loc)
	past := today.AddDate(0, 0, -7).Format("2006-01-02")
	future := today.AddDate(0, 0, 7).Format("2006-01-02")
	userOwnedDate := today.AddDate(0, 0, 14).Format("2006-01-02")

	// Disable self-write notifier — no daemon attached during tests.
	writer.SelfWriteNotifier = nil
	writer.SelfDeleteNotifier = nil

	mkEvent := func(date string, userOwned bool) string {
		e := &model.Event{
			Title:     "Workout",
			Date:      date,
			StartTime: "10:00",
			EndTime:   "11:00",
			AllDay:    false,
			Type:      "single",
			SeriesID:  r.ID,
			UserOwned: userOwned,
			Path:      v.EventPath("health", date, "workout"),
			Body:      "[[workout]]\n",
		}
		if err := writer.WriteEvent(e); err != nil {
			t.Fatal(err)
		}
		return e.Path
	}
	pastPath := mkEvent(past, false)
	futPath := mkEvent(future, false)
	usrPath := mkEvent(userOwnedDate, true)

	return &fixture{v: v, rootPath: rootPath, pastPath: pastPath, futPath: futPath, usrPath: usrPath}
}

func TestRecurringDeleteWithPurgeEvents(t *testing.T) {
	f := newFixture(t)

	cmd := &cobra.Command{}
	cmd.PersistentFlags().String("vault", "", "")
	cmd.Flags().Bool("purge-events", true, "")
	if err := runRecurringDelete(cmd, []string{"workout"}); err != nil {
		t.Fatalf("runRecurringDelete: %v", err)
	}

	// Root file: gone.
	if _, err := os.Stat(f.rootPath); !os.IsNotExist(err) {
		t.Errorf("root still present: %v", err)
	}
	// Past event: untouched.
	if _, err := os.Stat(f.pastPath); err != nil {
		t.Errorf("past event removed (should stay archive-bound): %v", err)
	}
	// Future non-user-owned: gone.
	if _, err := os.Stat(f.futPath); !os.IsNotExist(err) {
		t.Errorf("future non-user-owned event still present: %v", err)
	}
	// Future user-owned: untouched.
	if _, err := os.Stat(f.usrPath); err != nil {
		t.Errorf("user-owned event removed (must be preserved): %v", err)
	}
	// Backup written.
	all, err := backup.List(f.v)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Slug != "workout" {
		t.Errorf("expected one workout backup, got %+v", all)
	}
}

func TestRecurringDeleteWithoutPurgeEventsLeavesEvents(t *testing.T) {
	f := newFixture(t)

	cmd := &cobra.Command{}
	cmd.PersistentFlags().String("vault", "", "")
	cmd.Flags().Bool("purge-events", false, "")
	if err := runRecurringDelete(cmd, []string{"workout"}); err != nil {
		t.Fatal(err)
	}

	// Root: gone.
	if _, err := os.Stat(f.rootPath); !os.IsNotExist(err) {
		t.Errorf("root still present: %v", err)
	}
	// All events still on disk.
	for _, p := range []string{f.pastPath, f.futPath, f.usrPath} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("event removed without --purge-events: %s", p)
		}
	}
}

func TestRecurringRestoreFromBackup(t *testing.T) {
	f := newFixture(t)

	// Delete via CLI (creates a backup).
	cmd := &cobra.Command{}
	cmd.PersistentFlags().String("vault", "", "")
	cmd.Flags().Bool("purge-events", false, "")
	if err := runRecurringDelete(cmd, []string{"workout"}); err != nil {
		t.Fatal(err)
	}

	// Restore.
	rcmd := &cobra.Command{}
	rcmd.PersistentFlags().String("vault", "", "")
	if err := runRecurringRestore(rcmd, []string{"workout"}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, err := os.ReadFile(f.rootPath)
	if err != nil {
		t.Fatalf("restored file missing: %v", err)
	}
	if !strings.Contains(string(got), "title: Workout") {
		t.Errorf("restored content lost frontmatter: %q", got)
	}
}

func TestRecurringRestoreRefusesIfRootExists(t *testing.T) {
	f := newFixture(t)

	// Drop a backup so the lookup itself succeeds.
	if _, err := backup.WriteFromFile(f.v, "workout", f.rootPath, model.Today(model.Location())); err != nil {
		t.Fatal(err)
	}

	rcmd := &cobra.Command{}
	rcmd.PersistentFlags().String("vault", "", "")
	err := runRecurringRestore(rcmd, []string{"workout"})
	if err == nil {
		t.Fatal("expected error when root already exists, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error didn't mention existing file: %v", err)
	}
}

func TestRecurringRestoreNoBackup(t *testing.T) {
	f := newFixture(t)
	// Remove root so the existence check passes.
	if err := os.Remove(f.rootPath); err != nil {
		t.Fatal(err)
	}

	rcmd := &cobra.Command{}
	rcmd.PersistentFlags().String("vault", "", "")
	err := runRecurringRestore(rcmd, []string{"workout"})
	if err == nil {
		t.Fatal("expected error when no backup exists")
	}
}

func TestBackupListAndRestoreRoundTrip(t *testing.T) {
	f := newFixture(t)

	cmd := &cobra.Command{}
	cmd.PersistentFlags().String("vault", "", "")
	cmd.Flags().Bool("purge-events", false, "")
	if err := runRecurringDelete(cmd, []string{"workout"}); err != nil {
		t.Fatal(err)
	}

	all, err := backup.List(f.v)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(all))
	}
	if all[0].Slug != "workout" {
		t.Errorf("slug = %q", all[0].Slug)
	}

	// Restore round-trips to the same content.
	rcmd := &cobra.Command{}
	rcmd.PersistentFlags().String("vault", "", "")
	if err := runRecurringRestore(rcmd, []string{"workout"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f.rootPath); err != nil {
		t.Errorf("restored file missing: %v", err)
	}
}
