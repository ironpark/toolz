package apply

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	"github.com/ironpark/toolz/cli/planr/internal/planlock"
	"github.com/ironpark/toolz/cli/planr/internal/validation"
	"github.com/ironpark/toolz/cli/planr/internal/vfs"
)

// Edit applies an edit checkout back onto the document it was checked out
// from, after verifying that the document has not changed in the meantime.
func Edit(raw []byte, settings config.Config, repoRoot string, dryRun bool, output io.Writer) (Operation, error) {
	front, _, err := mdoc.Split(string(raw))
	if err != nil {
		return Operation{}, validation.Wrap(err, "frontmatter", "frontmatter")
	}
	selector, ok := front["planr_edit"].(string)
	if !ok || strings.TrimSpace(selector) == "" {
		detail := "edit document requires planr_edit in frontmatter"
		return Operation{}, validation.NewFailure(validation.Record{Rule: "edit_identity", Section: "frontmatter", Detail: detail}, detail)
	}
	targetValue, ok := front["planr_target"].(string)
	if !ok || strings.TrimSpace(targetValue) == "" {
		detail := "edit document requires planr_target in frontmatter"
		return Operation{}, validation.NewFailure(validation.Record{Rule: "target_required", Section: "frontmatter", Detail: detail}, detail)
	}
	base, ok := front["planr_base"].(string)
	if !ok || strings.TrimSpace(base) == "" {
		detail := "edit document requires mandatory planr_base in frontmatter; run planr edit again"
		return Operation{}, validation.NewFailure(validation.Record{Rule: "base_required", Detail: detail}, detail)
	}
	if !strings.HasPrefix(base, "sha256:") {
		detail := "planr_base must be a sha256 hash; run planr edit again"
		return Operation{}, validation.NewFailure(validation.Record{Rule: "base_invalid", Detail: detail}, detail)
	}
	decodedBase, decodeErr := hex.DecodeString(strings.TrimPrefix(base, "sha256:"))
	if decodeErr != nil || len(decodedBase) != sha256.Size {
		detail := "planr_base must be a sha256 hash; run planr edit again"
		return Operation{}, validation.NewFailure(validation.Record{Rule: "base_invalid", Detail: detail}, detail)
	}
	planArg, targetKind, phaseID, section, err := parseEditDocumentSelector(selector, front)
	if err != nil {
		return Operation{}, validation.NewFailure(validation.Record{Rule: "edit_selector", Section: "frontmatter", Detail: err.Error()}, err.Error())
	}
	planDirectories := settings.PlanDirs(repoRoot)
	planRoot, planDirectory, err := plan.FindDirectory(planDirectories, planArg)
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
	target, err := editTarget(planRoot, planDirectory, phaseID, section)
	if err != nil {
		return Operation{}, err
	}
	expectedTarget, err := RelativeTargetPath(repoRoot, targetValue)
	if err != nil {
		return Operation{}, err
	}
	actualTarget, err := RelativeTargetPath(repoRoot, target)
	if err != nil {
		return Operation{}, err
	}
	if expectedTarget != actualTarget {
		return Operation{}, fmt.Errorf("planr_target %q does not identify %s", targetValue, actualTarget)
	}
	currentRaw, err := vfs.ReadFile(target)
	if err != nil {
		return Operation{}, err
	}
	currentHash := mdoc.Hash(currentRaw)
	if currentHash != base {
		detail := fmt.Sprintf("cannot apply edit for %s: planr_base %s does not match the current on-disk document hash %s; run planr edit again", selector, base, currentHash)
		record := validation.Record{Rule: "base_mismatch", Detail: detail}
		if targetKind == TargetPhase {
			record.Phase = validation.IntPointer(phaseID)
		}
		return Operation{}, validation.NewFailure(record, detail)
	}
	if targetKind == TargetPhase {
		return phaseEdit(raw, front, currentRaw, target, planRoot, planDirectory, phaseID, dryRun, output)
	}
	return sectionEdit(raw, currentRaw, target, planDirectory, section, dryRun, output)
}

func phaseEdit(raw []byte, incomingFront map[string]any, currentRaw []byte, target, planRoot, planDirectory string, phaseID int, dryRun bool, output io.Writer) (Operation, error) {
	currentFront, currentBody, err := mdoc.Split(string(currentRaw))
	if err != nil {
		return Operation{}, fmt.Errorf("parse %s: %w", filepath.Base(target), err)
	}
	currentStatus, _ := currentFront["status"].(string)
	incomingStatus := mdoc.FrontString(incomingFront, "status")
	if incomingStatus != currentStatus {
		detail := fmt.Sprintf("cannot apply phase edit for %s phase %02d: status changed from %q to %q; use `planr phase %s` to change phase status", planDirectory, phaseID, currentStatus, incomingStatus, phaseStatusCommand(incomingStatus))
		return Operation{}, validation.NewFailure(validation.Record{Rule: "status_transition", Section: "frontmatter", Phase: validation.IntPointer(phaseID), Detail: detail}, detail)
	}
	if !plan.StatusValues[currentStatus] {
		detail := fmt.Sprintf("%s phase %02d has invalid status %q", planDirectory, phaseID, currentStatus)
		return Operation{}, validation.NewFailure(validation.Record{Rule: "status", Section: "frontmatter", Phase: validation.IntPointer(phaseID), Detail: detail}, detail)
	}
	if err := plan.ValidateStatusChange(incomingFront, currentStatus); err != nil {
		return Operation{}, validation.NewFailure(validation.Record{Rule: "status_metadata", Section: "frontmatter", Phase: validation.IntPointer(phaseID), Detail: err.Error()}, err.Error())
	}
	if value, found := incomingFront["planr_phase"]; found && fmt.Sprint(value) != strconv.Itoa(phaseID) {
		detail := fmt.Sprintf("edit document identifies phase %v, but target is phase %02d", value, phaseID)
		return Operation{}, validation.NewFailure(validation.Record{Rule: "phase_identity", Section: "frontmatter", Phase: validation.IntPointer(phaseID), Detail: detail}, detail)
	}
	phases, err := plan.ReadPhases(planRoot)
	if err != nil {
		return Operation{}, err
	}
	meta, normalizedFront, err := editablePhaseMeta(incomingFront, planDirectory, phaseID, phases)
	if err != nil {
		if len(validation.Records(err)) > 0 {
			return Operation{}, err
		}
		return Operation{}, validation.NewFailure(validation.Record{Rule: "phase_metadata", Section: "frontmatter", Phase: validation.IntPointer(phaseID), Detail: err.Error()}, err.Error())
	}
	if value, found := incomingFront["planr_slug"]; found && fmt.Sprint(value) != meta.Slug {
		detail := fmt.Sprintf("edit document identifies slug %q, but target is %q", value, meta.Slug)
		return Operation{}, validation.NewFailure(validation.Record{Rule: "phase_identity", Section: "frontmatter", Phase: validation.IntPointer(phaseID), Detail: detail}, detail)
	}
	if value, found := incomingFront["slug"]; found {
		if fmt.Sprint(value) != meta.Slug {
			detail := fmt.Sprintf("edit document cannot change phase slug from %q to %q", meta.Slug, value)
			return Operation{}, validation.NewFailure(validation.Record{Rule: "phase_identity", Section: "frontmatter", Phase: validation.IntPointer(phaseID), Detail: detail}, detail)
		}
	}
	if meta.Slug != plan.PhaseSlug(phases, phaseID) {
		detail := "phase edit cannot change the phase slug"
		return Operation{}, validation.NewFailure(validation.Record{Rule: "phase_identity", Section: "frontmatter", Phase: validation.IntPointer(phaseID), Detail: detail}, detail)
	}
	title := mdoc.Title(mdoc.Body(raw))
	if title == "unnamed phase" {
		detail := "phase document must contain a Markdown title"
		return Operation{}, validation.NewFailure(validation.Record{Rule: "phase_document", Section: "phase", Phase: validation.IntPointer(phaseID), Detail: detail}, detail)
	}
	planned, completion, err := draft.SplitPhaseDocumentSections(title, mdoc.Body(raw))
	if err != nil {
		return Operation{}, validation.NewFailure(validation.Record{Rule: "phase_document", Section: "phase", Phase: validation.IntPointer(phaseID), Detail: err.Error()}, err.Error())
	}
	if planned == "" || completion == "" {
		detail := fmt.Sprintf("phase %q work and completion must not be empty", title)
		return Operation{}, validation.NewFailure(validation.Record{Rule: "phase_document", Section: "phase", Phase: validation.IntPointer(phaseID), Detail: detail}, detail)
	}
	for _, key := range editEnvelopeKeys {
		delete(normalizedFront, key)
	}
	normalizedFront["status"] = currentStatus
	if completedAt, found := currentFront["completed_at"]; found {
		normalizedFront["completed_at"] = completedAt
	} else {
		delete(normalizedFront, "completed_at")
	}
	newPhase, err := mdoc.Render(normalizedFront, mdoc.Body(raw))
	if err != nil {
		return Operation{}, err
	}
	documents := map[string]string{target: newPhase}
	diffs := []Diff{{Path: absolutePath(target), Before: string(currentRaw), After: newPhase}}
	if title != mdoc.Title(currentBody) {
		planPath := filepath.Join(planRoot, "PLAN.md")
		planRaw, readErr := vfs.ReadFile(planPath)
		if readErr != nil {
			return Operation{}, readErr
		}
		_, planBody, frontErr := mdoc.Split(string(planRaw))
		if frontErr != nil {
			return Operation{}, frontErr
		}
		updatedBody, updateErr := plan.ReplaceChecklistEntry(planBody, phaseID, title, meta.Slug, currentStatus == "done")
		if updateErr != nil {
			return Operation{}, updateErr
		}
		updatedPlan, renderErr := mdoc.WithBody(string(planRaw), updatedBody)
		if renderErr != nil {
			return Operation{}, renderErr
		}
		documents[planPath] = updatedPlan
		diffs = append(diffs, Diff{Path: absolutePath(planPath), Before: string(planRaw), After: updatedPlan})
	}
	op := makeOperation("edit_phase", draft.PlanName(planDirectory)+"#"+strconv.Itoa(phaseID), dryRun, documents, diffs)
	if dryRun || !op.Changed {
		return op, nil
	}
	if err := mdoc.WriteAtomically(target, newPhase); err != nil {
		return Operation{}, err
	}
	if planPath, ok := changedPlanPath(documents, target); ok {
		if err := mdoc.WriteAtomically(planPath, documents[planPath]); err != nil {
			return Operation{}, err
		}
	}
	fmt.Fprintf(output, "Updated %s\n", target)
	return op, nil
}

func changedPlanPath(documents map[string]string, phasePath string) (string, bool) {
	planPath := filepath.Join(filepath.Dir(filepath.Dir(phasePath)), "PLAN.md")
	_, found := documents[planPath]
	return planPath, found
}

func phaseStatusCommand(status string) string {
	switch status {
	case "in-progress":
		return "start"
	case "done":
		return "done"
	case "planned":
		return "reset"
	case "conditional":
		return "set --status conditional"
	default:
		return "set --status <status>"
	}
}

func editablePhaseMeta(front map[string]any, planDirectory string, phaseID int, phases []plan.StoredPhase) (draft.Meta, map[string]any, error) {
	slug := plan.PhaseSlug(phases, phaseID)
	meta := draft.Meta{Phase: phaseID, Slug: slug}
	status := mdoc.FrontString(front, "status")
	meta.Status = status
	if perf, ok := front["perf_phase"].(bool); ok {
		meta.PerfPhase = perf
	}
	if condition, ok := front["entry_condition"].(string); ok && strings.TrimSpace(condition) != "" {
		value := strings.TrimSpace(condition)
		meta.EntryCondition = &value
	}
	dependencies, normalized, err := editablePhaseDependencies(front["depends_on"], planDirectory, phases)
	if err != nil {
		return draft.Meta{}, nil, err
	}
	meta.DependsOn = dependencies
	normalizedFront := mdoc.CopyFront(front)
	normalizedFront["depends_on"] = normalized
	if err := validateNewPhaseEditDependencies(planDirectory, meta, phases); err != nil {
		return draft.Meta{}, nil, err
	}
	return meta, normalizedFront, nil
}

func editablePhaseDependencies(value any, planDirectory string, phases []plan.StoredPhase) ([]int, any, error) {
	var values []any
	switch typed := value.(type) {
	case []any:
		values = typed
	case []string:
		for _, item := range typed {
			values = append(values, item)
		}
	case nil:
		values = nil
	default:
		return nil, nil, fmt.Errorf("phase depends_on must be a list")
	}
	refs := make([]draft.Ref, 0, len(values))
	for _, item := range values {
		switch typed := item.(type) {
		case int:
			id := typed
			refs = append(refs, draft.Ref{Number: &id})
		case int64:
			id := int(typed)
			refs = append(refs, draft.Ref{Number: &id})
		case float64:
			if typed != float64(int(typed)) {
				return nil, nil, fmt.Errorf("phase dependency %v must be a whole phase number", typed)
			}
			id := int(typed)
			refs = append(refs, draft.Ref{Number: &id})
		case string:
			raw := strings.TrimSpace(typed)
			if dependency, err := draft.ParseDependency(raw); err == nil && dependency.Phase != nil {
				if dependency.Plan != draft.PlanName(planDirectory) {
					return nil, nil, fmt.Errorf("phase dependency %q must reference a phase in %s", raw, draft.PlanName(planDirectory))
				}
				refs = append(refs, draft.Ref{Number: dependency.Phase})
			} else if parsed, parseErr := strconv.Atoi(raw); parseErr == nil {
				id := parsed
				refs = append(refs, draft.Ref{Number: &id})
			} else {
				refs = append(refs, draft.Ref{Slug: raw})
			}
		default:
			return nil, nil, fmt.Errorf("phase depends_on entries must be phase numbers or strings")
		}
	}
	ids, err := resolvePhaseDraftDependencies(refs, phases)
	if err != nil {
		return nil, nil, err
	}
	normalized := make([]string, len(ids))
	for index, id := range ids {
		normalized[index] = fmt.Sprintf("%s#%d", planDirectory, id)
	}
	return ids, normalized, nil
}

func validateNewPhaseEditDependencies(planDirectory string, edited draft.Meta, phases []plan.StoredPhase) error {
	planName := draft.PlanName(planDirectory)
	all := make([]draft.Phase, 0, len(phases))
	for _, phase := range phases {
		if phase.ID == edited.Phase {
			all = append(all, draft.Phase{Title: phase.Title, Meta: edited})
			continue
		}
		converted, err := plan.StoredPhaseDraft(planName, phase)
		if err != nil {
			return err
		}
		all = append(all, converted)
	}
	return draft.ValidatePhaseDependencies(all)
}

func sectionEdit(raw []byte, currentRaw []byte, target, planDirectory, section string, dryRun bool, output io.Writer) (Operation, error) {
	_, incomingBody, err := mdoc.Split(string(raw))
	if err != nil {
		return Operation{}, err
	}
	_, currentBody, err := mdoc.Split(string(currentRaw))
	if err != nil {
		return Operation{}, err
	}
	var updatedBody string
	switch section {
	case "plan":
		incomingStart, incomingEnd, incomingFound := plan.ChecklistBounds(incomingBody)
		if !incomingFound || strings.TrimSpace(incomingBody[incomingStart:incomingEnd]) != ChecklistPlaceholder {
			detail := "plan section checkout must keep the derived checklist region unchanged"
			return Operation{}, validation.NewFailure(validation.Record{Rule: "derived_region", Section: "PLAN", Detail: detail}, detail)
		}
		start, end, found := plan.ChecklistBounds(currentBody)
		if !found {
			detail := "PLAN.md does not contain a # Phases section"
			return Operation{}, validation.NewFailure(validation.Record{Rule: "derived_region", Section: "PLAN", Detail: detail}, detail)
		}
		updatedBody = incomingBody[:incomingStart] + currentBody[start:end] + incomingBody[incomingEnd:]
	default:
		updatedBody = incomingBody
	}
	updated, err := mdoc.WithBody(string(currentRaw), updatedBody)
	if err != nil {
		return Operation{}, err
	}
	op := makeOperation("edit_"+section, draft.PlanName(planDirectory), dryRun,
		map[string]string{target: updated}, []Diff{{Path: absolutePath(target), Before: string(currentRaw), After: updated}})
	if dryRun || !op.Changed {
		return op, nil
	}
	fmt.Fprintf(output, "Updated %s\n", target)
	return op, mdoc.WriteAtomically(target, updated)
}
