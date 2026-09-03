package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/ironpark/toolz/cli/planr/internal/jsonout"
	"github.com/ironpark/toolz/cli/planr/internal/schema"
	ucli "github.com/urfave/cli/v3"
)

func newSchemaCommand() *ucli.Command {
	return &ucli.Command{
		Name:   "schema",
		Usage:  "describe the plan document contract",
		Flags:  []ucli.Flag{jsonFlag()},
		Action: runSchema,
	}
}

func runSchema(_ context.Context, cmd *ucli.Command) error {
	if cmd.NArg() != 0 {
		return fmt.Errorf("schema does not accept positional arguments")
	}
	value := schema.Value()
	if cmd.Bool("json") {
		return jsonout.Write(value)
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
