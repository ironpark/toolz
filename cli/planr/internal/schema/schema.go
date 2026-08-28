package schema

import (
	"github.com/ironpark/toolz/cli/planr/internal/draft"
)

type Output struct {
	Name                  string     `json:"name"`
	Version               int        `json:"version"`
	RequiredPlanSections  []string   `json:"required_plan_sections"`
	PlanFrontmatter       []string   `json:"plan_frontmatter"`
	PhaseStatuses         []string   `json:"phase_statuses"`
	NewPhaseStatuses      []string   `json:"new_phase_statuses"`
	DependencyNotation    string     `json:"dependency_notation"`
	PhaseBlock            PhaseBlock `json:"phase_block"`
	RegisteredPhase       PhaseBlock `json:"registered_phase_document"`
	PhaseDraftFrontmatter []string   `json:"phase_draft_frontmatter"`
	EditFrontmatter       []string   `json:"edit_frontmatter"`
	ApplyKinds            []string   `json:"apply_kinds"`
	DerivedRegions        []string   `json:"derived_regions"`
	ValidationErrors      string     `json:"validation_errors"`
}

type PhaseBlock struct {
	Heading      string   `json:"heading"`
	Metadata     []string `json:"metadata"`
	BodySections []string `json:"body_sections"`
	NumberPolicy string   `json:"number_policy"`
}

func Value() Output {
	return Output{
		Name:                 "planr-plan-documents",
		Version:              1,
		RequiredPlanSections: append([]string{}, draft.RequiredSections...),
		PlanFrontmatter:      []string{"plan_name", "description", "depends_on"},
		PhaseStatuses:        []string{"planned", "conditional", "in-progress", "done"},
		NewPhaseStatuses:     []string{"planned", "conditional"},
		DependencyNotation:   "plan-name or plan-name#phase-number; phase drafts may use an existing phase number or slug",
		PhaseBlock: PhaseBlock{
			Heading:      "## PHASE — <title>",
			Metadata:     []string{"phase", "slug", "perf_phase", "depends_on", "status", "entry_condition"},
			BodySections: []string{"### Planned Work (or the localized equivalent)", "### Done When (or the localized equivalent)"},
			NumberPolicy: "non-negative and unique within the plan; apply assigns new phase numbers as max existing + 1",
		},
		RegisteredPhase: PhaseBlock{
			Heading:      "# <title>",
			Metadata:     []string{"status", "entry_condition", "perf_phase", "depends_on", "blocks"},
			BodySections: []string{"## Planned Work (or the localized equivalent)", "## Done When (or the localized equivalent)"},
			NumberPolicy: "the number is in the phases/NN-slug.md filename and in plan links; it is never reassigned",
		},
		PhaseDraftFrontmatter: []string{"planr_new: phase", "planr_plan", "phase_title", "slug", "perf_phase", "depends_on", "status", "entry_condition"},
		EditFrontmatter:       []string{"planr_edit", "planr_target", "planr_base", "planr_section (section checkouts)", "planr_phase and planr_slug (phase checkouts)"},
		ApplyKinds:            []string{"full plan draft", "phase draft", "planr_edit checkout"},
		DerivedRegions:        []string{"PLAN.md phase checklist", "PLAN.md plan_status", "phase status transitions"},
		// Both evaluated agents reached for schema first and never discovered
		// that apply can report failures as data, so the contract advertises it.
		ValidationErrors: "run `planr apply --json` to get failures as {rule, section, phase, line, detail} instead of prose",
	}
}
