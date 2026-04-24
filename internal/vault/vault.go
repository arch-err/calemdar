// Package vault resolves the vault root and produces paths inside it.
package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Vault struct {
	Root     string
	BasePath string // subfolder inside Root; "" means at root
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
	// Expand leading ~ / ~user — filepath.Abs doesn't do this and users put
	// it in the config file regardless.
	if expanded, err := expandTilde(p); err == nil {
		p = expanded
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

// BaseDir is the directory under which recurring/, events/, archive/ live.
// Equal to Root when BasePath is empty.
func (v *Vault) BaseDir() string      { return filepath.Join(v.Root, v.BasePath) }
func (v *Vault) RecurringDir() string { return filepath.Join(v.BaseDir(), "recurring") }
func (v *Vault) EventsDir() string    { return filepath.Join(v.BaseDir(), "events") }
func (v *Vault) ArchiveDir() string   { return filepath.Join(v.BaseDir(), "archive") }

// EventPath returns the canonical path for an expanded event.
// dateStr is YYYY-MM-DD. Events live flat under events/<calendar>/ — Full
// Calendar's local-source reader does NOT recurse into subfolders, so a
// year subfolder would hide the events from its index.
func (v *Vault) EventPath(calendar, dateStr, slug string) string {
	return filepath.Join(v.EventsDir(), calendar, dateStr+"-"+slug+".md")
}

// expandTilde turns a leading "~" or "~/" into the user's home dir. Returns
// the input unchanged if no tilde prefix.
func expandTilde(p string) (string, error) {
	if p == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p, err
		}
		return filepath.Join(home, p[2:]), nil
	}
	return p, nil
}
