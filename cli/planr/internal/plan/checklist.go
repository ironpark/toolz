package plan

import (
	"fmt"
	"strings"

	"github.com/ironpark/toolz/cli/planr/internal/draft"
)

// ChecklistPlaceholder marks the derived phase checklist region in a PLAN.md
// checkout. `edit --section plan` swaps the real checklist for it, and `apply`
// refuses the document unless it comes back untouched.
const ChecklistPlaceholder = "<!-- planr: phase checklist is derived; do not edit -->"

// AppendChecklistEntry inserts a checklist entry for a new phase at the end of
// the `# Phases` section of a PLAN.md body.
func AppendChecklistEntry(body string, phaseID int, title, slug string) (string, error) {
	marker := fmt.Sprintf("[Phase %02d:", phaseID)
	if strings.Contains(body, marker) {
		return "", fmt.Errorf("checklist already contains phase %02d", phaseID)
	}
	lines := strings.SplitAfter(body, "\n")
	offset := 0
	phasesHeadingEnd := -1
	insertion := len(body)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "# Phases" {
			phasesHeadingEnd = offset + len(line)
		} else if phasesHeadingEnd >= 0 && strings.HasPrefix(trimmed, "# ") {
			insertion = offset
			break
		}
		offset += len(line)
	}
	if phasesHeadingEnd < 0 {
		return "", fmt.Errorf("PLAN.md does not contain a # Phases section")
	}
	entry := ChecklistEntry(phaseID, title, slug, false)
	before := strings.TrimRight(body[:insertion], "\n")
	after := strings.TrimLeft(body[insertion:], "\n")
	if after == "" {
		return before + "\n\n" + entry + "\n", nil
	}
	return before + "\n" + entry + "\n\n" + after, nil
}

// ReplaceChecklistEntry rewrites the checklist entry of an existing phase,
// which is how a phase title change propagates back into PLAN.md.
func ReplaceChecklistEntry(body string, phaseID int, title, slug string, done bool) (string, error) {
	return TransformChecklistEntry(body, phaseID, func(line string) (string, bool) {
		replacement := ChecklistEntry(phaseID, title, slug, done) + "\n"
		if !strings.HasSuffix(line, "\n") {
			replacement = strings.TrimSuffix(replacement, "\n")
		}
		return replacement, true
	})
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
