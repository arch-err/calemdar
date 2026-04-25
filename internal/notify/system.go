package notify

import (
	"context"
	"fmt"
	"os/exec"
)

// SystemConfig configures the system (linux desktop) backend.
type SystemConfig struct {
	// BinaryPath is the absolute path to notify-send. Defaults to
	// "notify-send" when empty (looked up via $PATH).
	BinaryPath string
	// Urgency maps to notify-send -u {low|normal|critical}. Empty leaves
	// notify-send to use its default (normal).
	Urgency string
}

// System is the libnotify-via-notify-send backend. Wired up so users
// running the daemon under `systemd --user` get a desktop popup whenever
// a notify rule fires.
type System struct {
	cfg SystemConfig
}

// NewSystem returns a configured system backend. Like NewNtfy, this does
// not auto-register.
func NewSystem(cfg SystemConfig) *System { return &System{cfg: cfg} }

// Name implements Backend.
func (System) Name() string { return "system" }

// Send shells out to notify-send. We pass all values as argv (no shell);
// the body is passed positionally (notify-send takes "summary" and an
// optional "body" as positional args).
func (s *System) Send(ctx context.Context, msg Notification) error {
	bin := s.cfg.BinaryPath
	if bin == "" {
		bin = "notify-send"
	}
	args := []string{
		"--app-name=calemdar",
		"--category=calendar",
	}
	if s.cfg.Urgency != "" {
		args = append(args, "--urgency="+s.cfg.Urgency)
	}
	if len(msg.Tags) > 0 {
		// notify-send has no native tag concept; surface them as a hint.
		// Many libnotify daemons ignore unknown hints gracefully.
		for _, t := range msg.Tags {
			args = append(args, "--hint=string:x-calemdar-tag:"+t)
		}
	}
	args = append(args, msg.Title, msg.Body)

	cmd := exec.CommandContext(ctx, bin, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("notify-send: %w (output: %s)", err, string(out))
	}
	return nil
}

// SendTest fires a single canned desktop notification. Used by the cli.
func (s *System) SendTest(ctx context.Context) error {
	return s.Send(ctx, Notification{
		Title: "calemdar: test",
		Body:  "calemdar system test — if you see this, the wiring is good.",
		Tags:  []string{"calendar", "test"},
	})
}
