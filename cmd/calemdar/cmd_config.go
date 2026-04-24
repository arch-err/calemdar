package main

import (
	"fmt"
	"os"

	"github.com/arch-err/calemdar/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show config file path + active values",
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the config file lookup path",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := config.Path()
		if err != nil {
			return err
		}
		fmt.Println(p)
		return nil
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the active (post-merge) configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		enc := yaml.NewEncoder(os.Stdout)
		enc.SetIndent(2)
		defer enc.Close()
		return enc.Encode(config.Active)
	},
}

func init() {
	configCmd.AddCommand(configPathCmd, configShowCmd)
}
