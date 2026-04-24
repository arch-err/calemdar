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
	// Loads config once, before any subcommand runs.
	PersistentPreRunE: loadConfig,
	// Don't print usage on runtime errors; cobra only shows Usage on flag
	// parse errors, which is what that message is for.
	SilenceUsage: true,
}

func main() {
	// Register template funcs before any template is rendered.
	cobra.AddTemplateFunc("cy", cyan)
	cobra.AddTemplateFunc("gr", gray)
	cobra.AddTemplateFunc("yl", yellow)
	applyHelpStyling(rootCmd)

	rootCmd.PersistentFlags().String("vault", "", "vault root path (overrides config + env)")

	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(reindexCmd)
	rootCmd.AddCommand(expandCmd)
	rootCmd.AddCommand(extendCmd)
	rootCmd.AddCommand(archiveCmd)
	rootCmd.AddCommand(reactorCmd)
	rootCmd.AddCommand(eventCmd)
	rootCmd.AddCommand(seriesCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(notifyCmd)

	// cobra prints "Error: ..." itself; we just need the non-zero exit.
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// loadConfig runs before every subcommand. Loads + validates the config
// file (optional, defaults used on absence), then applies runtime-wide
// settings (timezone, calendar list).
//
// For `config` subcommands we tolerate load errors — otherwise a broken
// config file would lock the user out of fixing it via `calemdar config edit`.
func loadConfig(cmd *cobra.Command, args []string) error {
	err := config.LoadAndApply()
	if err != nil && !isConfigSubcommand(cmd) {
		return err
	}
	if loc, lerr := model.ResolvedLocation(config.Active.Timezone); lerr == nil {
		model.SetTimezone(loc)
	}
	model.SetCalendars(config.Active.Calendars)
	return nil
}

func isConfigSubcommand(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == "config" {
			return true
		}
	}
	return false
}

// resolveVault returns the active vault. Precedence:
//  1. --vault flag
//  2. $CALEMDAR_VAULT
//  3. config.Active.Vault
func resolveVault(cmd *cobra.Command) (*vault.Vault, error) {
	var path string
	if override, _ := cmd.Flags().GetString("vault"); override != "" {
		path = override
	} else if env := os.Getenv(vault.EnvVar); env != "" {
		path = env
	} else if config.Active.Vault != "" {
		path = config.Active.Vault
	} else {
		return nil, fmt.Errorf("vault not configured: set in %s, $%s, or --vault flag",
			configPathOrLiteral(), vault.EnvVar)
	}
	v, err := vault.Resolve(path)
	if err != nil {
		return nil, err
	}
	v.BasePath = config.Active.BasePath
	return v, nil
}

func configPathOrLiteral() string {
	p, err := config.Path()
	if err != nil {
		return "~/.config/calemdar/config.yaml"
	}
	return p
}
