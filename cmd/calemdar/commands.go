package main

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/arch-err/calemdar/internal/expand"
	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/series"
	"github.com/arch-err/calemdar/internal/writer"
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
	Use:   "expand <id-or-slug>",
	Short: "Force-expand a single series",
	Args:  cobra.ExactArgs(1),
	RunE:  runExpand,
}

func runExpand(cmd *cobra.Command, args []string) error {
	v, err := resolveVault(cmd)
	if err != nil {
		return err
	}
	r, err := series.FindByIDOrSlug(v, args[0])
	if err != nil {
		return err
	}
	if r == nil {
		return fmt.Errorf("series %q not found", args[0])
	}

	loc := model.Stockholm()
	today := model.Today(loc)
	end := today.AddDate(0, 12, 0)

	events, err := expand.Expand(r, today, end, time.Now())
	if err != nil {
		return err
	}

	var created, updated, skipped int
	for _, e := range events {
		e.Path = v.EventPath(r.Calendar, e.Date, r.Slug)

		if existing, err := model.ParseEvent(e.Path); err == nil {
			if existing.UserOwned {
				skipped++
				continue
			}
			updated++
		} else {
			created++
		}

		if err := writer.WriteEvent(e); err != nil {
			return fmt.Errorf("write %s: %w", e.Path, err)
		}
	}

	fmt.Printf("series %s (%s): %d events — created %d, updated %d, skipped %d (user-owned)\n",
		r.Slug, r.ID, len(events), created, updated, skipped)
	return nil
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
	RunE:  runSeriesList,
}

func runSeriesList(cmd *cobra.Command, args []string) error {
	v, err := resolveVault(cmd)
	if err != nil {
		return err
	}
	roots, err := series.LoadAll(v)
	if err != nil {
		return err
	}
	if len(roots) == 0 {
		fmt.Println("no recurring series")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SLUG\tCALENDAR\tTITLE\tFREQ\tINTERVAL\tSTART\tUNTIL")
	for _, r := range roots {
		until := r.Until
		if until == "" {
			until = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			r.Slug, r.Calendar, r.Title, r.Freq, r.Interval, r.StartDate, until)
	}
	return w.Flush()
}

var seriesShowCmd = &cobra.Command{
	Use:   "show <id-or-slug>",
	Short: "Show a recurring series",
	Args:  cobra.ExactArgs(1),
	RunE:  runSeriesShow,
}

func runSeriesShow(cmd *cobra.Command, args []string) error {
	v, err := resolveVault(cmd)
	if err != nil {
		return err
	}
	r, err := series.FindByIDOrSlug(v, args[0])
	if err != nil {
		return err
	}
	if r == nil {
		return fmt.Errorf("series %q not found", args[0])
	}
	fmt.Printf("id:         %s\n", r.ID)
	fmt.Printf("slug:       %s\n", r.Slug)
	fmt.Printf("title:      %s\n", r.Title)
	fmt.Printf("calendar:   %s\n", r.Calendar)
	fmt.Printf("freq:       %s\n", r.Freq)
	fmt.Printf("interval:   %d\n", r.Interval)
	fmt.Printf("start-date: %s\n", r.StartDate)
	if r.Until != "" {
		fmt.Printf("until:      %s\n", r.Until)
	}
	if r.StartTime != "" {
		fmt.Printf("time:       %s–%s\n", r.StartTime, r.EndTime)
	}
	if len(r.ByDay) > 0 {
		fmt.Printf("byday:      %v\n", r.ByDay)
	}
	if len(r.ByMonthDay) > 0 {
		fmt.Printf("bymonthday: %v\n", r.ByMonthDay)
	}
	if len(r.Exceptions) > 0 {
		fmt.Printf("exceptions: %v\n", r.Exceptions)
	}
	fmt.Printf("path:       %s\n", r.Path)
	return nil
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
