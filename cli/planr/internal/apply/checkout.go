package apply

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
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
	kind := TargetPhase
	if section != "" {
		kind = TargetSection
	}
	var (
		target string
		err    error
	)
	if kind == TargetPhase {
		target, err = plan.FindPhaseFile(planRoot, phaseID)
	} else {
		target = filepath.Join(planRoot, plan.SectionFile(section))
		_, err = os.Stat(target)
	}
	if err != nil {
		return CheckoutDocument{}, fmt.Errorf("%s: %w", planDirectory, err)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		return CheckoutDocument{}, err
	}
	targetRelative, err := RelativeTargetPath(repoRoot, target)
	if err != nil {
		return CheckoutDocument{}, err
	}
	front := map[string]any{}
	selector := draft.PlanName(planDirectory)
	var body string
	if kind == TargetPhase {
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
			body = body[:start] + "\n" + plan.ChecklistPlaceholder + "\n" + body[end:]
		}
	}
	front["planr_edit"] = selector
	front["planr_target"] = targetRelative
	front["planr_base"] = mdoc.Hash(raw)
	document, err := mdoc.Render(front, body)
	if err != nil {
		return CheckoutDocument{}, err
	}
	return CheckoutDocument{Kind: kind, Selector: selector, Section: section, Target: targetRelative, Base: mdoc.Hash(raw), Document: document}, nil
}
