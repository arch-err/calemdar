package serve

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
	"github.com/arch-err/calemdar/internal/watch"
)

// setup creates a tempdir vault scaffold + open store. Returns both.
func setup(t *testing.T) (*vault.Vault, *store.Store) {
	t.Helper()
	root := t.TempDir()
	v := &vault.Vault{Root: root}
	for _, sub := range []string{"recurring", "events/health", "archive", ".calemdar"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	config.Active = config.Defaults()

	s, err := store.Open(v)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return v, s
}

// snapshotRoot writes a recurring root file and seeds the snapshot in
// sqlite. Mirrors what dispatchRecurring(Changed) would do, but without
// requiring a full reconcile.
func snapshotRoot(t *testing.T, v *vault.Vault, s *store.Store, slug string, content string) *model.Root {
	t.Helper()
	target := filepath.Join(v.RecurringDir(), slug+".md")
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := model.ParseRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertSeries(r); err != nil {
		t.Fatal(err)
	}
	return r
}

const sampleRoot = `---
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

body content
`

func TestExternalDeleteTriggersAutoRestore(t *testing.T) {
	v, s := setup(t)
	r := snapshotRoot(t, v, s, "workout", sampleRoot)

	// Confirm sqlite captured the snapshot.
	got, err := s.GetSeriesByPath(r.Path)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.RawSource == "" {
		t.Fatal("no snapshot in sqlite")
	}

	// Simulate an external delete.
	if err := os.Remove(r.Path); err != nil {
		t.Fatal(err)
	}

	// Hand the watcher Event to dispatch.
	ev := watch.Event{Kind: watch.Deleted, Source: watch.SourceRecurring, Path: r.Path}
	if err := dispatch(Options{Vault: v, Store: s}, ev); err != nil {
		t.Fatalf("dispatch returned error: %v", err)
	}

	// File restored.
	restored, err := os.ReadFile(r.Path)
	if err != nil {
		t.Fatalf("file not restored: %v", err)
	}
	if string(restored) != sampleRoot {
		t.Errorf("restored contents differ:\nwant: %q\ngot:  %q", sampleRoot, restored)
	}

	// Backup mirror exists.
	all, err := backup.List(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].Slug != "workout" {
		t.Fatalf("expected exactly 1 backup for workout, got %+v", all)
	}
	bk, err := os.ReadFile(all[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(bk) != sampleRoot {
		t.Errorf("backup contents differ:\nwant: %q\ngot:  %q", sampleRoot, bk)
	}
}

func TestExternalDeleteWithoutSnapshotIsNoop(t *testing.T) {
	v, s := setup(t)
	// File on disk but no sqlite row at all.
	target := filepath.Join(v.RecurringDir(), "ghost.md")
	if err := os.WriteFile(target, []byte(sampleRoot), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}

	ev := watch.Event{Kind: watch.Deleted, Source: watch.SourceRecurring, Path: target}
	if err := dispatch(Options{Vault: v, Store: s}, ev); err != nil {
		t.Fatalf("dispatch returned error: %v", err)
	}

	// File still gone.
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("expected file to remain deleted, got err=%v", err)
	}
	// No backup written.
	all, _ := backup.List(v)
	if len(all) != 0 {
		t.Errorf("expected no backup, got %+v", all)
	}
}

func TestRestoreSurfacesBodyVerbatim(t *testing.T) {
	// The whole file is the snapshot, including frontmatter quirks.
	v, s := setup(t)
	weird := strings.Replace(sampleRoot, "body content", "body with a # heading\n\n[[wikilink]]", 1)
	r := snapshotRoot(t, v, s, "workout", weird)

	if err := os.Remove(r.Path); err != nil {
		t.Fatal(err)
	}
	ev := watch.Event{Kind: watch.Deleted, Source: watch.SourceRecurring, Path: r.Path}
	if err := dispatch(Options{Vault: v, Store: s}, ev); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(r.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != weird {
		t.Errorf("restore not byte-exact:\nwant: %q\ngot:  %q", weird, got)
	}
}
