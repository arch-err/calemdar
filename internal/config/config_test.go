package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	d := Defaults()
	if cfg.Timezone != d.Timezone || cfg.NightlyAt != d.NightlyAt {
		t.Errorf("defaults not applied: %+v", cfg)
	}
}

func TestLoadOverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	path := filepath.Join(dir, "calemdar", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `vault: /tmp/my-vault
timezone: UTC
nightly_at: "04:30"
horizon_months: 24
archive_cutoff_months: 3
debounce_ms: 1000
calendars: [a, b, c]
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Vault != "/tmp/my-vault" {
		t.Errorf("Vault = %q", cfg.Vault)
	}
	if cfg.Timezone != "UTC" {
		t.Errorf("Timezone = %q", cfg.Timezone)
	}
	if cfg.NightlyAt != "04:30" {
		t.Errorf("NightlyAt = %q", cfg.NightlyAt)
	}
	if cfg.HorizonMonths != 24 {
		t.Errorf("HorizonMonths = %d", cfg.HorizonMonths)
	}
	if len(cfg.Calendars) != 3 {
		t.Errorf("Calendars = %v", cfg.Calendars)
	}
}

func TestLoadPartialMerge(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	path := filepath.Join(dir, "calemdar", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	// Only vault set; other keys should fall back to defaults.
	if err := os.WriteFile(path, []byte("vault: /tmp/v\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Vault != "/tmp/v" {
		t.Errorf("Vault = %q", cfg.Vault)
	}
	if cfg.Timezone != "Europe/Stockholm" {
		t.Errorf("Timezone should default: %q", cfg.Timezone)
	}
	if cfg.NightlyAt != "03:00" {
		t.Errorf("NightlyAt should default: %q", cfg.NightlyAt)
	}
}

func TestPathUsesXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	got, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	want := "/xdg/calemdar/config.yaml"
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}
