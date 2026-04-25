// Package reactor scans the vault's events/ tree for Full Calendar-authored
// recurring events (type: recurring | rrule), translates them into calemdar
// Root files, deletes the originals, and reconciles the new series.
package reactor

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/arch-err/calemdar/internal/fcparse"
	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/reconcile"
	"github.com/arch-err/calemdar/internal/vault"
	"github.com/arch-err/calemdar/internal/writer"
)

// Migration describes a single FC → Root conversion.
type Migration struct {
	FromPath string
	ToPath   string
	Series   *model.Root
	Report   *reconcile.Report
}

// Run scans events/ and migrates every type: recurring / type: rrule event
// found. Returns one Migration per converted file.
func Run(v *vault.Vault) ([]*Migration, error) {
	root := v.EventsDir()
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	var out []*Migration
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		kind, err := fcparse.Detect(path)
		if err != nil {
			return fmt.Errorf("detect %s: %w", path, err)
		}
		if kind != fcparse.TypeRecurring && kind != fcparse.TypeRRule {
			return nil
		}

		m, err := migrateOne(v, path, kind)
		if err != nil {
			return fmt.Errorf("migrate %s: %w", path, err)
		}
		out = append(out, m)
		return nil
	})
	if err != nil {
		return out, err
	}
	return out, nil
}

func migrateOne(v *vault.Vault, path, kind string) (*Migration, error) {
	cal, err := calendarFromPath(v, path)
	if err != nil {
		return nil, err
	}
	if !model.ValidCalendar(cal) {
		return nil, fmt.Errorf("calendar %q from path is not a v1 calendar", cal)
	}

	var r *model.Root
	switch kind {
	case fcparse.TypeRecurring:
		fc, err := fcparse.ReadRecurring(path)
		if err != nil {
			return nil, err
		}
		r, err = fcparse.TranslateRecurring(fc, cal)
		if err != nil {
			return nil, err
		}
		r.Slug = fcparse.Slugify(fc.Title)
	case fcparse.TypeRRule:
		fc, err := fcparse.ReadRRule(path)
		if err != nil {
			return nil, err
		}
		r, err = fcparse.TranslateRRule(fc, cal)
		if err != nil {
			return nil, err
		}
		r.Slug = fcparse.Slugify(fc.Title)
	default:
		return nil, fmt.Errorf("unexpected kind %q", kind)
	}
	if r.Slug == "" {
		return nil, fmt.Errorf("title %q slugifies to empty", r.Title)
	}

	// Preserve the body, trimming the leading blank left by FC's writer.
	body, err := fcparse.ReadBody(path)
	if err != nil {
		return nil, err
	}
	r.Body = strings.TrimLeft(body, "\n")

	targetPath := filepath.Join(v.RecurringDir(), r.Slug+".md")
	if _, err := os.Stat(targetPath); err == nil {
		return nil, fmt.Errorf("slug collision: %s already exists (rename the source event or edit the slug)", targetPath)
	}
	r.Path = targetPath

	if err := writer.WriteRoot(r); err != nil {
		return nil, err
	}
	// Mark as self-delete BEFORE the syscall — once the inode is gone
	// there's nothing for a post-syscall stat to suppress against.
	writer.NotifySelfDelete(path)
	if err := os.Remove(path); err != nil {
		return nil, fmt.Errorf("remove original %s: %w", path, err)
	}

	rep, err := reconcile.Series(v, r, nil)
	if err != nil {
		return nil, err
	}

	return &Migration{
		FromPath: path,
		ToPath:   targetPath,
		Series:   r,
		Report:   rep,
	}, nil
}

// calendarFromPath returns the first path component under events/.
// e.g. <vault>/events/health/Workout.md → "health".
func calendarFromPath(v *vault.Vault, path string) (string, error) {
	rel, err := filepath.Rel(v.EventsDir(), path)
	if err != nil {
		return "", err
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("event not inside a calendar folder: %s", path)
	}
	return parts[0], nil
}
