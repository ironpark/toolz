package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	ucli "github.com/urfave/cli/v3"
)

// printPlanGroups prints each summary grouped by its plans directory, letting
// the caller render the per-plan detail lines.
func printPlanGroups(summaries []plan.Summary, render func(name string, summary plan.Summary)) {
	currentDirectory := ""
	for _, summary := range summaries {
		directory, name := filepath.Split(summary.Label)
		if directory != currentDirectory {
			fmt.Printf("%s\n", directory)
			currentDirectory = directory
		}
		render(name, summary)
	}
}

func printPlanList(title string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Printf("    %s:\n", title)
	for _, value := range values {
		fmt.Printf("      - %s\n", value)
	}
}

func statusCommand(_ context.Context, cmd *ucli.Command) error {
	if cmd.NArg() > 1 {
		return fmt.Errorf("status accepts at most one plan name")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	planDirectories, err := config.PlanPaths(cwd)
	if err != nil {
		return err
	}
	summaries, _, err := plan.CollectSummaries(planDirectories, cmd.Args().First())
	if err != nil {
		return err
	}
	// A repository with no plans yet is an empty result, not a failure; only an
	// explicitly requested plan that does not exist is an error.
	if len(summaries) == 0 {
		if filter := cmd.Args().First(); filter != "" {
			return fmt.Errorf("plan %q not found", filter)
		}
		if !cmd.Bool("json") {
			fmt.Println("No plans found")
			return nil
		}
	}
	requiredPlans := plan.AnnotateWaits(summaries)
	if cmd.NArg() == 0 {
		// Completed plans stay hidden unless an open plan still depends on them.
		visible := summaries[:0]
		for _, summary := range summaries {
			if summary.Status != "done" || requiredPlans[summary.Name] {
				visible = append(visible, summary)
			}
		}
		summaries = visible
	}
	if cmd.Bool("json") {
		return writeJSON(makeStatusJSON(summaries))
	}
	printPlanGroups(summaries, func(name string, summary plan.Summary) {
		done, total, _ := summary.Progress()
		fmt.Printf("  %s: %s (%d/%d phases done)\n", name, summary.Status, done, total)
		remaining := []string{}
		for _, phase := range summary.Phases {
			if phase.Status != "done" {
				remaining = append(remaining, fmt.Sprintf("%s (%s)", phase.Title, phase.Status))
			}
		}
		printPlanList("remaining", remaining)
		printPlanList("wait", summary.Wait)
	})
	return nil
}
