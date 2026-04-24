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

func TestNotificationsDefaults(t *testing.T) {
	d := Defaults()
	if d.Notifications.Enabled {
		t.Error("Enabled should default to false")
	}
	if len(d.Notifications.LeadMinutes) != 2 ||
		d.Notifications.LeadMinutes[0] != 5 ||
		d.Notifications.LeadMinutes[1] != 60 {
		t.Errorf("LeadMinutes = %v, want [5 60]", d.Notifications.LeadMinutes)
	}
}

func TestNotificationsValidateRequiresURLAndTopicWhenEnabled(t *testing.T) {
	c := Defaults()
	c.Vault = "/tmp/v"
	c.Notifications.Enabled = true
	if err := c.Validate(); err == nil {
		t.Error("expected error when enabled without url/topic")
	}
	c.Notifications.NtfyURL = "https://ntfy.sh"
	c.Notifications.NtfyTopic = "t"
	if err := c.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNotificationsValidateLeadMinutesPositive(t *testing.T) {
	c := Defaults()
	c.Notifications.LeadMinutes = []int{5, 0, 60}
	if err := c.Validate(); err == nil {
		t.Error("expected error for zero lead minute")
	}
	c.Notifications.LeadMinutes = []int{-1}
	if err := c.Validate(); err == nil {
		t.Error("expected error for negative lead minute")
	}
}

func TestNotificationsMergeFromFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	path := filepath.Join(dir, "calemdar", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `vault: /tmp/my-vault
notifications:
  enabled: true
  ntfy_url: https://ntfy.example
  ntfy_topic: topic-abc
  lead_minutes: [10, 30]
  calendars: [work]
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Notifications.Enabled {
		t.Error("Enabled should be true")
	}
	if cfg.Notifications.NtfyURL != "https://ntfy.example" {
		t.Errorf("NtfyURL = %q", cfg.Notifications.NtfyURL)
	}
	if cfg.Notifications.NtfyTopic != "topic-abc" {
		t.Errorf("NtfyTopic = %q", cfg.Notifications.NtfyTopic)
	}
	if len(cfg.Notifications.LeadMinutes) != 2 || cfg.Notifications.LeadMinutes[0] != 10 {
		t.Errorf("LeadMinutes = %v", cfg.Notifications.LeadMinutes)
	}
	if len(cfg.Notifications.Calendars) != 1 || cfg.Notifications.Calendars[0] != "work" {
		t.Errorf("Calendars = %v", cfg.Notifications.Calendars)
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
