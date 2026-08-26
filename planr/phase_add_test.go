package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
)

func TestPhaseAddCommandAddsPhaseAndChecklist(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), []byte("plans_dir: plan\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	planRoot := filepath.Join(root, "plan", "00-checkout-v2")
	draft := draft{
		Name:         "checkout-v2",
		Description:  "checkout refresh",
		NextPhase:    0,
		NextText:     "Implement the API.",
		Goals:        "Ship checkout.",
		Scope:        "Checkout.",
		Context:      "Existing checkout.",
		Verification: "go test ./...",
		Ordering:     "API first.",
		Phases: []draftPhase{
			{Title: "API Contract", Meta: phaseMeta{Phase: 0, Slug: "api-contract", Status: "planned"}, Planned: "Add API.", Completion: "API tests pass."},
			{Title: "Checkout UI", Meta: phaseMeta{Phase: 1, Slug: "checkout-ui", Status: "planned", DependsOn: []int{0}}, Planned: "Add UI.", Completion: "UI tests pass."},
		},
	}
	if err := writePlan(planRoot, draft, "00-checkout-v2"); err != nil {
		t.Fatalf("writePlan() unexpected error: %v", err)
	}

	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	defer os.Chdir(workingDirectory)

	if err := newPhaseAddTestCommand().Run(context.Background(), []string{
		"phase-add", "checkout-v2", "Cache Warmup",
		"--work", "Add cache warming.",
		"--done-when", "Cache hit-rate tests pass.",
		"--depends-on", "1",
	}); err != nil {
		t.Fatalf("phase add unexpected error: %v", err)
	}

	phasePath := filepath.Join(planRoot, "phases", "02-cache-warmup.md")
	phaseRaw, err := os.ReadFile(phasePath)
	if err != nil {
		t.Fatalf("read added phase: %v", err)
	}
	phaseFront, _, err := frontmatter(string(phaseRaw))
	if err != nil {
		t.Fatalf("parse added phase: %v", err)
	}
	if got, want := phaseFront["depends_on"], []any{"00-checkout-v2#1"}; !strings.Contains(stringifyYAMLValue(got), "00-checkout-v2#1") {
		t.Fatalf("depends_on = %#v, want %v", got, want)
	}
	planRaw, err := os.ReadFile(filepath.Join(planRoot, "PLAN.md"))
	if err != nil {
		t.Fatalf("read updated PLAN.md: %v", err)
	}
	if !strings.Contains(string(planRaw), "[Phase 02: Cache Warmup](phases/02-cache-warmup.md)") {
		t.Fatalf("PLAN.md does not contain the new phase checklist:\n%s", planRaw)
	}
}

func TestPhaseAddRejectsCompletedPlan(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), []byte("plans_dir: plan\n"), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	planRoot := filepath.Join(root, "plan", "00-checkout-v2")
	draft := draft{
		Name: "checkout-v2", Description: "checkout", NextPhase: 0, NextText: "Done.",
		Goals: "Ship.", Scope: "Checkout.", Context: "Existing.", Verification: "Tests.", Ordering: "None.",
		Phases: []draftPhase{{Title: "API", Meta: phaseMeta{Phase: 0, Slug: "api", Status: "planned"}, Planned: "Build.", Completion: "Pass."}},
	}
	if err := writePlan(planRoot, draft, "00-checkout-v2"); err != nil {
		t.Fatalf("writePlan() unexpected error: %v", err)
	}
	planPath := filepath.Join(planRoot, "PLAN.md")
	raw, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("read PLAN.md: %v", err)
	}
	front, body, err := frontmatter(string(raw))
	if err != nil {
		t.Fatalf("parse PLAN.md: %v", err)
	}
	front["plan_status"] = "done"
	if err := writeFrontmatterFile(planPath, front, body); err != nil {
		t.Fatalf("mark plan done: %v", err)
	}

	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	defer os.Chdir(workingDirectory)

	err = newPhaseAddTestCommand().Run(context.Background(), []string{
		"phase-add", "checkout-v2", "New Phase",
		"--work", "Build.", "--done-when", "Pass.",
	})
	if err == nil || !strings.Contains(err.Error(), "already done") {
		t.Fatalf("phase add error = %v, want already done", err)
	}
}

func newPhaseAddTestCommand() *cli.Command {
	return &cli.Command{
		Name: "phase-add",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "slug"},
			&cli.StringSliceFlag{Name: "depends-on"},
			&cli.StringFlag{Name: "status", Value: "planned"},
			&cli.StringFlag{Name: "entry-condition"},
			&cli.BoolFlag{Name: "perf-phase"},
			&cli.StringFlag{Name: "work"},
			&cli.StringFlag{Name: "done-when"},
		},
		Action: phaseAddCommand,
	}
}

func stringifyYAMLValue(value any) string {
	return fmt.Sprintf("%v", value)
}
