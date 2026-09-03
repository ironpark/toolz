package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
	"github.com/ironpark/toolz/cli/planr/internal/planstore"
	"github.com/ironpark/toolz/cli/planr/internal/vfs"
)

// EnsureDependenciesMet refuses to advance a phase whose prerequisites are not
// done. It covers both the phase's own depends_on and the plan-level
// depends_on in PLAN.md, which is what `status` reports as `wait`. Resetting a
// phase to planned or conditional moves backwards and is never blocked.
func EnsureDependenciesMet(planDirectories []string, planRoot, planDirectory string, phaseID int, status string) error {
	if status != draft.StatusInProgress && status != draft.StatusDone {
		return nil
	}
	phases, err := ReadPhases(planRoot)
	if err != nil {
		return err
	}
	local := map[int]StoredPhase{}
	var target *StoredPhase
	for index, phase := range phases {
		local[phase.ID] = phase
		if phase.ID == phaseID {
			target = &phases[index]
		}
	}
	// A missing phase is reported by UpdatePhaseStatusLocked with a better message.
	if target == nil {
		return nil
	}

	unmet := []string{}
	for _, raw := range target.Dependencies {
		dependency, parseErr := draft.ParseDependency(raw)
		if parseErr != nil {
			unmet = append(unmet, fmt.Sprintf("%s (unreadable dependency)", raw))
			continue
		}
		if dependency.Plan == draft.PlanName(planDirectory) && dependency.Phase != nil {
			phase, found := local[*dependency.Phase]
			switch {
			case !found:
				unmet = append(unmet, fmt.Sprintf("phase %02d (not found)", *dependency.Phase))
			case phase.Status != draft.StatusDone:
				unmet = append(unmet, fmt.Sprintf("phase %02d %q (%s)", phase.ID, phase.Title, phase.Status))
			}
			continue
		}
		if reason := unmetDependency(planDirectories, dependency); reason != "" {
			unmet = append(unmet, reason)
		}
	}
	planDependencies, err := planLevelDependencies(planRoot)
	if err != nil {
		return err
	}
	for _, dependency := range planDependencies {
		if reason := unmetDependency(planDirectories, dependency); reason != "" {
			unmet = append(unmet, reason)
		}
	}
	if len(unmet) == 0 {
		return nil
	}
	lines := make([]string, len(unmet))
	for index, reason := range unmet {
		lines[index] = "  - " + reason
	}
	return fmt.Errorf("cannot set %s phase %02d to %s while its dependencies are unfinished:\n%s\nfinish them first or use --force",
		planDirectory, phaseID, status, strings.Join(lines, "\n"))
}

// unmetDependency describes why a dependency on another plan is not
// satisfied, or returns an empty string when it is. A dependency naming a plan
// that was never registered counts as unmet: drafts may reference plans that do
// not exist yet, but work cannot proceed past one.
func unmetDependency(planDirectories []string, dependency draft.Dependency) string {
	label := draft.DependencyLabel(dependency)
	planRoot, _, err := FindDirectory(planDirectories, dependency.Plan)
	if err != nil {
		return fmt.Sprintf("%s (not registered)", label)
	}
	if dependency.Phase == nil {
		done, err := AlreadyDone(planRoot)
		if err != nil {
			return fmt.Sprintf("%s (unreadable)", label)
		}
		if !done {
			return fmt.Sprintf("%s (in-progress)", label)
		}
		return ""
	}
	phases, err := ReadPhases(planRoot)
	if err != nil {
		return fmt.Sprintf("%s (unreadable)", label)
	}
	for _, phase := range phases {
		if phase.ID != *dependency.Phase {
			continue
		}
		if phase.Status != draft.StatusDone {
			return fmt.Sprintf("%s (%s)", label, phase.Status)
		}
		return ""
	}
	return fmt.Sprintf("%s (phase not found)", label)
}

// planLevelDependencies reads the depends_on list from a plan's PLAN.md.
func planLevelDependencies(planRoot string) ([]draft.Dependency, error) {
	front, _, err := ReadDocument(planRoot, "PLAN.md")
	if err != nil {
		return nil, err
	}
	dependencies := []draft.Dependency{}
	for _, raw := range mdoc.Strings(front["depends_on"]) {
		dependency, err := draft.ParseDependency(raw)
		if err != nil {
			continue
		}
		dependencies = append(dependencies, dependency)
	}
	return dependencies, nil
}

func WillComplete(planRoot string, phaseID int) (bool, error) {
	phases, err := ReadPhases(planRoot)
	if err != nil {
		return false, err
	}
	if len(phases) == 0 {
		return false, nil
	}
	found := false
	for _, phase := range phases {
		if phase.ID == phaseID {
			found = true
			continue
		}
		if phase.Status != draft.StatusDone {
			return false, nil
		}
	}
	if !found {
		return false, fmt.Errorf("phase %02d not found", phaseID)
	}
	return true, nil
}

func AlreadyDone(planRoot string) (bool, error) {
	front, _, err := ReadDocument(planRoot, "PLAN.md")
	if err != nil {
		return false, err
	}
	status := mdoc.FrontString(front, "plan_status")
	return status == draft.StatusDone, nil
}

// UpdatePhaseStatusLocked writes one phase's status and refreshes the derived
// PLAN.md checklist, reporting whether the change completed the whole plan. The
// caller must already hold the plan lock from planlock.AcquirePlan.
func UpdatePhaseStatusLocked(planRoot, planDirectory string, phaseID int, status string) (bool, error) {
	phasePath, err := FindPhaseFile(planRoot, phaseID)
	if err != nil {
		return false, fmt.Errorf("%s: %w", planDirectory, err)
	}
	phaseRaw, err := vfs.ReadFile(phasePath)
	if err != nil {
		return false, err
	}
	phaseFront, phaseBody, err := mdoc.Split(string(phaseRaw))
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", filepath.Base(phasePath), err)
	}
	if err := ValidateStatusChange(phaseFront, status); err != nil {
		return false, fmt.Errorf("%s phase %02d: %w", planDirectory, phaseID, err)
	}
	phaseFront["status"] = status
	completedAt := ""
	// completed_at records when the phase reached done; reopening it clears the stamp.
	if status == draft.StatusDone {
		completedAt = CompletionTimestamp()
		phaseFront["completed_at"] = completedAt
	} else {
		delete(phaseFront, "completed_at")
	}
	phaseContents, err := mdoc.Render(phaseFront, phaseBody)
	if err != nil {
		return false, err
	}

	phases, err := ReadPhases(planRoot)
	if err != nil {
		return false, err
	}
	for index := range phases {
		if phases[index].ID == phaseID {
			phases[index].Status = status
			break
		}
	}
	planPath := filepath.Join(planRoot, "PLAN.md")
	planRaw, err := vfs.ReadFile(planPath)
	if err != nil {
		return false, err
	}
	planFront, planBody, err := mdoc.Split(string(planRaw))
	if err != nil {
		return false, fmt.Errorf("parse PLAN.md: %w", err)
	}
	completed := len(phases) > 0
	for _, phase := range phases {
		if phase.Status != draft.StatusDone {
			completed = false
			break
		}
	}
	if completed {
		planFront["plan_status"] = draft.StatusDone
		planFront["completed_at"] = completedAt
	} else {
		planFront["plan_status"] = draft.StatusInProgress
		delete(planFront, "completed_at")
	}
	planBody, err = UpdateChecklist(planBody, phaseID, status == draft.StatusDone)
	if err != nil {
		return false, fmt.Errorf("update PLAN.md phase checklist: %w", err)
	}
	planContents, err := mdoc.Render(planFront, planBody)
	if err != nil {
		return false, err
	}
	if err := planstore.Apply(
		planstore.Update(phasePath, string(phaseRaw), phaseContents),
		planstore.Update(planPath, string(planRaw), planContents),
	); err != nil {
		return false, err
	}
	return completed, nil
}

func UpdateChecklist(body string, phaseID int, done bool) (string, error) {
	checkmark := " "
	if done {
		checkmark = "x"
	}
	return TransformChecklistEntry(body, phaseID, func(line string) (string, bool) {
		open := strings.Index(line, "[")
		if open < 0 || open+2 >= len(line) || line[open+2] != ']' {
			return "", false
		}
		return line[:open] + "[" + checkmark + "]" + line[open+3:], true
	})
}

func ValidateStatusChange(front map[string]any, status string) error {
	if status == draft.StatusConditional {
		condition := mdoc.FrontString(front, "entry_condition")
		if strings.TrimSpace(condition) == "" {
			return fmt.Errorf("conditional status requires a non-empty entry_condition")
		}
	}
	if status == draft.StatusPlanned && front["entry_condition"] != nil {
		return fmt.Errorf("planned status requires entry_condition: null")
	}
	return nil
}

func FindDirectory(planDirectories []string, planArg string) (string, string, error) {
	type match struct {
		root, directory string
	}
	matches := []match{}
	for _, plans := range planDirectories {
		entries, err := vfs.ReadDir(plans)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", "", err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if entry.Name() == planArg || draft.PlanName(entry.Name()) == planArg {
				matches = append(matches, match{root: filepath.Join(plans, entry.Name()), directory: entry.Name()})
			}
		}
	}
	if len(matches) == 0 {
		return "", "", fmt.Errorf("plan %q not found", planArg)
	}
	if len(matches) > 1 {
		return "", "", fmt.Errorf("plan %q is ambiguous; use its numbered directory name", planArg)
	}
	return matches[0].root, matches[0].directory, nil
}

func FindPhaseFile(planRoot string, phaseID int) (string, error) {
	entries, err := vfs.ReadDir(filepath.Join(planRoot, "phases"))
	if err != nil {
		return "", fmt.Errorf("read phases: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := PhaseFilePrefix.FindStringSubmatch(entry.Name())
		if len(match) != 3 {
			continue
		}
		id, err := strconv.Atoi(match[1])
		if err == nil && id == phaseID {
			return filepath.Join(planRoot, "phases", entry.Name()), nil
		}
	}
	return "", fmt.Errorf("phase %02d not found", phaseID)
}

// PhaseFileSlug returns the slug encoded in a phase document's filename.
func PhaseFileSlug(path string) string {
	match := PhaseFilePrefix.FindStringSubmatch(filepath.Base(path))
	if len(match) == 3 {
		return match[2]
	}
	return ""
}

// NextPhaseID returns the phase number a newly added phase should take.
func NextPhaseID(phases []StoredPhase) int {
	next := 0
	for _, phase := range phases {
		if phase.ID >= next {
			next = phase.ID + 1
		}
	}
	return next
}

// SlugifyTitle collapses each run of non-alphanumeric characters into a
// single dash and trims the dashes at both ends.
func SlugifyTitle(title string) string {
	var builder strings.Builder
	pendingDash := false
	for _, value := range strings.ToLower(title) {
		if (value >= 'a' && value <= 'z') || (value >= '0' && value <= '9') {
			if pendingDash {
				builder.WriteByte('-')
				pendingDash = false
			}
			builder.WriteRune(value)
			continue
		}
		pendingDash = builder.Len() > 0
	}
	return builder.String()
}

// PhaseSlug returns the stored slug of the phase with the given number.
func PhaseSlug(phases []StoredPhase, id int) string {
	for _, phase := range phases {
		if phase.ID == id {
			return phase.Slug
		}
	}
	return ""
}

// StoredPhaseDraft converts one stored phase into the draft shape used by
// dependency validation, resolving its internal depends_on references.
func StoredPhaseDraft(planName string, phase StoredPhase) (draft.Phase, error) {
	meta := draft.Meta{Phase: phase.ID, Slug: phase.Slug, Status: phase.Status}
	for _, raw := range phase.Dependencies {
		dependency, err := draft.ParseDependency(raw)
		if err != nil || dependency.Phase == nil || dependency.Plan != planName {
			return draft.Phase{}, fmt.Errorf("phase %d has invalid internal dependency %q", phase.ID, raw)
		}
		meta.DependsOn = append(meta.DependsOn, *dependency.Phase)
	}
	return draft.Phase{Title: phase.Title, Meta: meta}, nil
}

// StoredPhaseDrafts converts a plan's stored phases into draft phases so the
// shared dependency-graph validation can run against them.
func StoredPhaseDrafts(planDirectory string, phases []StoredPhase) ([]draft.Phase, error) {
	planName := draft.PlanName(planDirectory)
	all := make([]draft.Phase, 0, len(phases)+1)
	for _, phase := range phases {
		converted, err := StoredPhaseDraft(planName, phase)
		if err != nil {
			return nil, err
		}
		all = append(all, converted)
	}
	return all, nil
}

// CompletionTimestamp is the stamp written into completed_at frontmatter.
func CompletionTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}
