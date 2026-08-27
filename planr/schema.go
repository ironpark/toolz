package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/urfave/cli/v3"
)

type schemaOutput struct {
	Name                  string           `json:"name"`
	Version               int              `json:"version"`
	RequiredPlanSections  []string         `json:"required_plan_sections"`
	PlanFrontmatter       []string         `json:"plan_frontmatter"`
	PhaseStatuses         []string         `json:"phase_statuses"`
	NewPhaseStatuses      []string         `json:"new_phase_statuses"`
	DependencyNotation    string           `json:"dependency_notation"`
	PhaseBlock            schemaPhaseBlock `json:"phase_block"`
	RegisteredPhase       schemaPhaseBlock `json:"registered_phase_document"`
	PhaseDraftFrontmatter []string         `json:"phase_draft_frontmatter"`
	EditFrontmatter       []string         `json:"edit_frontmatter"`
	ApplyKinds            []string         `json:"apply_kinds"`
	DerivedRegions        []string         `json:"derived_regions"`
	ValidationErrors      string           `json:"validation_errors"`
}

type schemaPhaseBlock struct {
	Heading      string   `json:"heading"`
	Metadata     []string `json:"metadata"`
	BodySections []string `json:"body_sections"`
	NumberPolicy string   `json:"number_policy"`
}

func schemaValue() schemaOutput {
	return schemaOutput{
		Name:                 "planr-plan-documents",
		Version:              1,
		RequiredPlanSections: append([]string{}, requiredSections...),
		PlanFrontmatter:      []string{"plan_name", "description", "depends_on"},
		PhaseStatuses:        []string{"planned", "conditional", "in-progress", "done"},
		NewPhaseStatuses:     []string{"planned", "conditional"},
		DependencyNotation:   "plan-name or plan-name#phase-number; phase drafts may use an existing phase number or slug",
		PhaseBlock: schemaPhaseBlock{
			Heading:      "## PHASE — <title>",
			Metadata:     []string{"phase", "slug", "perf_phase", "depends_on", "status", "entry_condition"},
			BodySections: []string{"### Planned Work (or the localized equivalent)", "### Done When (or the localized equivalent)"},
			NumberPolicy: "non-negative and unique within the plan; apply assigns new phase numbers as max existing + 1",
		},
		RegisteredPhase: schemaPhaseBlock{
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

func schemaCommand(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() != 0 {
		return fmt.Errorf("schema does not accept positional arguments")
	}
	value := schemaValue()
	if cmd.Bool("json") {
		return writeJSON(value)
	}
	fmt.Printf("document: %s\n", value.Name)
	fmt.Printf("version: %d\n", value.Version)
	fmt.Printf("required_plan_sections: %s\n", strings.Join(value.RequiredPlanSections, ", "))
	fmt.Printf("phase_statuses: %s\n", strings.Join(value.PhaseStatuses, ", "))
	fmt.Printf("dependency_notation: %s\n", value.DependencyNotation)
	fmt.Printf("phase_block: %s\n", value.PhaseBlock.Heading)
	fmt.Printf("phase_metadata: %s\n", strings.Join(value.PhaseBlock.Metadata, ", "))
	fmt.Printf("phase_body_sections: %s\n", strings.Join(value.PhaseBlock.BodySections, "; "))
	fmt.Printf("new_phase_frontmatter: %s\n", strings.Join(value.PhaseDraftFrontmatter, ", "))
	fmt.Printf("edit_frontmatter: %s\n", strings.Join(value.EditFrontmatter, ", "))
	fmt.Printf("derived_regions: %s\n", strings.Join(value.DerivedRegions, "; "))
	return nil
}
