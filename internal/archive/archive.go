// Package archive moves events older than the cutoff (6 months) from
// events/<calendar>/<year>/ to archive/<year>/<calendar>/.
package archive

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/arch-err/calemdar/internal/config"
	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/vault"
	"github.com/arch-err/calemdar/internal/writer"
)

// Report summarises an archive run.
type Report struct {
	Moved int
	Paths []string // relative to vault root
}

// Run moves old events to archive/. Cutoff is configured via
// config.Active.ArchiveCutoffMonths, today in the configured timezone.
func Run(v *vault.Vault) (*Report, error) {
	loc := model.Location()
	cutoff := model.Today(loc).AddDate(0, -config.Active.ArchiveCutoffMonths, 0)
	return RunWithCutoff(v, cutoff)
}

// RunWithCutoff is Run but with an explicit cutoff date (for testing).
// Events with date < cutoff are moved.
func RunWithCutoff(v *vault.Vault, cutoff time.Time) (*Report, error) {
	rep := &Report{}
	eventsDir := v.EventsDir()
	if _, err := os.Stat(eventsDir); errors.Is(err, os.ErrNotExist) {
		return rep, nil
	}

	err := filepath.WalkDir(eventsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		e, err := model.ParseEvent(path)
		if err != nil {
			return fmt.Errorf("archive: parse %s: %w", path, err)
		}
		eDate, err := model.ParseDate(e.Date, cutoff.Location())
		if err != nil {
			return fmt.Errorf("archive: bad date in %s: %w", path, err)
		}
		if !eDate.Before(cutoff) {
			return nil
		}

		calendar := calendarFromPath(v, path)
		year := e.Date[:4]
		targetDir := filepath.Join(v.ArchiveDir(), year, calendar)
		target := filepath.Join(targetDir, filepath.Base(path))

		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			return err
		}
		// Mark the source as a self-delete BEFORE the rename — Rename
		// erases the inode from the source's POV, so a post-syscall
		// NotifySelf can only flag it as a removal anyway. Doing it
		// pre-syscall makes the intent explicit. The target lives under
		// archive/, which the watcher doesn't track, so no notification
		// is needed there.
		writer.NotifySelfDelete(path)
		if err := os.Rename(path, target); err != nil {
			return fmt.Errorf("archive: move %s: %w", path, err)
		}
		rel, _ := filepath.Rel(v.Root, target)
		rep.Paths = append(rep.Paths, rel)
		rep.Moved++
		return nil
	})
	return rep, err
}

// calendarFromPath returns the first path component under events/.
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
