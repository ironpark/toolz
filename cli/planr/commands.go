package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/hooks"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	"github.com/urfave/cli/v3"
)

func newCommand(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() < 1 || cmd.NArg() > 2 {
		return fmt.Errorf("new requires <plan-name> and a short description")
	}
	selector := cmd.Args().First()
	if strings.Contains(selector, "#") {
		if cmd.NArg() != 1 {
			return fmt.Errorf("phase draft selector must be the only positional argument")
		}
		return newPhaseCommand(cmd, selector)
	}
	return newPlanCommand(cmd)
}

func newPlanCommand(cmd *cli.Command) error {
	name := cmd.Args().First()
	if !draft.KebabPattern.MatchString(name) {
		return fmt.Errorf("plan name %q must be lowercase kebab-case", name)
	}
	descriptionInput := cmd.String("description")
	if cmd.NArg() == 2 {
		if descriptionInput != "" {
			return fmt.Errorf("description must be provided either as the second argument or with --description, not both")
		}
		descriptionInput = cmd.Args().Get(1)
	}
	description, err := draft.RequireDescription(descriptionInput)
	if err != nil {
		return err
	}
	output := cmd.String("output")
	if output == "" {
		output = name + ".md"
	}
	absOutput, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if !cmd.Bool("json") {
		if _, err := os.Stat(absOutput); err == nil {
			return fmt.Errorf("draft file already exists: %s", absOutput)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	dependsOn, err := draft.NormalizeDependencies(cmd.StringSlice("depends-on"), name)
	if err != nil {
		return fmt.Errorf("invalid dependencies for plan %q: %w", name, err)
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return err
	}
	settings, repoRoot, err := config.Load(workingDirectory)
	if err != nil {
		return err
	}
	settings = commandSettings(settings, cmd)
	if err := runDocumentHooks(repoRoot, settings, "before", hooks.EventNew, name, -1, "draft", cmd.Bool("json")); err != nil {
		return err
	}
	rendered, err := doc.RenderNewDraft(settings.Language, name, dependsOn, description)
	if err != nil {
		return err
	}
	if cmd.Bool("json") {
		if err := writeJSON(makeTemplateJSON("plan", name, rendered)); err != nil {
			return err
		}
	} else {
		if err := os.WriteFile(absOutput, []byte(rendered), 0644); err != nil {
			return err
		}
		fmt.Printf("Created %s\n", absOutput)
	}
	if err := runDocumentHooks(repoRoot, settings, "after", hooks.EventNew, name, -1, "draft", cmd.Bool("json")); err != nil {
		return err
	}
	return nil
}

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

func statusCommand(_ context.Context, cmd *cli.Command) error {
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
