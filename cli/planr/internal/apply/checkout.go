package apply

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	"github.com/ironpark/toolz/cli/planr/internal/vfs"
)

// CheckoutDocument is an editable copy of a stored plan document, wrapped in
// the planr_* envelope that Edit consumes when the document comes back.
type CheckoutDocument struct {
	Kind     string
	Selector string
	Section  string
	Target   string
	Base     string
	Document string
}

// Checkout renders the editable envelope for a phase (section == "") or for a
// plan section. It is the producer half of the format Edit parses, so both
// sides of the round trip stay defined in one place.
func Checkout(repoRoot, planRoot, planDirectory string, phaseID int, section string) (CheckoutDocument, error) {
	target, err := editTarget(planRoot, planDirectory, phaseID, section)
	if err != nil {
		return CheckoutDocument{}, err
	}
	raw, err := vfs.ReadFile(target)
	if err != nil {
		return CheckoutDocument{}, err
	}
	targetRelative, err := RelativeTargetPath(repoRoot, target)
	if err != nil {
		return CheckoutDocument{}, err
	}
	base := mdoc.Hash(raw)
	front := map[string]any{}
	selector := draft.PlanName(planDirectory)
	kind := TargetSection
	var body string
	if section == "" {
		kind = TargetPhase
		stored, phaseBody, splitErr := mdoc.Split(string(raw))
		if splitErr != nil {
			return CheckoutDocument{}, fmt.Errorf("parse %s: %w", filepath.Base(target), splitErr)
		}
		front = mdoc.CopyFront(stored)
		selector += "#" + strconv.Itoa(phaseID)
		front["planr_phase"] = phaseID
		front["planr_slug"] = plan.PhaseFileSlug(target)
		body = phaseBody
	} else {
		front["planr_section"] = section
		if _, body, err = mdoc.Split(string(raw)); err != nil {
			return CheckoutDocument{}, err
		}
		if section == "plan" {
			start, end, found := plan.ChecklistBounds(body)
			if !found {
				return CheckoutDocument{}, fmt.Errorf("PLAN.md does not contain a # Phases section")
			}
			body = body[:start] + "\n" + ChecklistPlaceholder + "\n" + body[end:]
		}
	}
	front["planr_edit"] = selector
	front["planr_target"] = targetRelative
	front["planr_base"] = base
	document, err := mdoc.Render(front, body)
	if err != nil {
		return CheckoutDocument{}, err
	}
	return CheckoutDocument{Kind: kind, Selector: selector, Section: section, Target: targetRelative, Base: base, Document: document}, nil
}

// editTarget resolves the document an edit addresses: a phase document when
// section is empty, otherwise the plan section's file. Checkout and Edit share
// it so the producer and consumer of a checkout always resolve the same file.
func editTarget(planRoot, planDirectory string, phaseID int, section string) (string, error) {
	var (
		target string
		err    error
	)
	if section == "" {
		target, err = plan.FindPhaseFile(planRoot, phaseID)
	} else {
		target = filepath.Join(planRoot, plan.SectionFile(section))
		_, err = vfs.Stat(target)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", planDirectory, err)
	}
	return target, nil
}
