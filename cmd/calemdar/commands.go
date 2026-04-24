package main

import (
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the watcher + nightly timers",
	RunE: func(cmd *cobra.Command, args []string) error {
		return errNotImplemented("serve")
	},
}

var reindexCmd = &cobra.Command{
	Use:   "reindex",
	Short: "Rebuild SQLite cache from disk",
	RunE: func(cmd *cobra.Command, args []string) error {
		return errNotImplemented("reindex")
	},
}

var expandCmd = &cobra.Command{
	Use:   "expand <series-id>",
	Short: "Force-expand a single series",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return errNotImplemented("expand")
	},
}

var extendCmd = &cobra.Command{
	Use:   "extend",
	Short: "Extend the 12-month horizon for all series",
	RunE: func(cmd *cobra.Command, args []string) error {
		return errNotImplemented("extend")
	},
}

var archiveCmd = &cobra.Command{
	Use:   "archive",
	Short: "Archive events older than 6 months",
	RunE: func(cmd *cobra.Command, args []string) error {
		return errNotImplemented("archive")
	},
}

var eventCmd = &cobra.Command{
	Use:   "event",
	Short: "Manage one-off events",
}

var eventNewCmd = &cobra.Command{
	Use:   "new",
	Short: "Create a new one-off event",
	RunE: func(cmd *cobra.Command, args []string) error {
		return errNotImplemented("event new")
	},
}

var eventListCmd = &cobra.Command{
	Use:   "list",
	Short: "List events in a range",
	RunE: func(cmd *cobra.Command, args []string) error {
		return errNotImplemented("event list")
	},
}

var eventShowCmd = &cobra.Command{
	Use:   "show <path>",
	Short: "Show one event",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return errNotImplemented("event show")
	},
}

var seriesCmd = &cobra.Command{
	Use:   "series",
	Short: "Manage recurring series",
}

var seriesNewCmd = &cobra.Command{
	Use:   "new",
	Short: "Create a new recurring series",
	RunE: func(cmd *cobra.Command, args []string) error {
		return errNotImplemented("series new")
	},
}

var seriesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all recurring series",
	RunE: func(cmd *cobra.Command, args []string) error {
		return errNotImplemented("series list")
	},
}

var seriesShowCmd = &cobra.Command{
	Use:   "show <id-or-slug>",
	Short: "Show a recurring series",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return errNotImplemented("series show")
	},
}

var seriesExceptCmd = &cobra.Command{
	Use:   "except <id-or-slug> <date>",
	Short: "Add a date to a series' exceptions list",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return errNotImplemented("series except")
	},
}

func init() {
	eventCmd.AddCommand(eventNewCmd, eventListCmd, eventShowCmd)
	seriesCmd.AddCommand(seriesNewCmd, seriesListCmd, seriesShowCmd, seriesExceptCmd)

	eventListCmd.Flags().String("range", "week", "date range: today | week | month | all")
}
