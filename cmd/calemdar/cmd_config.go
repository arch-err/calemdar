package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open the config file in $EDITOR; validate on save",
	RunE:  runConfigEdit,
}

const configStub = `# calemdar config — see examples/config.yaml in the repo for every key.
# At minimum, set vault: to the absolute path of your Obsidian vault.

vault: ""
`

func runConfigEdit(cmd *cobra.Command, args []string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		return fmt.Errorf("$EDITOR not set")
	}

	path, err := config.Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(path, []byte(configStub), 0o644); err != nil {
			return fmt.Errorf("write stub: %w", err)
		}
		fmt.Fprintf(os.Stderr, "created stub at %s\n", path)
	}

	// Support "EDITOR=code --wait" or "EDITOR=emacsclient -nw" etc.
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return fmt.Errorf("$EDITOR empty after split")
	}
	cmdArgs := append(append([]string{}, parts[1:]...), path)
	c := exec.Command(parts[0], cmdArgs...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		// Editor exited non-zero — validate anyway; the user may have saved
		// before quitting.
		fmt.Fprintf(os.Stderr, "editor exited: %v (validating file anyway)\n", err)
	}

	// Validate the post-edit state.
	fresh, err := config.Load()
	if err != nil {
		return fmt.Errorf("invalid after edit:\n  %w\n\nfix the file and re-run `calemdar config edit`", err)
	}
	config.Active = fresh
	fmt.Println("config ok.")
	return nil
}

func init() {
	configCmd.AddCommand(configPathCmd, configShowCmd, configEditCmd)
}
