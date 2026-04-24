package main

import (
	"github.com/spf13/cobra"
)

// applyHelpStyling sets a colored usage template on root and refreshes the
// Long description so the wordmark uses ANSI gray on "md" instead of
// asterisks.
func applyHelpStyling(root *cobra.Command) {
	root.Long = appName() + " — Obsidian Full Calendar recurring-event manager.\n\n" +
		"Expands recurring templates into individual per-occurrence markdown\n" +
		"files so Full Calendar sees only flat single events. Drag, drop, edit\n" +
		"without worry — the plugin's recurring-event footgun never fires."

	root.SetUsageTemplate(usageTemplate)
}

// usageTemplate is a lightly-colored take on cobra's default usage template.
// Section headers are bold cyan; flag summaries keep default tone; command
// names are yellow. Aliases and examples use gray.
//
// Template authoring notes:
//   - {{ cy "Usage:" }} uses the custom funcmap (registered in init).
//   - {{trimTrailingWhitespaces}} is built into cobra.
var usageTemplate = `{{cy "Usage:"}}{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

{{cy "Aliases:"}}
  {{gr .NameAndAliases}}{{end}}{{if .HasExample}}

{{cy "Examples:"}}
{{gr .Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

{{cy "Available Commands:"}}{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{yl (rpad .Name .NamePadding)}} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{cy (printf "%s:" $group.Title)}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{yl (rpad .Name .NamePadding)}} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

{{cy "Additional Commands:"}}{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{yl (rpad .Name .NamePadding)}} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

{{cy "Flags:"}}
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

{{cy "Global Flags:"}}
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

{{cy "Additional help topics:"}}{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{yl (rpad .CommandPath .CommandPathPadding)}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

{{gr (printf "Use \"%s [command] --help\" for more information about a command." .CommandPath)}}{{end}}
`
