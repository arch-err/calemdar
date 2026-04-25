// Package backup mirrors recurring root files to a laptop-local backup
// directory before they're deleted. The directory lives under
// <vault>/.calemdar/backup/recurring/ — the same .calemdar/ that holds
// the sqlite cache, which is intentionally NOT synced across devices.
//
// Backup file naming: <slug>-<RFC3339-utc>.md. Colons in the timestamp
// are replaced with dashes so the filename is portable.
package backup

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/arch-err/calemdar/internal/vault"
)

// SubDir is the path under <vault>/.calemdar/ where recurring backups live.
const SubDir = ".calemdar/backup/recurring"

// Dir returns the absolute backup directory for the given vault.
func Dir(v *vault.Vault) string {
	return filepath.Join(v.Root, SubDir)
}

// Entry is one backup file.
type Entry struct {
	Slug     string
	When     time.Time
	Path     string
	Filename string
}

// stampFormat replaces colons with dashes so the filename is safe on
// every fs we'll ever care about. ParseStamp inverts this.
const stampFormat = "2006-01-02T15-04-05Z"

func formatStamp(t time.Time) string {
	return t.UTC().Format(stampFormat)
}

func parseStamp(s string) (time.Time, error) {
	return time.Parse(stampFormat, s)
}

// Filename returns the canonical backup filename for slug at t.
func Filename(slug string, t time.Time) string {
	return slug + "-" + formatStamp(t) + ".md"
}

// WriteFromBytes writes content to <vault>/.calemdar/backup/recurring/<slug>-<ts>.md
// and returns the absolute path it landed at.
func WriteFromBytes(v *vault.Vault, slug string, content []byte, when time.Time) (string, error) {
	if slug == "" {
		return "", fmt.Errorf("backup: empty slug")
	}
	dir := Dir(v)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("backup: mkdir: %w", err)
	}
	target := filepath.Join(dir, Filename(slug, when))
	if err := os.WriteFile(target, content, 0o644); err != nil {
		return "", fmt.Errorf("backup: write: %w", err)
	}
	return target, nil
}

// WriteFromFile copies srcPath into the backup dir under slug. Used when
// the original file still exists on disk (external delete is *about* to
// happen and we caught the precursor; or a CLI is preserving an
// about-to-be-removed root).
func WriteFromFile(v *vault.Vault, slug, srcPath string, when time.Time) (string, error) {
	raw, err := os.ReadFile(srcPath)
	if err != nil {
		return "", fmt.Errorf("backup: read source: %w", err)
	}
	return WriteFromBytes(v, slug, raw, when)
}

// List returns every backup currently on disk, sorted newest-first.
func List(v *vault.Vault) ([]Entry, error) {
	dir := Dir(v)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []Entry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		ent, ok := parseEntry(dir, e.Name())
		if !ok {
			continue
		}
		out = append(out, ent)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Slug != out[j].Slug {
			return out[i].Slug < out[j].Slug
		}
		return out[i].When.After(out[j].When)
	})
	return out, nil
}

// LatestForSlug returns the most recent backup matching slug, or nil if
// none exists.
func LatestForSlug(v *vault.Vault, slug string) (*Entry, error) {
	all, err := List(v)
	if err != nil {
		return nil, err
	}
	var best *Entry
	for i := range all {
		if all[i].Slug != slug {
			continue
		}
		if best == nil || all[i].When.After(best.When) {
			b := all[i]
			best = &b
		}
	}
	return best, nil
}

// parseEntry expects a filename of the form <slug>-<RFC3339-utc>.md
// where the timestamp uses dashes instead of colons. Returns ok=false on
// any parse miss; caller silently skips.
func parseEntry(dir, name string) (Entry, bool) {
	base := strings.TrimSuffix(name, ".md")
	// The stamp is fixed-length; split off the trailing 20 chars (the
	// stamp) and treat everything before "-" as the slug.
	const stampLen = len("2006-01-02T15-04-05Z")
	if len(base) < stampLen+2 {
		return Entry{}, false
	}
	stampStart := len(base) - stampLen
	if base[stampStart-1] != '-' {
		return Entry{}, false
	}
	slug := base[:stampStart-1]
	when, err := parseStamp(base[stampStart:])
	if err != nil {
		return Entry{}, false
	}
	return Entry{
		Slug:     slug,
		When:     when,
		Path:     filepath.Join(dir, name),
		Filename: name,
	}, true
}
