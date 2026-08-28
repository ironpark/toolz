package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	"github.com/urfave/cli/v3"
)

const scratchDirectory = ".planr"

func editCommand(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() != 1 {
		return fmt.Errorf("edit requires <plan-name>#<phase-number> or <plan-name> with --section")
	}
	section := strings.TrimSpace(cmd.String("section"))
	selector := cmd.Args().First()
	if section != "" {
		if !validSection(section) {
			return fmt.Errorf("invalid edit section %q; use goals, context, or plan", section)
		}
		if strings.Contains(selector, "#") {
			return fmt.Errorf("edit --section accepts a plan name, not a phase selector")
		}
	} else {
		planArg, targetKind, _, _, err := parseEditSelector(selector)
		if err != nil {
			return err
		}
		if targetKind != "phase" {
			return fmt.Errorf("edit requires a phase selector or --section")
		}
		selector = planArg + "#" + strings.TrimSpace(strings.SplitN(cmd.Args().First(), "#", 2)[1])
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	settings, repoRoot, err := config.Load(cwd)
	if err != nil {
		return err
	}
	settings = commandSettings(settings, cmd)
	planDirectories := settings.PlanDirs(repoRoot)
	planArg := selector
	targetKind := "section"
	phaseID := -1
	if section == "" {
		var parsedSection string
		planArg, targetKind, phaseID, parsedSection, err = parseEditSelector(selector)
		if err != nil {
			return err
		}
		section = parsedSection
	}
	planRoot, planDirectory, err := plan.FindDirectory(planDirectories, planArg)
	if err != nil {
		return err
	}
	var target string
	if targetKind == "phase" {
		target, err = plan.FindPhaseFile(planRoot, phaseID)
	} else {
		target = filepath.Join(planRoot, sectionFile(section))
		_, err = os.Stat(target)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", planDirectory, err)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	targetRelative, err := relativeTargetPath(repoRoot, target)
	if err != nil {
		return err
	}
	editSelector := draft.Name(planDirectory) + "#"
	checkoutFront := map[string]any{}
	var body string
	if targetKind == "phase" {
		front, phaseBody, frontErr := mdoc.Split(string(raw))
		if frontErr != nil {
			return fmt.Errorf("parse %s: %w", filepath.Base(target), frontErr)
		}
		checkoutFront = mdoc.CopyFront(front)
		editSelector += strconv.Itoa(phaseID)
		checkoutFront["planr_phase"] = phaseID
		checkoutFront["planr_slug"] = phaseSlugFromPath(target)
		body = phaseBody
	} else {
		editSelector = draft.Name(planDirectory)
		checkoutFront["planr_section"] = section
		_, body, err = mdoc.Split(string(raw))
		if err != nil {
			return err
		}
		if section == "plan" {
			start, end, found := doctorChecklistBounds(body)
			if !found {
				return fmt.Errorf("PLAN.md does not contain a # Phases section")
			}
			body = body[:start] + "\n" + planChecklistPlaceholder + "\n" + body[end:]
		}
	}
	checkoutFront["planr_edit"] = editSelector
	checkoutFront["planr_target"] = targetRelative
	checkoutFront["planr_base"] = mdoc.Hash(raw)
	document, err := mdoc.Render(checkoutFront, body)
	if err != nil {
		return err
	}
	if cmd.Bool("json") {
		return writeJSON(editJSONOutput{Kind: targetKind, Selector: editSelector, Section: section, Target: targetRelative, Base: mdoc.Hash(raw), Document: document})
	}
	output := cmd.String("output")
	if output == "" {
		output = filepath.Join(repoRoot, scratchDirectory, scratchFileName(planDirectory, targetKind, phaseID, section))
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
	if err := mdoc.WriteAtomically(absOutput, document); err != nil {
		return err
	}
	fmt.Printf("Checked out %s\n", absOutput)
	return nil
}

func scratchFileName(planDirectory, targetKind string, phaseID int, section string) string {
	name := draft.Name(planDirectory)
	if targetKind == "phase" {
		return fmt.Sprintf("%s-phase-%02d.md", name, phaseID)
	}
	return fmt.Sprintf("%s-section-%s.md", name, section)
}

func phaseSlugFromPath(path string) string {
	match := plan.PhaseFilePrefix.FindStringSubmatch(filepath.Base(path))
	if len(match) == 3 {
		return match[2]
	}
	return ""
}
