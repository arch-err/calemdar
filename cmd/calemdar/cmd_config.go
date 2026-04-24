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
		path, _ := config.Path()
		if _, err := os.Stat(path); os.IsNotExist(err) {
			fmt.Printf("# no config file at %s — showing defaults only\n", path)
		} else {
			fmt.Printf("# active config (defaults merged with %s)\n", path)
		}
		enc := yaml.NewEncoder(os.Stdout)
		enc.SetIndent(2)
		defer enc.Close()
		return enc.Encode(config.Active)
	},
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Write a default config file (errors if one already exists)",
	RunE:  runConfigInit,
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open the config file in $EDITOR; validate on save",
	RunE:  runConfigEdit,
}

func runConfigInit(cmd *cobra.Command, args []string) error {
	path, err := config.Path()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists — edit it with `calemdar config edit`", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}
	stub, err := buildStub()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, stub, 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s\nnext: `calemdar config edit` to set vault and tweak\n", path)
	return nil
}

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
		stub, err := buildStub()
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, stub, 0o644); err != nil {
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

// buildStub materialises config.Defaults() as a YAML blob with a leading
// header. Ensures the first `config edit` shows the same view as
// `config show` rather than a surprising blank file.
func buildStub() ([]byte, error) {
	var body strings.Builder
	body.WriteString("# calemdar config — edit freely. Validation runs on save.\n")
	body.WriteString("# See examples/config.yaml in the repo for key-by-key docs.\n")
	body.WriteString("#\n")
	body.WriteString("# vault is REQUIRED for the daemon. Set it to your Obsidian vault path.\n\n")

	enc := yaml.NewEncoder(&yamlBuffer{s: &body})
	enc.SetIndent(2)
	if err := enc.Encode(config.Defaults()); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return []byte(body.String()), nil
}

// yamlBuffer adapts a strings.Builder to the io.Writer the yaml encoder wants.
type yamlBuffer struct{ s *strings.Builder }

func (b *yamlBuffer) Write(p []byte) (int, error) { return b.s.Write(p) }

func init() {
	configCmd.AddCommand(configPathCmd, configShowCmd, configInitCmd, configEditCmd)
}
