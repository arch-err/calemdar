// Package actions loads ~/.config/calemdar/actions.yaml and runs named
// entries from it. Actions are intentionally NOT stored in the synced
// vault — see docs/security-reviews/notif-threat-model-2026-04-25.md
// for why. Vault frontmatter references actions by short name; the
// laptop-local actions.yaml is the trust boundary that resolves names
// to executable commands.
package actions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// nameRE keeps action names tight: lowercase, alpha-first, hyphens only.
// The same regex is enforced on the vault side (model.actionNameRE)
// so what one side accepts, the other side accepts.
var nameRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)

// DefaultTimeout is applied when an action has no timeout: field of its
// own. 30s is plenty for "open zoom" / "lock screen" style hooks; the
// runner kills longer runs to keep the daemon responsive.
const DefaultTimeout = 30 * time.Second

// Action is one entry from actions.yaml. cmd and shell are mutually
// exclusive; exactly one must be set.
//
//	some-action:
//	  cmd: ["/usr/bin/notify-send", "-u", "low", "hi"]
//	  timeout: 10s
//	other-action:
//	  shell: "echo hi >> ~/.local/state/foo.log"
type Action struct {
	Name    string        `yaml:"-"`
	Cmd     CmdSpec       `yaml:"cmd"`
	Shell   string        `yaml:"shell"`
	Timeout time.Duration `yaml:"timeout,omitempty"`
}

// CmdSpec accepts either a string ("/path/to/script") or an array of
// strings (["/path/to/script", "arg1", "arg2"]) in YAML. Both forms
// produce the same Argv slice; the runner spawns it as direct exec
// (no shell), so shell metacharacters in either form are passed
// literally to the program.
type CmdSpec struct {
	Argv []string
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (c *CmdSpec) UnmarshalYAML(node *yaml.Node) error {
	if node == nil || node.Kind == 0 {
		return nil
	}
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Value == "" {
			return nil
		}
		c.Argv = []string{node.Value}
		return nil
	case yaml.SequenceNode:
		var s []string
		if err := node.Decode(&s); err != nil {
			return err
		}
		c.Argv = s
		return nil
	default:
		return fmt.Errorf("cmd must be a string or a sequence, got %v", node.Tag)
	}
}

// Config is the actions.yaml on-disk shape.
type Config struct {
	Actions map[string]Action `yaml:"actions"`
}

// Path returns the resolved absolute path to actions.yaml. Resolution
// follows XDG: $XDG_CONFIG_HOME/calemdar/actions.yaml, then
// $HOME/.config/calemdar/actions.yaml.
func Path() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "calemdar", "actions.yaml"), nil
}

// Load reads actions.yaml at the given path. Missing file returns an
// empty config (not an error) so the daemon can start cleanly before
// any actions are configured. Validation runs after parse.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{Actions: map[string]Action{}}, nil
		}
		return nil, fmt.Errorf("actions: read %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("actions: parse %s: %w", path, err)
	}
	if cfg.Actions == nil {
		cfg.Actions = map[string]Action{}
	}
	for name, a := range cfg.Actions {
		a.Name = name
		if !nameRE.MatchString(name) {
			return nil, fmt.Errorf("actions: name %q must match %s", name, nameRE.String())
		}
		if len(a.Cmd.Argv) > 0 && a.Shell != "" {
			return nil, fmt.Errorf("actions: %q sets both cmd and shell — pick one", name)
		}
		if len(a.Cmd.Argv) == 0 && a.Shell == "" {
			return nil, fmt.Errorf("actions: %q must set cmd or shell", name)
		}
		if a.Timeout < 0 {
			return nil, fmt.Errorf("actions: %q timeout must be >= 0", name)
		}
		cfg.Actions[name] = a
	}
	return &cfg, nil
}

// Runner spawns actions with curated env, bounded concurrency, and a
// per-spawn timeout. Safe for concurrent use.
type Runner struct {
	cfg     *Config
	maxPara int
	sem     chan struct{}
	mu      sync.RWMutex
}

// NewRunner returns a runner backed by cfg. maxConcurrent caps the
// number of actions spawned at once — the cap exists so a flurry of
// fires can't fork-bomb the daemon. <=0 falls back to 4.
func NewRunner(cfg *Config, maxConcurrent int) *Runner {
	if maxConcurrent <= 0 {
		maxConcurrent = 4
	}
	return &Runner{
		cfg:     cfg,
		maxPara: maxConcurrent,
		sem:     make(chan struct{}, maxConcurrent),
	}
}

// Reload swaps in a fresh config. Used by the daemon when actions.yaml
// changes — keeps the runner's semaphore intact.
func (r *Runner) Reload(cfg *Config) {
	r.mu.Lock()
	r.cfg = cfg
	r.mu.Unlock()
}

// Lookup returns the action by name and a found-flag. Used by the cli
// for `calemdar actions list / show / test`.
func (r *Runner) Lookup(name string) (Action, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.cfg == nil {
		return Action{}, false
	}
	a, ok := r.cfg.Actions[name]
	return a, ok
}

// Names returns the registered action names. Order is unspecified.
func (r *Runner) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.cfg == nil {
		return nil
	}
	out := make([]string, 0, len(r.cfg.Actions))
	for k := range r.cfg.Actions {
		out = append(out, k)
	}
	return out
}

// Run spawns the named action with the supplied env. Blocks until the
// child exits or the per-action timeout expires (whichever first).
//
// Env is curated: only the keys passed in `env` reach the child plus a
// minimal $PATH. No parent-process env inheritance — keeps secrets
// (NTFY_TOKEN, etc.) out of action subprocesses.
func (r *Runner) Run(ctx context.Context, name string, env map[string]string) error {
	a, ok := r.Lookup(name)
	if !ok {
		return fmt.Errorf("action %q not registered in actions.yaml", name)
	}
	timeout := a.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Gate concurrency.
	select {
	case r.sem <- struct{}{}:
	case <-cctx.Done():
		return cctx.Err()
	}
	defer func() { <-r.sem }()

	var cmd *exec.Cmd
	switch {
	case len(a.Cmd.Argv) > 0:
		cmd = exec.CommandContext(cctx, a.Cmd.Argv[0], a.Cmd.Argv[1:]...)
	case a.Shell != "":
		cmd = exec.CommandContext(cctx, "sh", "-c", a.Shell)
	default:
		// Validate would have rejected this, but defensive.
		return fmt.Errorf("action %q has neither cmd nor shell", name)
	}

	// Curated env: minimal PATH + per-event CALEMDAR_* variables. We
	// intentionally do NOT inherit os.Environ() so the action subprocess
	// can't see the daemon's secrets.
	cmd.Env = curateEnv(env)

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("spawn %q: %w (output: %s)", name, err, string(out))
	}
	return nil
}

// curateEnv builds the child env. PATH is fixed-defaulted; HOME comes
// from the parent (so ~/-relative paths in the action still work).
func curateEnv(extra map[string]string) []string {
	env := []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
	}
	if home := os.Getenv("HOME"); home != "" {
		env = append(env, "HOME="+home)
	}
	if user := os.Getenv("USER"); user != "" {
		env = append(env, "USER="+user)
	}
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}
