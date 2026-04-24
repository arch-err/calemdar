package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/series"
	"github.com/arch-err/calemdar/internal/vault"
)

// ReindexReport summarises a Reindex run.
type ReindexReport struct {
	Series      int
	Occurrences int
}

// Reindex wipes the cache and repopulates it from the vault's recurring/ and
// events/ trees. Idempotent.
func (s *Store) Reindex(v *vault.Vault) (*ReindexReport, error) {
	if err := s.Wipe(); err != nil {
		return nil, err
	}

	roots, err := series.LoadAll(v)
	if err != nil {
		return nil, err
	}
	for _, r := range roots {
		if err := s.UpsertSeries(r); err != nil {
			return nil, fmt.Errorf("reindex: upsert series %s: %w", r.Slug, err)
		}
	}

	rep := &ReindexReport{Series: len(roots)}

	eventsDir := v.EventsDir()
	if _, err := os.Stat(eventsDir); errors.Is(err, os.ErrNotExist) {
		return rep, nil
	}

	err = filepath.WalkDir(eventsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		e, perr := model.ParseEvent(path)
		if perr != nil {
			return fmt.Errorf("reindex: parse %s: %w", path, perr)
		}
		calendar := calendarFromPath(v, path)
		if err := s.UpsertOccurrence(e, calendar); err != nil {
			return fmt.Errorf("reindex: upsert %s: %w", path, err)
		}
		rep.Occurrences++
		return nil
	})
	return rep, err
}

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
