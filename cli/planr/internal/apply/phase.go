package apply

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/hooks"
	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	"github.com/ironpark/toolz/cli/planr/internal/planlock"
	"github.com/ironpark/toolz/cli/planr/internal/planstore"
	"github.com/ironpark/toolz/cli/planr/internal/validation"
	"github.com/ironpark/toolz/cli/planr/internal/vfs"
)

// Phase adds a new phase to an existing plan from a new-phase draft.
func Phase(d PhaseDraft, settings config.Config, repoRoot string, dryRun bool, output io.Writer) (Operation, error) {
	planDirectories := settings.PlanDirs(repoRoot)
	planRoot, planDirectory, err := plan.FindDirectory(planDirectories, d.Plan)
	if err != nil {
		return Operation{}, err
	}
	if !dryRun {
		lock, err := planlock.AcquirePlan(planRoot)
		if err != nil {
			return Operation{}, err
		}
		defer lock.Close()
	}

	planPath := filepath.Join(planRoot, "PLAN.md")
	planRaw, err := vfs.ReadFile(planPath)
	if err != nil {
		return Operation{}, err
	}
	planFront, planBody, err := mdoc.Split(string(planRaw))
	if err != nil {
		return Operation{}, fmt.Errorf("parse %s/PLAN.md: %w", planDirectory, err)
	}
	if mdoc.FrontString(planFront, "plan_status") == draft.StatusDone {
		detail := fmt.Sprintf("plan %q is already done; new phases can only be applied to open plans", planDirectory)
		return Operation{}, validation.NewFailure(validation.Record{Rule: "plan_done", Section: "frontmatter", Detail: detail}, detail)
	}
	phases, err := plan.ReadPhases(planRoot)
	if err != nil {
		return Operation{}, err
	}
	phaseID := plan.NextPhaseID(phases)
	for _, phase := range phases {
		if phase.Slug == d.Meta.Slug {
			detail := fmt.Sprintf("phase slug %q already exists in plan %q", d.Meta.Slug, planDirectory)
			return Operation{}, validation.NewFailure(validation.Record{Rule: "phase_slug_duplicate", Section: "frontmatter", Detail: detail}, detail)
		}
	}
	dependencies, err := resolvePhaseDraftDependencies(d.Meta.DependsOnRefs, phases)
	if err != nil {
		return Operation{}, err
	}
	meta := d.Meta
	meta.Phase = phaseID
	meta.DependsOn = dependencies
	if err := validateNewPhaseDependencies(planDirectory, meta, d.Title, phases); err != nil {
		return Operation{}, err
	}
	updatedPlanBody, err := plan.AppendChecklistEntry(planBody, phaseID, d.Title, meta.Slug)
	if err != nil {
		return Operation{}, fmt.Errorf("update %s/PLAN.md: %w", planDirectory, err)
	}
	updatedPlanFront := mdoc.CopyFront(planFront)
	updatedPlanFront["plan_status"] = draft.StatusInProgress
	delete(updatedPlanFront, "completed_at")
	phasePath := filepath.Join(planRoot, plan.PhaseDocumentPath(phaseID, meta.Slug))
	phaseContents, err := mdoc.Render(plan.PhaseFrontmatter(planDirectory, meta), plan.PhaseDocumentBody(settings.Language, d.Title, d.Planned, d.Completion))
	if err != nil {
		return Operation{}, err
	}
	updatedPlanContents, err := mdoc.Render(updatedPlanFront, updatedPlanBody)
	if err != nil {
		return Operation{}, err
	}
	documents := map[string]string{phasePath: phaseContents, planPath: updatedPlanContents}
	diffs := []Diff{{Path: absolutePath(phasePath), After: phaseContents}, {Path: absolutePath(planPath), Before: string(planRaw), After: updatedPlanContents}}
	op := makeOperation("add_phase", draft.PlanName(planDirectory)+"#"+d.Title, dryRun, documents, diffs)
	if dryRun {
		return op, nil
	}
	if err := hooks.Run(repoRoot, settings.Hooks, settings.SkipHooks, "before", hooks.EventPhaseAdd, planDirectory, phaseID, meta.Status, output); err != nil {
		return Operation{}, err
	}
	if err := vfs.MkdirAll(filepath.Join(planRoot, "phases"), 0755); err != nil {
		return Operation{}, err
	}
	if err := planstore.Apply(
		planstore.Create(phasePath, phaseContents),
		planstore.Update(planPath, string(planRaw), updatedPlanContents),
	); err != nil {
		return Operation{}, err
	}
	fmt.Fprintf(output, "Added %s phase %02d: %s\n", planDirectory, phaseID, phasePath)
	if err := hooks.Run(repoRoot, settings.Hooks, settings.SkipHooks, "after", hooks.EventPhaseAdd, planDirectory, phaseID, meta.Status, output); err != nil {
		return Operation{}, err
	}
	return op, nil
}

func resolvePhaseDraftDependencies(refs []draft.Ref, existing []plan.StoredPhase) ([]int, error) {
	bySlug := map[string]int{}
	known := make([]string, 0, len(existing))
	for _, phase := range existing {
		bySlug[phase.Slug] = phase.ID
		known = append(known, phase.Slug)
	}
	sort.Strings(known)
	dependencies := []int{}
	seen := map[int]bool{}
	for _, ref := range refs {
		id := -1
		if ref.Number != nil {
			id = *ref.Number
		} else {
			var found bool
			id, found = bySlug[ref.Slug]
			if !found {
				detail := fmt.Sprintf("phase dependency %q is neither a phase number nor a slug of an existing phase; available slugs: %s", ref.Slug, strings.Join(known, ", "))
				return nil, validation.NewFailure(validation.Record{Rule: "dependency_reference", Section: "frontmatter", Detail: detail}, detail)
			}
		}
		if id < 0 {
			detail := fmt.Sprintf("phase dependency %d must be a non-negative phase number", id)
			return nil, validation.NewFailure(validation.Record{Rule: "dependency_reference", Section: "frontmatter", Detail: detail}, detail)
		}
		if seen[id] {
			detail := fmt.Sprintf("phase dependency %d is listed more than once", id)
			return nil, validation.NewFailure(validation.Record{Rule: "dependency_duplicate", Section: "frontmatter", Detail: detail}, detail)
		}
		seen[id] = true
		dependencies = append(dependencies, id)
	}
	sort.Ints(dependencies)
	return dependencies, nil
}

func validateNewPhaseDependencies(planDirectory string, newPhase draft.Meta, title string, existing []plan.StoredPhase) error {
	all, err := plan.StoredPhaseDrafts(planDirectory, existing)
	if err != nil {
		return err
	}
	all = append(all, draft.Phase{Title: title, Meta: newPhase})
	if err := draft.ValidatePhaseDependencies(all); err != nil {
		message := fmt.Sprintf("invalid dependencies for new phase %d: %v", newPhase.Phase, err)
		records := validation.Records(err)
		if len(records) == 0 {
			records = []validation.Record{{Rule: "dependency", Section: "frontmatter", Phase: validation.IntPointer(newPhase.Phase), Detail: err.Error()}}
		}
		return validation.NewFailures(records, message)
	}
	return nil
}
