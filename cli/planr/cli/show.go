package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/jsonout"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	ucli "github.com/urfave/cli/v3"
)

func showCommand(_ context.Context, cmd *ucli.Command) error {
	if cmd.NArg() < 1 || cmd.NArg() > 2 {
		return fmt.Errorf("show requires <plan-name> and optionally <phase-number>")
	}
	section := strings.TrimSpace(cmd.String("section"))
	if section != "" && !plan.ValidSection(section) {
		return fmt.Errorf("invalid show section %q; use goals, context, or plan", section)
	}
	if (section != "" || cmd.Bool("all")) && cmd.NArg() != 1 {
		return fmt.Errorf("show --section and --all accept only a plan name")
	}
	if section != "" && cmd.Bool("all") {
		return fmt.Errorf("show accepts either --section or --all, not both")
	}
	planArg := cmd.Args().First()

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	planDirectories, err := config.PlanPaths(cwd)
	if err != nil {
		return err
	}
	planRoot, planDirectory, err := plan.FindDirectory(planDirectories, planArg)
	if err != nil {
		return err
	}
	if section != "" {
		return showPlanSection(planRoot, planDirectory, section, cmd.Bool("json"))
	}
	if cmd.Bool("all") {
		return showAllPlan(planRoot, planDirectory, cmd.Bool("json"))
	}
	phases, err := plan.ReadPhases(planRoot)
	if err != nil {
		return err
	}

	phaseID := -1
	if cmd.NArg() == 2 {
		phaseID, err = strconv.Atoi(cmd.Args().Get(1))
		if err != nil || phaseID < 0 {
			return fmt.Errorf("phase number %q must be a non-negative integer", cmd.Args().Get(1))
		}
	} else {
		for _, phase := range phases {
			if phase.Status != "done" {
				phaseID = phase.ID
				break
			}
		}
		if phaseID < 0 {
			return fmt.Errorf("plan %q has no unfinished phases", planDirectory)
		}
	}

	var stored plan.StoredPhase
	found := false
	for _, phase := range phases {
		if phase.ID == phaseID {
			stored = phase
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%s: phase %02d not found", planDirectory, phaseID)
	}

	details, err := plan.ReadPhaseDetails(planRoot, planDirectory, stored)
	if err != nil {
		return err
	}
	if cmd.Bool("json") {
		return jsonout.Write(jsonout.Show(details))
	}

	fmt.Printf("Phase %02d: %s\n", details.ID, details.Title)
	fmt.Printf("status: %s\n", details.Status)
	printShowBody("planned_work", details.PlannedWork)
	printShowBody("done_when", details.DoneWhen)
	printShowList("depends_on", details.Dependencies)
	fmt.Printf("file: %s\n", details.File)
	return nil
}

func showPlanSection(planRoot, planDirectory, section string, jsonOutput bool) error {
	details, err := plan.ReadSection(planRoot, planDirectory, section)
	if err != nil {
		return err
	}
	if jsonOutput {
		return jsonout.Write(jsonout.ShowSection(details))
	}
	fmt.Print(details.Content)
	return nil
}

func showAllPlan(planRoot, planDirectory string, jsonOutput bool) error {
	if !jsonOutput {
		return fmt.Errorf("show --all requires --json")
	}
	details, err := plan.ReadAll(planRoot, planDirectory)
	if err != nil {
		return err
	}
	return jsonout.Write(jsonout.ShowAll(details))
}

func printShowBody(label, body string) {
	fmt.Printf("%s:\n", label)
	for _, line := range strings.Split(body, "\n") {
		fmt.Printf("  %s\n", line)
	}
}

func printShowList(label string, values []string) {
	if len(values) == 0 {
		fmt.Printf("%s: []\n", label)
		return
	}
	fmt.Printf("%s:\n", label)
	for _, value := range values {
		fmt.Printf("  - %s\n", value)
	}
}
