package archive

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/vault"
	"github.com/arch-err/calemdar/internal/writer"
)

func TestArchiveMovesOldEventsOnly(t *testing.T) {
	vaultRoot := t.TempDir()
	v := &vault.Vault{Root: vaultRoot}

	// Create one old event + one fresh event.
	old := &model.Event{
		Title: "Old", Date: "2025-10-01", AllDay: true, Type: "single",
		UserOwned: true,
		Body:      "",
		Path:      v.EventPath("health", "2025-10-01", "old"),
	}
	fresh := &model.Event{
		Title: "Fresh", Date: "2026-04-20", AllDay: true, Type: "single",
		UserOwned: true,
		Body:      "",
		Path:      v.EventPath("health", "2026-04-20", "fresh"),
	}
	if err := writer.WriteEvent(old); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteEvent(fresh); err != nil {
		t.Fatal(err)
	}

	// Cutoff: 2026-01-01. "Old" moves, "Fresh" stays.
	cutoff := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rep, err := RunWithCutoff(v, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Moved != 1 {
		t.Errorf("Moved = %d, want 1", rep.Moved)
	}

	// Old should be in archive/2025/health/, not in events/.
	archived := filepath.Join(vaultRoot, "archive", "2025", "health", "2025-10-01-old.md")
	if _, err := os.Stat(archived); err != nil {
		t.Errorf("archived file missing: %v", err)
	}
	if _, err := os.Stat(old.Path); !os.IsNotExist(err) {
		t.Errorf("old event still in events/: %v", err)
	}

	// Fresh should still be in events/.
	if _, err := os.Stat(fresh.Path); err != nil {
		t.Errorf("fresh event missing: %v", err)
	}
}

func TestArchiveIdempotent(t *testing.T) {
	vaultRoot := t.TempDir()
	v := &vault.Vault{Root: vaultRoot}
	old := &model.Event{
		Title: "Old", Date: "2025-01-01", AllDay: true, Type: "single",
		Body: "", Path: v.EventPath("health", "2025-01-01", "old"),
	}
	if err := writer.WriteEvent(old); err != nil {
		t.Fatal(err)
	}

	cutoff := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := RunWithCutoff(v, cutoff); err != nil {
		t.Fatal(err)
	}
	// Second run: nothing to move, no error.
	rep, err := RunWithCutoff(v, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Moved != 0 {
		t.Errorf("second run Moved = %d, want 0", rep.Moved)
	}
}
