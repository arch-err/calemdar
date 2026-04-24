// Package series loads recurring roots from the vault.
package series

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/vault"
)

// LoadAll returns every recurring root in the vault's recurring/ directory.
// Returns an empty slice (not an error) if the directory doesn't exist.
func LoadAll(v *vault.Vault) ([]*model.Root, error) {
	var roots []*model.Root
	dir := v.RecurringDir()

	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		r, err := model.ParseRoot(path)
		if err != nil {
			return fmt.Errorf("load series: %w", err)
		}
		roots = append(roots, r)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return roots, nil
}

// FindByIDOrSlug returns the first matching root. Matches against both ID
// and Slug. Returns nil, nil if no match.
func FindByIDOrSlug(v *vault.Vault, q string) (*model.Root, error) {
	roots, err := LoadAll(v)
	if err != nil {
		return nil, err
	}
	for _, r := range roots {
		if r.ID == q || r.Slug == q {
			return r, nil
		}
	}
	return nil, nil
}

// LoadEventsForSeries returns all expanded events on disk whose series-id
// matches r.ID. Scans only events/<r.Calendar>/ since we never write a
// series' events outside its calendar folder.
func LoadEventsForSeries(v *vault.Vault, r *model.Root) ([]*model.Event, error) {
	dir := filepath.Join(v.EventsDir(), r.Calendar)
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	var out []*model.Event
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		e, err := model.ParseEvent(path)
		if err != nil {
			return fmt.Errorf("load events: %w", err)
		}
		if e.SeriesID == r.ID {
			out = append(out, e)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
