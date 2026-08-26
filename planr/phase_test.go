package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
)

func TestUpdatePhaseStatusCompletesAndReopensPlan(t *testing.T) {
	plansRoot := t.TempDir()
	draft := draft{
		Name:         "checkout-v2",
		Description:  "checkout flow refresh",
		NextPhase:    0,
		NextText:     "Implement the API contract.",
		Goals:        "Ship checkout.",
		Scope:        "Checkout only.",
		Context:      "Existing checkout.",
		Verification: "go test ./...",
		Ordering:     "API before UI.",
		Phases: []draftPhase{
			{Title: "API Contract", Meta: phaseMeta{Phase: 0, Slug: "api-contract", Status: "planned"}, Planned: "Add the API.", Completion: "API tests pass."},
			{Title: "Checkout UI", Meta: phaseMeta{Phase: 1, Slug: "checkout-ui", Status: "planned", DependsOn: []int{0}}, Planned: "Add the UI.", Completion: "UI tests pass."},
		},
	}
	if err := writePlan(filepath.Join(plansRoot, "00-checkout-v2"), draft, "00-checkout-v2"); err != nil {
		t.Fatalf("writePlan() unexpected error: %v", err)
	}

	if _, done, err := updatePhaseStatus([]string{plansRoot}, "checkout-v2", 0, "done"); err != nil {
		t.Fatalf("update first phase: %v", err)
	} else if done {
		t.Fatal("plan completed after only one phase")
	}
	if _, done, err := updatePhaseStatus([]string{plansRoot}, "checkout-v2", 1, "done"); err != nil {
		t.Fatalf("update second phase: %v", err)
	} else if !done {
		t.Fatal("plan did not complete after all phases were done")
	}

	planPath := filepath.Join(plansRoot, "00-checkout-v2", "PLAN.md")
	assertPlanStatus(t, planPath, "done")
	assertPlanChecklist(t, planPath, 0, true)
	assertPlanChecklist(t, planPath, 1, true)

	if _, done, err := updatePhaseStatus([]string{plansRoot}, "00-checkout-v2", 0, "planned"); err != nil {
		t.Fatalf("reopen first phase: %v", err)
	} else if done {
		t.Fatal("plan remained complete after reopening a phase")
	}
	assertPlanStatus(t, planPath, "in-progress")
	assertPlanChecklist(t, planPath, 0, false)
}

func TestUpdatePhaseChecklist(t *testing.T) {
	body := "# Phases\n\n- [ ] [Phase 00: API Contract](phases/00-api-contract.md)\n- [x] [Phase 01: Checkout UI](phases/01-checkout-ui.md)\n"
	updated, err := updatePhaseChecklist(body, 0, true)
	if err != nil {
		t.Fatalf("updatePhaseChecklist() unexpected error: %v", err)
	}
	if !strings.Contains(updated, "- [x] [Phase 00: API Contract]") {
		t.Fatalf("phase 00 was not checked:\n%s", updated)
	}
	updated, err = updatePhaseChecklist(updated, 1, false)
	if err != nil {
		t.Fatalf("updatePhaseChecklist() reset unexpected error: %v", err)
	}
	if !strings.Contains(updated, "- [ ] [Phase 01: Checkout UI]") {
		t.Fatalf("phase 01 was not unchecked:\n%s", updated)
	}
}

func TestValidatePhaseStatusChange(t *testing.T) {
	if err := validatePhaseStatusChange(map[string]any{"entry_condition": nil}, "conditional"); err == nil || !strings.Contains(err.Error(), "entry_condition") {
		t.Fatalf("conditional phase without entry condition error = %v", err)
	}
	if err := validatePhaseStatusChange(map[string]any{"entry_condition": "only when ready"}, "planned"); err == nil || !strings.Contains(err.Error(), "entry_condition: null") {
		t.Fatalf("planned phase with entry condition error = %v", err)
	}
	if err := validatePhaseStatusChange(map[string]any{"entry_condition": "only when ready"}, "in-progress"); err != nil {
		t.Fatalf("in-progress phase unexpected error: %v", err)
	}
}

func TestUncommittedSourcePathsExcludePlanFiles(t *testing.T) {
	root := t.TempDir()
	if _, err := git.PlainInit(root, false); err != nil {
		t.Fatalf("git.PlainInit() unexpected error: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "plan", "00-checkout-v2"), 0755); err != nil {
		t.Fatalf("create plan directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "plan", "00-checkout-v2", "PLAN.md"), []byte("---\n---\n"), 0644); err != nil {
		t.Fatalf("write plan file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), []byte("plans_dir: plan\n"), 0644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	paths, err := uncommittedSourcePaths(root, []string{filepath.Join(root, "plan")})
	if err != nil {
		t.Fatalf("uncommittedSourcePaths() unexpected error: %v", err)
	}
	if len(paths) != 1 || paths[0] != "main.go" {
		t.Fatalf("uncommittedSourcePaths() = %#v, want [main.go]", paths)
	}
}

func assertPlanStatus(t *testing.T, path, want string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read PLAN.md: %v", err)
	}
	front, _, err := frontmatter(string(raw))
	if err != nil {
		t.Fatalf("parse PLAN.md: %v", err)
	}
	if got, _ := front["plan_status"].(string); got != want {
		t.Fatalf("plan_status = %q, want %q", got, want)
	}
}

func assertPlanChecklist(t *testing.T, path string, phaseID int, wantDone bool) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read PLAN.md: %v", err)
	}
	_, body, err := frontmatter(string(raw))
	if err != nil {
		t.Fatalf("parse PLAN.md: %v", err)
	}
	marker := fmt.Sprintf("[Phase %02d:", phaseID)
	open := strings.Index(body, marker)
	if open < 2 {
		t.Fatalf("phase %02d checklist entry not found", phaseID)
	}
	lineStart := strings.LastIndex(body[:open], "\n") + 1
	line := body[lineStart:]
	if newline := strings.IndexByte(line, '\n'); newline >= 0 {
		line = line[:newline]
	}
	want := "- [ ]"
	if wantDone {
		want = "- [x]"
	}
	if !strings.Contains(line, want) {
		t.Fatalf("phase %02d checklist line = %q, want %q", phaseID, line, want)
	}
}
