package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeYAML(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "actions.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadAcceptsCmdAsStringOrArray(t *testing.T) {
	p := writeYAML(t, `
actions:
  one:
    cmd: /usr/bin/true
  two:
    cmd: ["/usr/bin/echo", "hi"]
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	one := cfg.Actions["one"]
	if len(one.Cmd.Argv) != 1 || one.Cmd.Argv[0] != "/usr/bin/true" {
		t.Errorf("one: %v", one.Cmd.Argv)
	}
	two := cfg.Actions["two"]
	if len(two.Cmd.Argv) != 2 || two.Cmd.Argv[1] != "hi" {
		t.Errorf("two: %v", two.Cmd.Argv)
	}
}

func TestLoadRejectsCmdAndShell(t *testing.T) {
	p := writeYAML(t, `
actions:
  bad:
    cmd: /usr/bin/true
    shell: "echo hi"
`)
	if _, err := Load(p); err == nil {
		t.Error("expected error for both cmd and shell")
	}
}

func TestLoadRejectsNeitherCmdNorShell(t *testing.T) {
	p := writeYAML(t, `
actions:
  bad: {}
`)
	if _, err := Load(p); err == nil {
		t.Error("expected error for neither cmd nor shell")
	}
}

func TestLoadRejectsBadName(t *testing.T) {
	p := writeYAML(t, `
actions:
  Bad-Name:
    cmd: /usr/bin/true
`)
	if _, err := Load(p); err == nil {
		t.Error("expected name regex error")
	}
}

func TestLoadMissingFileEmpty(t *testing.T) {
	cfg, err := Load("/no/such/file.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Actions) != 0 {
		t.Errorf("expected empty config, got %d", len(cfg.Actions))
	}
}

func TestRunnerRunsCmdAndPropagatesEnv(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	body := `
actions:
  hello:
    cmd: ["/bin/sh", "-c", "echo $CALEMDAR_TITLE > ` + out + `"]
    timeout: 5s
`
	p := writeYAML(t, body)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRunner(cfg, 2)
	err = r.Run(context.Background(), "hello", map[string]string{
		"CALEMDAR_TITLE": "morning",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, _ := os.ReadFile(out)
	if strings.TrimSpace(string(got)) != "morning" {
		t.Errorf("got %q, want %q", string(got), "morning")
	}
}

func TestRunnerRunsShellWithRedirect(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	p := writeYAML(t, `
actions:
  shelly:
    shell: "echo $CALEMDAR_TITLE > `+out+`"
    timeout: 5s
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRunner(cfg, 2)
	if err := r.Run(context.Background(), "shelly", map[string]string{
		"CALEMDAR_TITLE": "evening",
	}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(out)
	if strings.TrimSpace(string(got)) != "evening" {
		t.Errorf("got %q, want %q", string(got), "evening")
	}
}

func TestRunnerRejectsUnknownAction(t *testing.T) {
	cfg, _ := Load("/no/such/file")
	r := NewRunner(cfg, 1)
	if err := r.Run(context.Background(), "missing", nil); err == nil {
		t.Error("expected unknown-action error")
	}
}

func TestRunnerTimeoutKills(t *testing.T) {
	p := writeYAML(t, `
actions:
  slow:
    cmd: ["/bin/sh", "-c", "sleep 5"]
    timeout: 200ms
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRunner(cfg, 1)
	start := time.Now()
	err = r.Run(context.Background(), "slow", nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Error("expected timeout error")
	}
	if elapsed > 2*time.Second {
		t.Errorf("did not honour timeout (took %v)", elapsed)
	}
}

func TestRunnerCuratesEnvDoesNotLeak(t *testing.T) {
	t.Setenv("SECRET_TOKEN", "ssshhh")
	dir := t.TempDir()
	out := filepath.Join(dir, "out.txt")
	p := writeYAML(t, `
actions:
  introspect:
    cmd: ["/bin/sh", "-c", "echo SECRET=$SECRET_TOKEN > `+out+`"]
    timeout: 5s
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRunner(cfg, 1)
	if err := r.Run(context.Background(), "introspect", nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(out)
	if strings.Contains(string(got), "ssshhh") {
		t.Errorf("parent env leaked into child: %q", string(got))
	}
}
