package main

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/arch-err/calemdar/internal/config"
	"github.com/arch-err/calemdar/internal/notify"
	"github.com/spf13/cobra"
)

var notifyCmd = &cobra.Command{
	Use:   "notify",
	Short: "Send and manage upcoming-event notifications",
}

var notifyTestCmd = &cobra.Command{
	Use:   "test",
	Short: "Send a single test push to the configured ntfy topic",
	RunE:  runNotifyTest,
}

func runNotifyTest(cmd *cobra.Command, args []string) error {
	c := config.Active.Notifications
	// Don't gate on c.Enabled — the whole point of this command is to prove
	// the URL/topic work before the user flips enabled on in the daemon.
	if c.NtfyURL == "" || c.NtfyTopic == "" {
		return fmt.Errorf("notifications not configured: set notifications.ntfy_url and notifications.ntfy_topic in %s",
			configPathOrLiteral())
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	n := notify.New(nil, c)
	if err := n.SendTest(ctx); err != nil {
		return fmt.Errorf("send test: %w", err)
	}
	fmt.Printf("sent test push → %s/%s\n", c.NtfyURL, c.NtfyTopic)
	return nil
}

func init() {
	notifyCmd.AddCommand(notifyTestCmd)
}
