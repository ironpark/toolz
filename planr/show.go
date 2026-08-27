package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/urfave/cli/v3"
)

type phaseDetails struct {
	plan, directory string
	id              int
	slug, title     string
	status          string
	plannedWork     string
	doneWhen        string
	dependencies    []string
	file            string
}

func showCommand(_ context.Context, cmd *cli.Command) error {
	if cmd.NArg() < 1 || cmd.NArg() > 2 {
		return fmt.Errorf("show requires <plan-name> and optionally <phase-number>")
	}
	section := strings.TrimSpace(cmd.String("section"))
	if section != "" && section != "goals" && section != "context" && section != "plan" {
		return fmt.Errorf("invalid show section %q; use goals, context, or plan", section)
	}
	if (section != "" || cmd.Bool("all")) && cmd.NArg() != 1 {
		return fmt.Errorf("show --section and --all accept only a plan name")
	}
	if section != "" && cmd.Bool("all") {
		return fmt.Errorf("show accepts either --section or --all, not both")
	}
	planArg := cmd.Args().First()

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	planDirectories, err := planPaths(cwd)
	if err != nil {
		return err
	}
	planRoot, planDirectory, err := findPlanDirectory(planDirectories, planArg)
	if err != nil {
		return err
	}
	if section != "" {
		return showPlanSection(planRoot, planDirectory, section, cmd.Bool("json"))
	}
	if cmd.Bool("all") {
		return showAllPlan(planRoot, planDirectory, cmd.Bool("json"))
	}
	phases, err := readPlanPhases(planRoot)
	if err != nil {
		return err
	}

	phaseID := -1
	if cmd.NArg() == 2 {
		phaseID, err = strconv.Atoi(cmd.Args().Get(1))
		if err != nil || phaseID < 0 {
			return fmt.Errorf("phase number %q must be a non-negative integer", cmd.Args().Get(1))
		}
	} else {
		for _, phase := range phases {
			if phase.status != "done" {
				phaseID = phase.id
				break
			}
		}
		if phaseID < 0 {
			return fmt.Errorf("plan %q has no unfinished phases", planDirectory)
		}
	}

	var stored storedPhase
	found := false
	for _, phase := range phases {
		if phase.id == phaseID {
			stored = phase
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%s: phase %02d not found", planDirectory, phaseID)
	}

	details, err := readPhaseDetails(planRoot, planDirectory, stored)
	if err != nil {
		return err
	}
	if cmd.Bool("json") {
		return writeJSON(makeShowJSON(details))
	}

	fmt.Printf("Phase %02d: %s\n", details.id, details.title)
	fmt.Printf("status: %s\n", details.status)
	printShowBody("planned_work", details.plannedWork)
	printShowBody("done_when", details.doneWhen)
	printShowList("depends_on", details.dependencies)
	fmt.Printf("file: %s\n", details.file)
	return nil
}

func showPlanSection(planRoot, planDirectory, section string, jsonOutput bool) error {
	path := filepath.Join(planRoot, sectionFile(section))
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(showSectionJSONOutput{Plan: planName(planDirectory), Directory: planDirectory, Section: section, Content: string(raw), File: absPath})
	}
	fmt.Print(string(raw))
	return nil
}

func showAllPlan(planRoot, planDirectory string, jsonOutput bool) error {
	if !jsonOutput {
		return fmt.Errorf("show --all requires --json")
	}
	front, _, err := readPlanDocument(planRoot, "PLAN.md")
	if err != nil {
		return err
	}
	documents := map[string]string{}
	for _, relative := range []string{"GOALS.md", "CONTEXT.md", "PLAN.md"} {
		path := filepath.Join(planRoot, relative)
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		documents[relative] = string(raw)
	}
	phases, err := readPlanPhases(planRoot)
	if err != nil {
		return err
	}
	phaseJSON := make([]showJSONOutput, 0, len(phases))
	for _, phase := range phases {
		details, detailsErr := readPhaseDetails(planRoot, planDirectory, phase)
		if detailsErr != nil {
			return detailsErr
		}
		phaseJSON = append(phaseJSON, makeShowJSON(details))
		path, pathErr := findPhaseFile(planRoot, phase.id)
		if pathErr != nil {
			return pathErr
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		relative, relErr := filepath.Rel(planRoot, path)
		if relErr != nil {
			return relErr
		}
		documents[filepath.ToSlash(relative)] = string(raw)
	}
	return writeJSON(showAllJSONOutput{
		Plan:         planName(planDirectory),
		Directory:    planDirectory,
		Status:       frontString(front, "plan_status"),
		Description:  frontString(front, "description"),
		DependsOn:    yamlStrings(front["depends_on"]),
		Goals:        documents["GOALS.md"],
		Context:      documents["CONTEXT.md"],
		PlanDocument: documents["PLAN.md"],
		Phases:       phaseJSON,
		Documents:    documents,
	})
}

func readPlanDocument(planRoot, name string) (map[string]any, string, error) {
	raw, err := os.ReadFile(filepath.Join(planRoot, name))
	if err != nil {
		return nil, "", err
	}
	front, body, err := frontmatter(string(raw))
	if err != nil {
		return nil, "", fmt.Errorf("parse %s: %w", name, err)
	}
	return front, body, nil
}

func readPhaseDetails(planRoot, planDirectory string, stored storedPhase) (phaseDetails, error) {
	phasePath, err := findPhaseFile(planRoot, stored.id)
	if err != nil {
		return phaseDetails{}, fmt.Errorf("%s: %w", planDirectory, err)
	}
	raw, err := os.ReadFile(phasePath)
	if err != nil {
		return phaseDetails{}, err
	}
	front, body, err := frontmatter(string(raw))
	if err != nil {
		return phaseDetails{}, fmt.Errorf("parse %s: %w", filepath.Base(phasePath), err)
	}
	plannedWork, doneWhen, err := splitPhaseDocumentSections(stored.title, body)
	if err != nil {
		return phaseDetails{}, fmt.Errorf("parse %s: %w", filepath.Base(phasePath), err)
	}
	absPath, err := filepath.Abs(phasePath)
	if err != nil {
		return phaseDetails{}, err
	}
	details := phaseDetails{plan: planName(planDirectory), directory: planDirectory, id: stored.id, slug: stored.slug, title: stored.title, status: stored.status, plannedWork: plannedWork, doneWhen: doneWhen, dependencies: yamlStrings(front["depends_on"]), file: absPath}
	if status, ok := front["status"].(string); ok && status != "" {
		details.status = status
	}
	return details, nil
}

func frontString(front map[string]any, key string) string {
	value, _ := front[key].(string)
	return value
}

func printShowBody(label, body string) {
	fmt.Printf("%s:\n", label)
	for _, line := range strings.Split(body, "\n") {
		fmt.Printf("  %s\n", line)
	}
}

func printShowList(label string, values []string) {
	if len(values) == 0 {
		fmt.Printf("%s: []\n", label)
		return
	}
	fmt.Printf("%s:\n", label)
	for _, value := range values {
		fmt.Printf("  - %s\n", value)
	}
}
