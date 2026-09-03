package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/jsonout"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	ucli "github.com/urfave/cli/v3"
)

func newOverviewCommand() *ucli.Command {
	return &ucli.Command{
		Name:          "overview",
		Usage:         "show a concise overview of all plans",
		ArgsUsage:     "[plan-name]",
		Flags:         []ucli.Flag{jsonFlag()},
		ShellComplete: planNameShellComplete,
		Action:        runOverview,
	}
}

func runOverview(_ context.Context, cmd *ucli.Command) error {
	if cmd.NArg() > 1 {
		return fmt.Errorf("overview accepts at most one plan name")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	planDirectories, err := config.PlanPaths(cwd)
	if err != nil {
		return err
	}
	// Collect every plan before resolving dependencies. When a single plan is
	// requested, its dependencies may live outside the filtered result and
	// still need to be classified as done, in-progress, or missing correctly.
	summaries, _, err := plan.CollectSummaries(planDirectories, "")
	if err != nil {
		return err
	}
	plan.AnnotateWaits(summaries)
	if filter := cmd.Args().First(); filter != "" {
		matched := summaries[:0]
		for _, summary := range summaries {
			if summary.Name == filter || filepath.Base(summary.Label) == filter {
				matched = append(matched, summary)
			}
		}
		// A repository with no plans yet is an empty result, not a failure; only
		// an explicitly requested plan that does not exist is an error.
		if len(matched) == 0 {
			return fmt.Errorf("plan %q not found", filter)
		}
		summaries = matched
	}
	if cmd.Bool("json") {
		return jsonout.Write(jsonout.Overview(summaries))
	}
	if len(summaries) == 0 {
		fmt.Println("No plans found")
		return nil
	}
	printPlanGroups(summaries, func(name string, summary plan.Summary) {
		status := summary.Status
		if status == "" {
			status = "unknown"
		}
		done, total, next := summary.Progress()
		fmt.Printf("  %s: %s (%d/%d phases)", name, status, done, total)
		if next != "" {
			fmt.Printf("; next: %s", next)
		}
		fmt.Println()
		printPlanList("wait", summary.Wait)
	})
	return nil
}
