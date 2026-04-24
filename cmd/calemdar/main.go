package main

import (
	"fmt"
	"os"

	"github.com/arch-err/calemdar/internal/config"
	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/vault"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "calemdar",
	Short: "Recurring-event manager for Obsidian Full Calendar",
	Long: `cale**md**ar expands recurring event templates into individual
per-occurrence markdown files, so Obsidian's Full Calendar plugin sees
only flat single events and its drag-a-recurring-event footgun never fires.`,
	// Loads config once, before any subcommand runs.
	PersistentPreRunE: loadConfig,
}

func main() {
	rootCmd.PersistentFlags().String("vault", "", "vault root path (overrides config + env)")

	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(reindexCmd)
	rootCmd.AddCommand(expandCmd)
	rootCmd.AddCommand(extendCmd)
	rootCmd.AddCommand(archiveCmd)
	rootCmd.AddCommand(reactorCmd)
	rootCmd.AddCommand(eventCmd)
	rootCmd.AddCommand(seriesCmd)
	rootCmd.AddCommand(configCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// loadConfig runs before every subcommand. Loads + validates the config
// file (optional, defaults used on absence), then applies runtime-wide
// settings (timezone, calendar list).
func loadConfig(cmd *cobra.Command, args []string) error {
	if err := config.LoadAndApply(); err != nil {
		return err
	}
	if loc, err := model.ResolvedLocation(config.Active.Timezone); err == nil {
		model.SetTimezone(loc)
	}
	model.SetCalendars(config.Active.Calendars)
	return nil
}

// resolveVault returns the active vault. Precedence:
//  1. --vault flag
//  2. $CALEMDAR_VAULT
//  3. config.Active.Vault
func resolveVault(cmd *cobra.Command) (*vault.Vault, error) {
	if override, _ := cmd.Flags().GetString("vault"); override != "" {
		return vault.Resolve(override)
	}
	if env := os.Getenv(vault.EnvVar); env != "" {
		return vault.Resolve(env)
	}
	if config.Active.Vault != "" {
		return vault.Resolve(config.Active.Vault)
	}
	return nil, fmt.Errorf("vault not configured: set in %s, $%s, or --vault flag",
		configPathOrLiteral(), vault.EnvVar)
}

func configPathOrLiteral() string {
	p, err := config.Path()
	if err != nil {
		return "~/.config/calemdar/config.yaml"
	}
	return p
}
