package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	"github.com/urfave/cli/v3"
)

func phaseRemoveCommand(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() != 2 {
		return fmt.Errorf("phase rm requires <planName-name> <phase-number>")
	}
	phaseID, err := strconv.Atoi(cmd.Args().Get(1))
	if err != nil || phaseID < 0 {
		return fmt.Errorf("phase number %q must be a non-negative integer", cmd.Args().Get(1))
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	settings, repoRoot, err := config.Load(cwd)
	if err != nil {
		return err
	}
	planDirectories := settings.PlanDirs(repoRoot)
	planRoot, planDirectory, err := findPlanDirectory(planDirectories, cmd.Args().First())
	if err != nil {
		return err
	}
	planLock, err := acquirePlanLock(planRoot)
	if err != nil {
		return err
	}
	defer planLock.close()

	planPath := filepath.Join(planRoot, "PLAN.md")
	planRaw, err := os.ReadFile(planPath)
	if err != nil {
		return err
	}
	planFront, planBody, err := mdoc.Split(string(planRaw))
	if err != nil {
		return fmt.Errorf("parse %s/PLAN.md: %w", planDirectory, err)
	}
	if status, _ := planFront["plan_status"].(string); status == "done" {
		return fmt.Errorf("planName %q is already done; phase rm is only allowed for open plans", planDirectory)
	}

	phases, err := readPlanPhases(planRoot)
	if err != nil {
		return err
	}
	phasePath, err := findPhaseFile(planRoot, phaseID)
	if err != nil {
		return fmt.Errorf("%s: %w", planDirectory, err)
	}
	if dependents := phaseDependents(phases, planDirectory, phaseID); len(dependents) > 0 && !cmd.Bool("force") {
		return fmt.Errorf("cannot remove %s phase %02d because %s depend on it; use --force",
			planDirectory, phaseID, formatPhaseDependents(dependents))
	}
	updatedBody, err := removePhaseChecklist(planBody, phaseID)
	if err != nil {
		return fmt.Errorf("update %s/PLAN.md phase checklist: %w", planDirectory, err)
	}

	remaining := make([]storedPhase, 0, len(phases)-1)
	for _, phase := range phases {
		if phase.id != phaseID {
			remaining = append(remaining, phase)
		}
	}
	completed := len(remaining) > 0
	for _, phase := range remaining {
		if phase.status != "done" {
			completed = false
			break
		}
	}
	if completed {
		planFront["plan_status"] = "done"
		planFront["completed_at"] = completionTimestamp()
	} else {
		planFront["plan_status"] = "in-progress"
		delete(planFront, "completed_at")
	}

	if err := mdoc.WriteFile(planPath, planFront, updatedBody); err != nil {
		return err
	}
	if err := os.Remove(phasePath); err != nil {
		// Keep the documents consistent if the filesystem refuses the removal.
		if restoreErr := mdoc.WriteAtomically(planPath, string(planRaw)); restoreErr != nil {
			return fmt.Errorf("remove %s: %w; restore PLAN.md: %v", filepath.Base(phasePath), err, restoreErr)
		}
		return fmt.Errorf("remove %s: %w", filepath.Base(phasePath), err)
	}
	fmt.Printf("Removed %s phase %02d: %s\n", planDirectory, phaseID, phasePath)
	if completed {
		fmt.Printf("Plan %s marked done\n", planDirectory)
	}
	return nil
}

func phaseDependents(phases []storedPhase, planDirectory string, phaseID int) []storedPhase {
	planName := plan.Name(planDirectory)
	dependents := []storedPhase{}
	for _, phase := range phases {
		if phase.id == phaseID {
			continue
		}
		for _, raw := range phase.dependencies {
			dependency, err := plan.ParseDependency(strings.TrimSpace(raw))
			if err == nil && dependency.Plan == planName && dependency.Phase != nil && *dependency.Phase == phaseID {
				dependents = append(dependents, phase)
				break
			}
		}
	}
	return dependents
}

func formatPhaseDependents(phases []storedPhase) string {
	values := make([]string, len(phases))
	for index, phase := range phases {
		values[index] = fmt.Sprintf("phase %02d %q", phase.id, phase.title)
	}
	return strings.Join(values, ", ")
}

func removePhaseChecklist(body string, phaseID int) (string, error) {
	return transformChecklistEntry(body, phaseID, func(string) (string, bool) {
		return "", true
	})
}
