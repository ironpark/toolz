package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ironpark/toolz/cli/planr/internal/apply"
	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/jsonout"
	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	ucli "github.com/urfave/cli/v3"
)

const scratchDirectory = ".planr"

func editCommand(_ context.Context, cmd *ucli.Command) error {
	if cmd.NArg() != 1 {
		return fmt.Errorf("edit requires <plan-name>#<phase-number> or <plan-name> with --section")
	}
	section := strings.TrimSpace(cmd.String("section"))
	selector := cmd.Args().First()
	planArg := selector
	phaseID := -1
	if section != "" {
		if !plan.ValidSection(section) {
			return fmt.Errorf("invalid edit section %q; use goals, context, or plan", section)
		}
		if strings.Contains(selector, "#") {
			return fmt.Errorf("edit --section accepts a plan name, not a phase selector")
		}
	} else {
		parsedPlan, parsedPhase, err := apply.ParseEditSelector(selector)
		if err != nil {
			return err
		}
		planArg, phaseID = parsedPlan, parsedPhase
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	settings, repoRoot, err := config.Load(cwd)
	if err != nil {
		return err
	}
	settings = settings.WithSkipHooks(cmd.Bool("no-hooks"))
	planRoot, planDirectory, err := plan.FindDirectory(settings.PlanDirs(repoRoot), planArg)
	if err != nil {
		return err
	}
	checkout, err := apply.Checkout(repoRoot, planRoot, planDirectory, phaseID, section)
	if err != nil {
		return err
	}
	if cmd.Bool("json") {
		return jsonout.Write(jsonout.Edit(checkout))
	}
	output := cmd.String("output")
	if output == "" {
		output = filepath.Join(repoRoot, scratchDirectory, scratchFileName(planDirectory, phaseID, section))
	}
	absOutput, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if _, err := os.Stat(absOutput); err == nil {
		return fmt.Errorf("editable file already exists: %s", absOutput)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absOutput), 0755); err != nil {
		return err
	}
	if err := mdoc.WriteAtomically(absOutput, checkout.Document); err != nil {
		return err
	}
	fmt.Printf("Checked out %s\n", absOutput)
	return nil
}

func scratchFileName(planDirectory string, phaseID int, section string) string {
	name := draft.PlanName(planDirectory)
	if section == "" {
		return fmt.Sprintf("%s-phase-%02d.md", name, phaseID)
	}
	return fmt.Sprintf("%s-section-%s.md", name, section)
}
