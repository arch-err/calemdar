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
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ntfyTopicRE matches ntfy's documented topic charset: alnum + hyphen +
// underscore, 1-64 chars. Rejects anything that would alter the URL path
// shape (slashes, query, fragment).
var ntfyTopicRE = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Config is the on-disk shape. All fields optional; zero values → defaults.
// Vault and BasePath are intentionally NOT omitempty — the stub and `show`
// both render them even when empty so the user can't miss them.
type Config struct {
	Vault string `yaml:"vault"`
	// BasePath is the subfolder (relative to Vault) under which `recurring/`,
	// `events/`, and `archive/` live. Empty means "at vault root".
	BasePath            string        `yaml:"base_path"`
	Timezone            string        `yaml:"timezone,omitempty"`
	NightlyAt           string        `yaml:"nightly_at,omitempty"`     // "HH:MM" 24h
	HorizonMonths       int           `yaml:"horizon_months,omitempty"` // default 12
	ArchiveCutoffMonths int           `yaml:"archive_cutoff_months,omitempty"`
	DebounceMs          int           `yaml:"debounce_ms,omitempty"`
	Calendars           []string      `yaml:"calendars,omitempty"`
	Notifications       Notifications `yaml:"notifications"`
}

// Notifications controls the per-event notification subsystem.
//
// Per-event rules live in event/root frontmatter as a `notify:` list.
// This block governs the daemon-side knobs: which backends are
// available, how often the scheduler ticks, the action runner setup.
type Notifications struct {
	Enabled bool `yaml:"enabled"`

	// TickInterval is how often the scheduler wakes. "1m" is the
	// default; values below 30s are clamped at validation time.
	TickInterval Duration `yaml:"tick_interval,omitempty"`
	// MaxLead caps the longest event-bound lead the scheduler will
	// honour. "23h" by default — keeps the lookahead window tight.
	MaxLead Duration `yaml:"max_lead,omitempty"`
	// MaxConcurrentSpawns caps the action runner's parallelism so a
	// flurry of fires can't fork-bomb the daemon.
	MaxConcurrentSpawns int `yaml:"max_concurrent_spawns,omitempty"`
	// Calendars filters which calendars the scheduler considers.
	// Empty means all configured calendars.
	Calendars []string `yaml:"calendars,omitempty"`

	// Backends is the per-backend toggle + per-backend config. A
	// backend is registered iff its Enabled flag is true.
	Backends Backends `yaml:"backends"`

	// Actions wires the script-runner side. Disabled by default per
	// the threat model recommendation: vault frontmatter cannot trigger
	// scripts unless the user explicitly opts in.
	Actions ActionsConfig `yaml:"actions"`
}

// Backends groups one config block per registered backend. New backends
// add a field here.
type Backends struct {
	System SystemBackend `yaml:"system"`
	Ntfy   NtfyBackend   `yaml:"ntfy"`
}

// SystemBackend is the libnotify-via-notify-send backend.
type SystemBackend struct {
	Enabled    bool   `yaml:"enabled"`
	BinaryPath string `yaml:"binary_path,omitempty"` // override notify-send binary
	Urgency    string `yaml:"urgency,omitempty"`     // "low" | "normal" | "critical"
}

// NtfyBackend is the ntfy.sh / self-hosted-ntfy backend.
type NtfyBackend struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url"`   // base, e.g. "https://ntfy.sh"
	Topic   string `yaml:"topic"` // topic name
}

// ActionsConfig wires the script-runner side. ConfigPath is honoured
// when set; empty falls back to the XDG default
// (~/.config/calemdar/actions.yaml).
type ActionsConfig struct {
	Enabled    bool   `yaml:"enabled"`
	ConfigPath string `yaml:"config_path,omitempty"`
}

// Duration is a time.Duration alias with YAML scalar parsing
// ("1m" / "30s" / "23h"). Stored as time.Duration so callers can use
// it directly.
type Duration time.Duration

// UnmarshalYAML accepts a scalar parsed by time.ParseDuration.
func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	if node == nil || node.Value == "" {
		return nil
	}
	v, err := time.ParseDuration(node.Value)
	if err != nil {
		return fmt.Errorf("duration %q: %w", node.Value, err)
	}
	*d = Duration(v)
	return nil
}

// MarshalYAML renders the duration in canonical form so `config show`
// prints a human-readable value.
func (d Duration) MarshalYAML() (any, error) {
	return time.Duration(d).String(), nil
}

// AsDuration is a small convenience for the common cast.
func (d Duration) AsDuration() time.Duration { return time.Duration(d) }

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
		Notifications: Notifications{
			Enabled:             false,
			TickInterval:        Duration(time.Minute),
			MaxLead:             Duration(23 * time.Hour),
			MaxConcurrentSpawns: 4,
			Backends: Backends{
				System: SystemBackend{Enabled: false},
				Ntfy:   NtfyBackend{Enabled: false},
			},
			Actions: ActionsConfig{Enabled: false},
		},
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

// Validate checks fields for sane values. BasePath is checked for
// traversal attempts (../) but not for existence — the vault or scaffold
// step creates it. Called by LoadAndApply and can be
// called by callers wanting a pre-flight check.
func (c Config) Validate() error {
	if strings.Contains(c.BasePath, "..") {
		return fmt.Errorf("config: base_path %q contains .. — must stay inside the vault", c.BasePath)
	}
	if strings.HasPrefix(c.BasePath, "/") || strings.HasPrefix(c.BasePath, string(os.PathSeparator)) {
		return fmt.Errorf("config: base_path %q must be relative to the vault (no leading slash)", c.BasePath)
	}
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
	return c.Notifications.Validate()
}

// Validate enforces notification-specific invariants. Pulled out of
// Config.Validate so other callers (config init, tests) can run it
// directly.
func (n Notifications) Validate() error {
	if !n.Enabled {
		// Loose check: ntfy topic, if set, must still match the regex
		// — saves users from typo-ing it before flipping enabled on.
		if n.Backends.Ntfy.Topic != "" && !ntfyTopicRE.MatchString(n.Backends.Ntfy.Topic) {
			return fmt.Errorf("config: notifications.backends.ntfy.topic %q invalid (must match %s)",
				n.Backends.Ntfy.Topic, ntfyTopicRE.String())
		}
		return nil
	}
	tick := n.TickInterval.AsDuration()
	if tick != 0 && tick < 30*time.Second {
		return fmt.Errorf("config: notifications.tick_interval %s below minimum 30s", tick)
	}
	max := n.MaxLead.AsDuration()
	if max != 0 && max > 24*time.Hour {
		return fmt.Errorf("config: notifications.max_lead %s above 24h cap", max)
	}
	if n.Backends.Ntfy.Enabled {
		if strings.TrimSpace(n.Backends.Ntfy.URL) == "" {
			return fmt.Errorf("config: notifications.backends.ntfy.url required when ntfy backend enabled")
		}
		if strings.TrimSpace(n.Backends.Ntfy.Topic) == "" {
			return fmt.Errorf("config: notifications.backends.ntfy.topic required when ntfy backend enabled")
		}
	}
	if n.Backends.Ntfy.Topic != "" && !ntfyTopicRE.MatchString(n.Backends.Ntfy.Topic) {
		return fmt.Errorf("config: notifications.backends.ntfy.topic %q invalid (must match %s)",
			n.Backends.Ntfy.Topic, ntfyTopicRE.String())
	}
	if n.Backends.System.Urgency != "" {
		switch n.Backends.System.Urgency {
		case "low", "normal", "critical":
		default:
			return fmt.Errorf("config: notifications.backends.system.urgency %q must be low|normal|critical",
				n.Backends.System.Urgency)
		}
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
	if file.BasePath != "" {
		base.BasePath = file.BasePath
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
	base.Notifications = mergeNotifications(base.Notifications, file.Notifications)
	return base
}

// mergeNotifications overlays non-zero fields of file onto base.
func mergeNotifications(base, file Notifications) Notifications {
	if file.Enabled {
		base.Enabled = true
	}
	if file.TickInterval != 0 {
		base.TickInterval = file.TickInterval
	}
	if file.MaxLead != 0 {
		base.MaxLead = file.MaxLead
	}
	if file.MaxConcurrentSpawns != 0 {
		base.MaxConcurrentSpawns = file.MaxConcurrentSpawns
	}
	if len(file.Calendars) > 0 {
		base.Calendars = file.Calendars
	}
	base.Backends = mergeBackends(base.Backends, file.Backends)
	base.Actions = mergeActions(base.Actions, file.Actions)
	return base
}

func mergeBackends(base, file Backends) Backends {
	if file.System.Enabled {
		base.System.Enabled = true
	}
	if file.System.BinaryPath != "" {
		base.System.BinaryPath = file.System.BinaryPath
	}
	if file.System.Urgency != "" {
		base.System.Urgency = file.System.Urgency
	}
	if file.Ntfy.Enabled {
		base.Ntfy.Enabled = true
	}
	if file.Ntfy.URL != "" {
		base.Ntfy.URL = file.Ntfy.URL
	}
	if file.Ntfy.Topic != "" {
		base.Ntfy.Topic = file.Ntfy.Topic
	}
	return base
}

func mergeActions(base, file ActionsConfig) ActionsConfig {
	if file.Enabled {
		base.Enabled = true
	}
	if file.ConfigPath != "" {
		base.ConfigPath = file.ConfigPath
	}
	return base
}
