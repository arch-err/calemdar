package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
	if d.Notifications.TickInterval.AsDuration() != time.Minute {
		t.Errorf("TickInterval = %v, want 1m", d.Notifications.TickInterval.AsDuration())
	}
	if d.Notifications.MaxLead.AsDuration() != 23*time.Hour {
		t.Errorf("MaxLead = %v, want 23h", d.Notifications.MaxLead.AsDuration())
	}
	if d.Notifications.MaxConcurrentSpawns != 4 {
		t.Errorf("MaxConcurrentSpawns = %d, want 4", d.Notifications.MaxConcurrentSpawns)
	}
}

func TestNotificationsValidateNtfyRequiresURLAndTopicWhenEnabled(t *testing.T) {
	c := Defaults()
	c.Vault = "/tmp/v"
	c.Notifications.Enabled = true
	c.Notifications.Backends.Ntfy.Enabled = true
	if err := c.Validate(); err == nil {
		t.Error("expected error when ntfy enabled without url/topic")
	}
	c.Notifications.Backends.Ntfy.URL = "https://ntfy.sh"
	c.Notifications.Backends.Ntfy.Topic = "t"
	if err := c.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNotificationsValidateTickInterval(t *testing.T) {
	c := Defaults()
	c.Vault = "/tmp/v"
	c.Notifications.Enabled = true
	c.Notifications.TickInterval = Duration(15 * time.Second) // below minimum
	if err := c.Validate(); err == nil {
		t.Error("expected error for tick_interval below 30s")
	}
}

func TestNotificationsValidateUrgency(t *testing.T) {
	c := Defaults()
	c.Vault = "/tmp/v"
	c.Notifications.Enabled = true
	c.Notifications.Backends.System.Urgency = "weird"
	if err := c.Validate(); err == nil {
		t.Error("expected error for unknown urgency")
	}
	c.Notifications.Backends.System.Urgency = "low"
	if err := c.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
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
  tick_interval: 2m
  max_lead: 12h
  calendars: [work]
  backends:
    system:
      enabled: true
      urgency: low
    ntfy:
      enabled: true
      url: https://ntfy.example
      topic: topic-abc
  actions:
    enabled: true
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
	if cfg.Notifications.TickInterval.AsDuration() != 2*time.Minute {
		t.Errorf("TickInterval = %v", cfg.Notifications.TickInterval.AsDuration())
	}
	if !cfg.Notifications.Backends.System.Enabled {
		t.Error("system.enabled should be true")
	}
	if cfg.Notifications.Backends.Ntfy.URL != "https://ntfy.example" {
		t.Errorf("ntfy.url = %q", cfg.Notifications.Backends.Ntfy.URL)
	}
	if cfg.Notifications.Backends.Ntfy.Topic != "topic-abc" {
		t.Errorf("ntfy.topic = %q", cfg.Notifications.Backends.Ntfy.Topic)
	}
	if !cfg.Notifications.Actions.Enabled {
		t.Error("actions.enabled should be true")
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
