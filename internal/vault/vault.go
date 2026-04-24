// Package vault resolves the vault root and produces paths inside it.
package vault

import (
	"fmt"
	"os"
	"path/filepath"
)

type Vault struct {
	Root string
}

// EnvVar is the environment variable holding the vault path.
const EnvVar = "CALEMDAR_VAULT"

// Resolve returns the vault. Lookup order:
//  1. override (typically a CLI flag)
//  2. $CALEMDAR_VAULT
//
// Fails if neither is set or the resolved path is not an existing directory.
func Resolve(override string) (*Vault, error) {
	p := override
	if p == "" {
		p = os.Getenv(EnvVar)
	}
	if p == "" {
		return nil, fmt.Errorf("vault path not set: use --vault or $%s", EnvVar)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("vault %q: %w", abs, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("vault %q: not a directory", abs)
	}
	return &Vault{Root: abs}, nil
}

func (v *Vault) RecurringDir() string { return filepath.Join(v.Root, "recurring") }
func (v *Vault) EventsDir() string    { return filepath.Join(v.Root, "events") }
func (v *Vault) ArchiveDir() string   { return filepath.Join(v.Root, "archive") }

// EventPath returns the canonical path for an expanded event.
// dateStr is YYYY-MM-DD. year is derived from the first four chars.
func (v *Vault) EventPath(calendar, dateStr, slug string) string {
	year := dateStr[:4]
	return filepath.Join(v.EventsDir(), calendar, year, dateStr+"-"+slug+".md")
}
