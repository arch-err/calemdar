package main

import (
	"fmt"

	"github.com/arch-err/calemdar/internal/config"
	"github.com/arch-err/calemdar/internal/vault"
	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Create the calendar subfolders in your vault (idempotent)",
	Long: `Creates <vault>/<base_path>/{recurring, archive, events/<each calendar>}
if they don't already exist. Safe to run multiple times — existing
directories are skipped. The serve daemon also does this at startup.`,
	RunE: runSetup,
}

func runSetup(cmd *cobra.Command, args []string) error {
	v, err := resolveVault(cmd)
	if err != nil {
		return err
	}
	rep, err := vault.Scaffold(v, config.Active.Calendars)
	if err != nil {
		return err
	}
	for _, p := range rep.Created {
		fmt.Printf("created  %s\n", p)
	}
	for _, p := range rep.Existed {
		fmt.Printf("existed  %s\n", p)
	}
	return nil
}
