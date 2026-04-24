package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/arch-err/calemdar/internal/archive"
	"github.com/arch-err/calemdar/internal/model"
	"github.com/arch-err/calemdar/internal/reactor"
	"github.com/arch-err/calemdar/internal/reconcile"
	"github.com/arch-err/calemdar/internal/series"
	"github.com/arch-err/calemdar/internal/store"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the watcher + nightly timers (daemon)",
	RunE:  runServe,
}

var reindexCmd = &cobra.Command{
	Use:   "reindex",
	Short: "Rebuild SQLite cache from disk",
	RunE:  runReindex,
}

func runReindex(cmd *cobra.Command, args []string) error {
	v, err := resolveVault(cmd)
	if err != nil {
		return err
	}
	s, err := store.Open(v)
	if err != nil {
		return err
	}
	defer s.Close()
	rep, err := s.Reindex(v)
	if err != nil {
		return err
	}
	fmt.Printf("reindexed: %d series, %d occurrences\n", rep.Series, rep.Occurrences)
	return nil
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
	rep, err := reconcile.Series(v, r)
	if err != nil {
		return err
	}
	printReport(r, rep)
	return nil
}

func printReport(r *model.Root, rep *reconcile.Report) {
	fmt.Printf("series %s (%s): %d in plan — created %d, updated %d, skipped %d (user-owned), swept %d orphans\n",
		r.Slug, r.ID, rep.InPlan, rep.Created, rep.Updated, rep.Skipped, rep.Swept)
}

var extendCmd = &cobra.Command{
	Use:   "extend",
	Short: "Expand every recurring series (12-month horizon)",
	RunE:  runExtend,
}

func runExtend(cmd *cobra.Command, args []string) error {
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
	for _, r := range roots {
		rep, err := reconcile.Series(v, r)
		if err != nil {
			return err
		}
		printReport(r, rep)
	}
	return nil
}

var archiveCmd = &cobra.Command{
	Use:   "archive",
	Short: "Archive events older than 6 months",
	RunE:  runArchive,
}

func runArchive(cmd *cobra.Command, args []string) error {
	v, err := resolveVault(cmd)
	if err != nil {
		return err
	}
	rep, err := archive.Run(v)
	if err != nil {
		return err
	}
	fmt.Printf("archived %d events\n", rep.Moved)
	for _, p := range rep.Paths {
		fmt.Printf("  %s\n", p)
	}
	return nil
}

var eventCmd = &cobra.Command{
	Use:   "event",
	Short: "Manage one-off events",
}

var eventNewCmd = &cobra.Command{
	Use:   "new",
	Short: "Create a new one-off event (interactive)",
	RunE:  runEventNew,
}

var eventListCmd = &cobra.Command{
	Use:   "list",
	Short: "List events in a range",
	RunE:  runEventList,
}

var eventShowCmd = &cobra.Command{
	Use:   "show <path>",
	Short: "Show one event",
	Args:  cobra.ExactArgs(1),
	RunE:  runEventShow,
}

var seriesCmd = &cobra.Command{
	Use:   "series",
	Short: "Manage recurring series",
}

var seriesNewCmd = &cobra.Command{
	Use:   "new",
	Short: "Create a new recurring series (interactive)",
	RunE:  runSeriesNew,
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
	RunE:  runSeriesExcept,
}

var reactorCmd = &cobra.Command{
	Use:   "reactor",
	Short: "Scan events/ for FC-authored recurring events and migrate to recurring/",
	RunE:  runReactor,
}

func runReactor(cmd *cobra.Command, args []string) error {
	v, err := resolveVault(cmd)
	if err != nil {
		return err
	}
	migrations, err := reactor.Run(v)
	if err != nil {
		return err
	}
	if len(migrations) == 0 {
		fmt.Println("no FC-authored recurring events found")
		return nil
	}
	for _, m := range migrations {
		fmt.Printf("migrated: %s → %s\n", m.FromPath, m.ToPath)
		printReport(m.Series, m.Report)
	}
	return nil
}

func init() {
	eventCmd.AddCommand(eventNewCmd, eventListCmd, eventShowCmd)
	seriesCmd.AddCommand(seriesNewCmd, seriesListCmd, seriesShowCmd, seriesExceptCmd)

	eventListCmd.Flags().String("range", "week", "date range: today | week | month | all")
}
