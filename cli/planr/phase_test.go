package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
)

func TestUpdatePhaseStatusCompletesAndReopensPlan(t *testing.T) {
	plansRoot := t.TempDir()
	planDraft := draft.Draft{
		Name:         "checkout-v2",
		Description:  "checkout flow refresh",
		NextPhase:    0,
		NextText:     "Implement the API contract.",
		Goals:        "Ship checkout.",
		Scope:        "Checkout only.",
		Context:      "Existing checkout.",
		Verification: "go test ./...",
		Ordering:     "API before UI.",
		Phases: []draft.Phase{
			{Title: "API Contract", Meta: draft.Meta{Phase: 0, Slug: "api-contract", Status: "planned"}, Planned: "Add the API.", Completion: "API tests pass."},
			{Title: "Checkout UI", Meta: draft.Meta{Phase: 1, Slug: "checkout-ui", Status: "planned", DependsOn: []int{0}}, Planned: "Add the UI.", Completion: "UI tests pass."},
		},
	}
	if err := writePlan(filepath.Join(plansRoot, "00-checkout-v2"), planDraft, "00-checkout-v2", doc.Korean); err != nil {
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

// The dependency graph application validates is only meaningful if it also governs
// execution, so advancing a phase ahead of its prerequisites is refused.
func TestEnsureDependenciesMetBlocksOutOfOrderPhases(t *testing.T) {
	plansRoot := t.TempDir()
	planRoot := filepath.Join(plansRoot, "00-checkout-v2")
	if err := writePlan(planRoot, dependentTestDraft(nil), "00-checkout-v2", doc.English); err != nil {
		t.Fatalf("writePlan() unexpected error: %v", err)
	}
	directories := []string{plansRoot}

	for _, status := range []string{"in-progress", "done"} {
		err := ensureDependenciesMet(directories, planRoot, "00-checkout-v2", 1, status)
		if err == nil {
			t.Fatalf("ensureDependenciesMet(%q) allowed phase 01 while phase 00 was planned", status)
		}
		if !strings.Contains(err.Error(), "API Contract") || !strings.Contains(err.Error(), "--force") {
			t.Fatalf("ensureDependenciesMet(%q) error = %v, want the blocking phase and --force", status, err)
		}
	}
	// Moving backwards is never blocked.
	if err := ensureDependenciesMet(directories, planRoot, "00-checkout-v2", 1, "planned"); err != nil {
		t.Fatalf("ensureDependenciesMet(planned) unexpected error: %v", err)
	}
	// A phase with no unfinished prerequisites proceeds.
	if err := ensureDependenciesMet(directories, planRoot, "00-checkout-v2", 0, "in-progress"); err != nil {
		t.Fatalf("ensureDependenciesMet() blocked an unblocked phase: %v", err)
	}

	if _, _, err := updatePhaseStatus(directories, "checkout-v2", 0, "done"); err != nil {
		t.Fatalf("complete first phase: %v", err)
	}
	if err := ensureDependenciesMet(directories, planRoot, "00-checkout-v2", 1, "done"); err != nil {
		t.Fatalf("ensureDependenciesMet() still blocked phase 01 after phase 00 was done: %v", err)
	}
}

// A plan-level dependency is what `status` reports as `wait`; it blocks the
// dependent plan's phases for the same reason a phase dependency does.
func TestEnsureDependenciesMetBlocksOnUnfinishedPlans(t *testing.T) {
	plansRoot := t.TempDir()
	planRoot := filepath.Join(plansRoot, "01-checkout-v2")
	if err := writePlan(planRoot, dependentTestDraft([]string{"api-foundation"}), "01-checkout-v2", doc.English); err != nil {
		t.Fatalf("writePlan() unexpected error: %v", err)
	}
	directories := []string{plansRoot}

	// The prerequisite plan is not registered yet, so work cannot start.
	err := ensureDependenciesMet(directories, planRoot, "01-checkout-v2", 0, "in-progress")
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("ensureDependenciesMet() error = %v, want an unregistered dependency", err)
	}

	apiRoot := filepath.Join(plansRoot, "00-api-foundation")
	if err := writePlan(apiRoot, dependentTestDraft(nil), "00-api-foundation", doc.English); err != nil {
		t.Fatalf("writePlan() unexpected error: %v", err)
	}
	err = ensureDependenciesMet(directories, planRoot, "01-checkout-v2", 0, "in-progress")
	if err == nil || !strings.Contains(err.Error(), "in-progress") {
		t.Fatalf("ensureDependenciesMet() error = %v, want an unfinished dependency", err)
	}

	for _, phase := range []int{0, 1} {
		if _, _, err := updatePhaseStatus(directories, "00-api-foundation", phase, "done"); err != nil {
			t.Fatalf("complete api-foundation phase %d: %v", phase, err)
		}
	}
	if err := ensureDependenciesMet(directories, planRoot, "01-checkout-v2", 0, "in-progress"); err != nil {
		t.Fatalf("ensureDependenciesMet() blocked on a completed plan: %v", err)
	}
}

func dependentTestDraft(dependsOn []string) draft.Draft {
	return draft.Draft{
		Name:         "checkout-v2",
		Description:  "checkout flow refresh",
		DependsOn:    dependsOn,
		NextPhase:    0,
		NextText:     "Implement the API contract.",
		Goals:        "Ship checkout.",
		Scope:        "Checkout only.",
		Context:      "Existing checkout.",
		Verification: "go test ./...",
		Ordering:     "API before UI.",
		Phases: []draft.Phase{
			{Title: "API Contract", Meta: draft.Meta{Phase: 0, Slug: "api-contract", Status: "planned"}, Planned: "Add the API.", Completion: "API tests pass."},
			{Title: "Checkout UI", Meta: draft.Meta{Phase: 1, Slug: "checkout-ui", Status: "planned", DependsOn: []int{0}}, Planned: "Add the UI.", Completion: "UI tests pass."},
		},
	}
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

	paths, err := uncommittedSourcePaths(root, []string{filepath.Join(root, "plan")}, nil)
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
	front, _, err := mdoc.Split(string(raw))
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
	_, body, err := mdoc.Split(string(raw))
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

// A draft left behind by `planr new` is planr's own output, not a source
// change, so it must not stand between the author and `planr phase done`.
func TestUncommittedSourcePathsIgnoresPlanDrafts(t *testing.T) {
	root := t.TempDir()
	if _, err := git.PlainInit(root, false); err != nil {
		t.Fatalf("git.PlainInit() unexpected error: %v", err)
	}
	files := map[string]string{
		"main.go":   "package main\n",
		"draft.md":  "---\nplan_name: demo\ndescription: \"x\"\n---\n# GOALS\n",
		"notes.md":  "# just notes\n",
		"empty.txt": "",
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	paths, err := uncommittedSourcePaths(root, []string{filepath.Join(root, "plan")}, nil)
	if err != nil {
		t.Fatalf("uncommittedSourcePaths() unexpected error: %v", err)
	}
	want := []string{"empty.txt", "main.go", "notes.md"}
	if len(paths) != len(want) {
		t.Fatalf("uncommittedSourcePaths() = %#v, want %#v", paths, want)
	}
	for index, path := range want {
		if paths[index] != path {
			t.Fatalf("uncommittedSourcePaths() = %#v, want %#v", paths, want)
		}
	}
}
