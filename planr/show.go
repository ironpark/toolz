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

	phasePath, err := findPhaseFile(planRoot, phaseID)
	if err != nil {
		return fmt.Errorf("%s: %w", planDirectory, err)
	}
	raw, err := os.ReadFile(phasePath)
	if err != nil {
		return err
	}
	front, body, err := frontmatter(string(raw))
	if err != nil {
		return fmt.Errorf("parse %s: %w", filepath.Base(phasePath), err)
	}
	plannedWork, doneWhen, err := splitPhaseDocumentSections(stored.title, body)
	if err != nil {
		return fmt.Errorf("parse %s: %w", filepath.Base(phasePath), err)
	}
	absPath, err := filepath.Abs(phasePath)
	if err != nil {
		return err
	}
	details := phaseDetails{
		plan:         planName(planDirectory),
		directory:    planDirectory,
		id:           phaseID,
		slug:         stored.slug,
		title:        stored.title,
		status:       stored.status,
		plannedWork:  plannedWork,
		doneWhen:     doneWhen,
		dependencies: yamlStrings(front["depends_on"]),
		file:         absPath,
	}
	if status, ok := front["status"].(string); ok && status != "" {
		details.status = status
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
