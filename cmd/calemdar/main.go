package main

import (
	"fmt"
	"os"

	"github.com/arch-err/calemdar/internal/vault"
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
	rootCmd.PersistentFlags().String("vault", "", "vault root path (or $"+vault.EnvVar+")")

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

func resolveVault(cmd *cobra.Command) (*vault.Vault, error) {
	override, _ := cmd.Flags().GetString("vault")
	return vault.Resolve(override)
}
