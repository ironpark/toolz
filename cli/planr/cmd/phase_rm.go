package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	"github.com/ironpark/toolz/cli/planr/internal/planlock"
	"github.com/ironpark/toolz/cli/planr/internal/planstore"
	"github.com/ironpark/toolz/cli/planr/internal/vfs"
	ucli "github.com/urfave/cli/v3"
)

func runRemovePhase(_ context.Context, cmd *ucli.Command) error {
	if cmd.NArg() != 2 {
		return fmt.Errorf("phase rm requires <plan-name> <phase-number>")
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
	planRoot, planDirectory, err := plan.FindDirectory(planDirectories, cmd.Args().First())
	if err != nil {
		return err
	}
	planLock, err := planlock.AcquirePlan(planRoot)
	if err != nil {
		return err
	}
	defer planLock.Close()

	planPath := filepath.Join(planRoot, "PLAN.md")
	planRaw, err := vfs.ReadFile(planPath)
	if err != nil {
		return err
	}
	planFront, planBody, err := mdoc.Split(string(planRaw))
	if err != nil {
		return fmt.Errorf("parse %s/PLAN.md: %w", planDirectory, err)
	}
	if mdoc.FrontString(planFront, "plan_status") == draft.StatusDone {
		return fmt.Errorf("plan %q is already done; phase rm is only allowed for open plans", planDirectory)
	}

	phases, err := plan.ReadPhases(planRoot)
	if err != nil {
		return err
	}
	phasePath, err := plan.FindPhaseFile(planRoot, phaseID)
	if err != nil {
		return fmt.Errorf("%s: %w", planDirectory, err)
	}
	phaseRaw, err := vfs.ReadFile(phasePath)
	if err != nil {
		return err
	}
	if dependents := phaseDependents(phases, planDirectory, phaseID); len(dependents) > 0 && !cmd.Bool("force") {
		return fmt.Errorf("cannot remove %s phase %02d because %s depend on it; use --force",
			planDirectory, phaseID, formatPhaseDependents(dependents))
	}
	updatedBody, err := removePhaseChecklist(planBody, phaseID)
	if err != nil {
		return fmt.Errorf("update %s/PLAN.md phase checklist: %w", planDirectory, err)
	}

	remaining := make([]plan.StoredPhase, 0, len(phases)-1)
	for _, phase := range phases {
		if phase.ID != phaseID {
			remaining = append(remaining, phase)
		}
	}
	completed := len(remaining) > 0
	for _, phase := range remaining {
		if phase.Status != draft.StatusDone {
			completed = false
			break
		}
	}
	if completed {
		planFront["plan_status"] = draft.StatusDone
		planFront["completed_at"] = plan.CompletionTimestamp()
	} else {
		planFront["plan_status"] = draft.StatusInProgress
		delete(planFront, "completed_at")
	}

	updatedPlan, err := mdoc.Render(planFront, updatedBody)
	if err != nil {
		return err
	}
	if err := planstore.Apply(
		planstore.Update(planPath, string(planRaw), updatedPlan),
		planstore.Delete(phasePath, string(phaseRaw)),
	); err != nil {
		return err
	}
	fmt.Printf("Removed %s phase %02d: %s\n", planDirectory, phaseID, phasePath)
	if completed {
		fmt.Printf("Plan %s marked done\n", planDirectory)
	}
	return nil
}

func phaseDependents(phases []plan.StoredPhase, planDirectory string, phaseID int) []plan.StoredPhase {
	planName := draft.PlanName(planDirectory)
	dependents := []plan.StoredPhase{}
	for _, phase := range phases {
		if phase.ID == phaseID {
			continue
		}
		for _, raw := range phase.Dependencies {
			dependency, err := draft.ParseDependency(strings.TrimSpace(raw))
			if err == nil && dependency.Plan == planName && dependency.Phase != nil && *dependency.Phase == phaseID {
				dependents = append(dependents, phase)
				break
			}
		}
	}
	return dependents
}

func formatPhaseDependents(phases []plan.StoredPhase) string {
	values := make([]string, len(phases))
	for index, phase := range phases {
		values[index] = fmt.Sprintf("phase %02d %q", phase.ID, phase.Title)
	}
	return strings.Join(values, ", ")
}

func removePhaseChecklist(body string, phaseID int) (string, error) {
	return plan.TransformChecklistEntry(body, phaseID, func(string) (string, bool) {
		return "", true
	})
}
