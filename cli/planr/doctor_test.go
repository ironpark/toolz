package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	"github.com/urfave/cli/v3"
)

func TestDoctorCommandAcceptsAConsistentPlan(t *testing.T) {
	repoRoot := doctorRepository(t)
	planRoot := filepath.Join(repoRoot, "plan", "00-checkout-v2")
	if err := plan.Write(planRoot, testDraft(), "00-checkout-v2", doc.English); err != nil {
		t.Fatalf("plan.Write() unexpected error: %v", err)
	}
	withWorkingDirectory(t, repoRoot)
	if err := newDoctorTestCommand().Run(context.Background(), []string{"doctor"}); err != nil {
		t.Fatalf("doctor returned an error for a consistent plan: %v", err)
	}
}

func TestDoctorDetectsAndFixesChecklistMismatch(t *testing.T) {
	repoRoot := doctorRepository(t)
	planRoot := filepath.Join(repoRoot, "plan", "00-checkout-v2")
	if err := plan.Write(planRoot, testDraft(), "00-checkout-v2", doc.English); err != nil {
		t.Fatalf("plan.Write() unexpected error: %v", err)
	}
	planPath := filepath.Join(planRoot, "PLAN.md")
	raw, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	broken := strings.Replace(string(raw), "[Phase 00: API Contract](phases/00-api-contract.md)", "[Phase 00: Wrong title](phases/00-wrong.md)", 1)
	if broken == string(raw) {
		t.Fatal("test did not change the checklist")
	}
	if err := os.WriteFile(planPath, []byte(broken), 0644); err != nil {
		t.Fatal(err)
	}

	plan, issues := inspectDoctorPlan(planRoot, "00-checkout-v2")
	if len(issues) != 0 || len(plan.checklistIssues) == 0 {
		t.Fatalf("inspectDoctorPlan() issues=%v checklist=%v, want checklist mismatch", issues, plan.checklistIssues)
	}
	withWorkingDirectory(t, repoRoot)
	if err := newDoctorTestCommand().Run(context.Background(), []string{"doctor"}); err == nil {
		t.Fatal("doctor accepted a mismatched checklist")
	}
	if err := newDoctorTestCommand().Run(context.Background(), []string{"doctor", "--fix"}); err != nil {
		t.Fatalf("doctor --fix returned an error: %v", err)
	}
	fixed, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fixed), "[Phase 00: API Contract](phases/00-api-contract.md)") {
		t.Fatalf("doctor --fix did not restore checklist:\n%s", fixed)
	}
	fixedPlan, fixedIssues := inspectDoctorPlan(planRoot, "00-checkout-v2")
	if len(fixedIssues) != 0 || len(fixedPlan.checklistIssues) != 0 {
		t.Fatalf("fixed plan still has issues=%v checklist=%v", fixedIssues, fixedPlan.checklistIssues)
	}
}

func TestDoctorDetectsBrokenDependencyAndFrontmatter(t *testing.T) {
	repoRoot := doctorRepository(t)
	planDraft := testDraft()
	planDraft.DependsOn = []string{"missing-plan"}
	planRoot := filepath.Join(repoRoot, "plan", "00-checkout-v2")
	if err := plan.Write(planRoot, planDraft, "00-checkout-v2", doc.English); err != nil {
		t.Fatalf("plan.Write() unexpected error: %v", err)
	}
	phasePath := filepath.Join(planRoot, "phases", "00-api-contract.md")
	if err := os.WriteFile(phasePath, []byte("---\nstatus: [broken\n"), 0644); err != nil {
		t.Fatal(err)
	}

	withWorkingDirectory(t, repoRoot)
	err := newDoctorTestCommand().Run(context.Background(), []string{"doctor"})
	if err == nil {
		t.Fatal("doctor accepted broken dependency/frontmatter")
	}
	if !strings.Contains(err.Error(), "problem") {
		t.Fatalf("doctor error = %v, want problem summary", err)
	}
}

func doctorRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if _, err := git.PlainInit(root, false); err != nil {
		t.Fatalf("git init: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "plan"), 0755); err != nil {
		t.Fatal(err)
	}
	return root
}

func withWorkingDirectory(t *testing.T, directory string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func newDoctorTestCommand() *cli.Command {
	return &cli.Command{
		Name: "doctor",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "fix"},
		},
		Action: doctorCommand,
	}
}
