// Package config loads the optional YAML config file from the XDG config
// directory. All fields are optional; missing fields fall back to defaults.
// CLI flags and environment variables continue to override this file.
//
// Call LoadAndApply once at startup; after that, Active holds the resolved
// configuration for the rest of the process to read.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the on-disk shape. All fields optional; zero values → defaults.
// Vault is intentionally NOT omitempty — when empty, the stub and `show`
// both render `vault: ""` so the user can't miss that it's unset.
type Config struct {
	Vault               string   `yaml:"vault"`
	Timezone            string   `yaml:"timezone,omitempty"`
	NightlyAt           string   `yaml:"nightly_at,omitempty"`     // "HH:MM" 24h
	HorizonMonths       int      `yaml:"horizon_months,omitempty"` // default 12
	ArchiveCutoffMonths int      `yaml:"archive_cutoff_months,omitempty"`
	DebounceMs          int      `yaml:"debounce_ms,omitempty"`
	Calendars           []string `yaml:"calendars,omitempty"`
}

// Defaults returns a Config with v1's built-in defaults filled in.
// Used to seed missing fields during Load.
func Defaults() Config {
	return Config{
		Timezone:            "Europe/Stockholm",
		NightlyAt:           "03:00",
		HorizonMonths:       12,
		ArchiveCutoffMonths: 6,
		DebounceMs:          500,
		Calendars:           []string{"health", "tech", "work", "life", "friends-family", "special"},
	}
}

// Path returns the expected config file path. Resolution: $XDG_CONFIG_HOME
// then ~/.config. Does not check existence.
func Path() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "calemdar", "config.yaml"), nil
}

// Active is the resolved configuration for the current process. Populated
// by LoadAndApply (or directly by tests). Zero value is Defaults().
var Active = Defaults()

// LoadAndApply reads the config file (if any), merges onto defaults, and
// stores the result in Active. Safe to call multiple times.
func LoadAndApply() error {
	cfg, err := Load()
	if err != nil {
		return err
	}
	Active = cfg
	return nil
}

// Validate checks fields for sane values. Called by LoadAndApply and can be
// called by callers wanting a pre-flight check.
func (c Config) Validate() error {
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		return fmt.Errorf("config: unknown timezone %q: %w", c.Timezone, err)
	}
	if _, err := time.Parse("15:04", c.NightlyAt); err != nil {
		return fmt.Errorf("config: nightly_at %q must be HH:MM: %w", c.NightlyAt, err)
	}
	if c.HorizonMonths < 1 || c.HorizonMonths > 120 {
		return fmt.Errorf("config: horizon_months %d out of range [1,120]", c.HorizonMonths)
	}
	if c.ArchiveCutoffMonths < 0 || c.ArchiveCutoffMonths > 120 {
		return fmt.Errorf("config: archive_cutoff_months %d out of range [0,120]", c.ArchiveCutoffMonths)
	}
	if c.DebounceMs < 1 || c.DebounceMs > 60000 {
		return fmt.Errorf("config: debounce_ms %d out of range [1,60000]", c.DebounceMs)
	}
	if len(c.Calendars) == 0 {
		return fmt.Errorf("config: calendars list is empty")
	}
	return nil
}

// Load reads the config file if present. Missing file is not an error —
// returns Defaults(). Parse errors are returned.
func Load() (Config, error) {
	cfg := Defaults()
	path, err := Path()
	if err != nil {
		return cfg, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("config: read %s: %w", path, err)
	}
	var file Config
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return cfg, fmt.Errorf("config: parse %s: %w", path, err)
	}
	merged := merge(cfg, file)
	if err := merged.Validate(); err != nil {
		return cfg, err
	}
	return merged, nil
}

// merge overlays non-zero fields of file onto base.
func merge(base, file Config) Config {
	if file.Vault != "" {
		base.Vault = file.Vault
	}
	if file.Timezone != "" {
		base.Timezone = file.Timezone
	}
	if file.NightlyAt != "" {
		base.NightlyAt = file.NightlyAt
	}
	if file.HorizonMonths != 0 {
		base.HorizonMonths = file.HorizonMonths
	}
	if file.ArchiveCutoffMonths != 0 {
		base.ArchiveCutoffMonths = file.ArchiveCutoffMonths
	}
	if file.DebounceMs != 0 {
		base.DebounceMs = file.DebounceMs
	}
	if len(file.Calendars) > 0 {
		base.Calendars = file.Calendars
	}
	return base
}
