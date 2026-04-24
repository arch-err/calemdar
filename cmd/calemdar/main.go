package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "calemdar",
	Short: "Recurring-event manager for Obsidian Full Calendar",
	Long: `cale**md**ar expands recurring event templates into individual
per-occurrence markdown files, so Obsidian's Full Calendar plugin sees
only flat single events and its drag-a-recurring-event footgun never fires.`,
}

func main() {
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(reindexCmd)
	rootCmd.AddCommand(expandCmd)
	rootCmd.AddCommand(extendCmd)
	rootCmd.AddCommand(archiveCmd)
	rootCmd.AddCommand(eventCmd)
	rootCmd.AddCommand(seriesCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
