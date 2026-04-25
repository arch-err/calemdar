package main

import (
	"context"
	"fmt"
	"os/signal"
	"strings"
	"syscall"

	"github.com/arch-err/calemdar/internal/actions"
	"github.com/arch-err/calemdar/internal/config"
	"github.com/arch-err/calemdar/internal/notify"
	"github.com/spf13/cobra"
)

var notifyCmd = &cobra.Command{
	Use:   "notify",
	Short: "Send and manage upcoming-event notifications",
}

var notifyTestCmd = &cobra.Command{
	Use:   "test [backend]",
	Short: "Send a test message via every enabled backend (or one named).",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runNotifyTest,
}

// runNotifyTest fires a test through every enabled backend (or a single
// named one). Bypasses the schedule layer — proves wiring before the
// daemon flips on.
func runNotifyTest(cmd *cobra.Command, args []string) error {
	cfg := config.Active.Notifications
	target := ""
	if len(args) > 0 {
		target = strings.ToLower(args[0])
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	tested := 0
	if (target == "" || target == "ntfy") && (cfg.Backends.Ntfy.Enabled || cfg.Backends.Ntfy.URL != "") {
		if cfg.Backends.Ntfy.URL == "" || cfg.Backends.Ntfy.Topic == "" {
			return fmt.Errorf("ntfy not configured: set notifications.backends.ntfy.url and .topic in %s",
				configPathOrLiteral())
		}
		n := notify.NewNtfy(notify.NtfyConfig{URL: cfg.Backends.Ntfy.URL, Topic: cfg.Backends.Ntfy.Topic})
		if err := n.SendTest(ctx); err != nil {
			return fmt.Errorf("ntfy: %w", err)
		}
		fmt.Printf("ntfy: sent test push → %s/%s\n", notify.RedactURL(cfg.Backends.Ntfy.URL), cfg.Backends.Ntfy.Topic)
		tested++
	}
	if (target == "" || target == "system") && cfg.Backends.System.Enabled {
		s := notify.NewSystem(notify.SystemConfig{
			BinaryPath: cfg.Backends.System.BinaryPath,
			Urgency:    cfg.Backends.System.Urgency,
		})
		if err := s.SendTest(ctx); err != nil {
			return fmt.Errorf("system: %w", err)
		}
		fmt.Println("system: sent test desktop notification")
		tested++
	}
	if tested == 0 {
		if target != "" {
			return fmt.Errorf("backend %q is not enabled or not configured", target)
		}
		return fmt.Errorf("no backends enabled in %s — nothing to test",
			configPathOrLiteral())
	}
	return nil
}

var notifyActionsCmd = &cobra.Command{
	Use:   "actions",
	Short: "List actions registered in actions.yaml",
	RunE:  runNotifyActions,
}

func runNotifyActions(cmd *cobra.Command, args []string) error {
	path := config.Active.Notifications.Actions.ConfigPath
	if path == "" {
		p, err := actions.Path()
		if err != nil {
			return err
		}
		path = p
	}
	cfg, err := actions.Load(path)
	if err != nil {
		return err
	}
	if len(cfg.Actions) == 0 {
		fmt.Printf("no actions registered (looked at %s)\n", path)
		return nil
	}
	fmt.Printf("# %s\n", path)
	for name, a := range cfg.Actions {
		switch {
		case len(a.Cmd.Argv) > 0:
			fmt.Printf("%-24s cmd: %v\n", name, a.Cmd.Argv)
		case a.Shell != "":
			fmt.Printf("%-24s shell: %s\n", name, a.Shell)
		}
	}
	return nil
}

func init() {
	notifyCmd.AddCommand(notifyTestCmd, notifyActionsCmd)
}
