package apply

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/hooks"
	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	"github.com/ironpark/toolz/cli/planr/internal/validation"
)

func applyTestSettings() config.Config {
	return config.Config{
		PlansDirs: []string{"plans"},
		Language:  doc.English,
		Hooks:     hooks.Config{Timeout: hooks.DefaultTimeout},
		SkipHooks: true,
	}
}

func applyTestDraft(name string) draft.Draft {
	return draft.Draft{
		Name:         name,
		Description:  "a test plan",
		NextPhase:    0,
		NextText:     "Implement the first phase.",
		Goals:        "Ship the test plan.",
		Scope:        "The test scope.",
		Context:      "The test context.",
		Verification: "go test ./...",
		Ordering:     "The first phase comes first.",
		Phases: []draft.Phase{
			{Title: "Foundation", Meta: draft.Meta{Phase: 0, Slug: "foundation", Status: "planned"}, Planned: "Build the foundation.", Completion: "Foundation tests pass."},
			{Title: "Follow-up", Meta: draft.Meta{Phase: 1, Slug: "follow-up", Status: "planned", DependsOn: []int{0}}, Planned: "Build the follow-up.", Completion: "Follow-up tests pass."},
		},
	}
}

func filledPhaseDraft(t *testing.T, language string) PhaseDraft {
	t.Helper()
	raw, err := doc.RenderNewPhaseDraft(language, "checkout-v2", "Cache Warmup", "cache-warmup")
	if err != nil {
		t.Fatalf("doc.RenderNewPhaseDraft() unexpected error: %v", err)
	}
	raw = strings.Replace(raw, "perf_phase: false", "perf_phase: true", 1)
	raw = strings.Replace(raw, "depends_on: []", "depends_on: [1]", 1)
	raw = strings.Replace(raw, "status: planned", "status: conditional", 1)
	raw = strings.Replace(raw, "entry_condition: null", "entry_condition: only after the cache API is ready", 1)
	raw = strings.ReplaceAll(raw, draft.Placeholder, "filled")
	parsed, err := parsePhaseDraft([]byte(raw))
	if err != nil {
		t.Fatalf("parsePhaseDraft() unexpected error: %v", err)
	}
	return parsed
}

// editCheckout renders the editable document that `planr edit` hands to Edit,
// through the same Checkout the command uses, so these tests exercise the
// envelope the shipping code actually produces.
func editCheckout(t *testing.T, repoRoot, planRoot, planDirectory string, phaseID int, section string) string {
	t.Helper()
	checkout, err := Checkout(repoRoot, planRoot, planDirectory, phaseID, section)
	if err != nil {
		t.Fatalf("Checkout() unexpected error: %v", err)
	}
	return checkout.Document
}

func TestApplyPhaseDraftAddsPhaseAndPreservesPhaseFlags(t *testing.T) {
	root := t.TempDir()
	settings := applyTestSettings()
	planRoot := filepath.Join(root, "plans", "00-checkout-v2")
	if err := plan.Write(planRoot, applyTestDraft("checkout-v2"), "00-checkout-v2", doc.English); err != nil {
		t.Fatalf("plan.Write() unexpected error: %v", err)
	}

	planDraft := filledPhaseDraft(t, doc.English)
	if _, err := Phase(planDraft, settings, root, false, false); err != nil {
		t.Fatalf("Phase() unexpected error: %v", err)
	}
	phasePath := filepath.Join(planRoot, "phases", "02-cache-warmup.md")
	raw, err := os.ReadFile(phasePath)
	if err != nil {
		t.Fatalf("read applied phase: %v", err)
	}
	front, _, err := mdoc.Split(string(raw))
	if err != nil {
		t.Fatalf("parse applied phase: %v", err)
	}
	if got, want := front["status"], "conditional"; got != want {
		t.Fatalf("status = %v, want %v", got, want)
	}
	if got, want := front["perf_phase"], true; got != want {
		t.Fatalf("perf_phase = %v, want %v", got, want)
	}
	if !strings.Contains(string(raw), "00-checkout-v2#1") || !strings.Contains(string(raw), "only after the cache API is ready") {
		t.Fatalf("applied phase lost its metadata:\n%s", raw)
	}
	planRaw, err := os.ReadFile(filepath.Join(planRoot, "PLAN.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(planRaw), "[Phase 02: Cache Warmup](phases/02-cache-warmup.md)") {
		t.Fatalf("PLAN.md does not contain the new checklist entry:\n%s", planRaw)
	}
}

func TestApplyPhaseDraftRefusesCompletedPlan(t *testing.T) {
	root := t.TempDir()
	settings := applyTestSettings()
	planRoot := filepath.Join(root, "plans", "00-checkout-v2")
	if err := plan.Write(planRoot, applyTestDraft("checkout-v2"), "00-checkout-v2", doc.English); err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(planRoot, "PLAN.md")
	raw, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	front, body, err := mdoc.Split(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	front["plan_status"] = "done"
	if err := mdoc.WriteFile(planPath, front, body); err != nil {
		t.Fatal(err)
	}

	err = func() error {
		_, err := Phase(filledPhaseDraft(t, doc.English), settings, root, false, false)
		return err
	}()
	if err == nil || !strings.Contains(err.Error(), "already done") {
		t.Fatalf("Phase() error = %v, want already done", err)
	}
	records := validation.Records(err)
	if len(records) != 1 || records[0].Rule != "plan_done" {
		t.Fatalf("validation records = %#v, want plan_done", records)
	}
}

func TestApplyDryRunDoesNotCreatePlanFiles(t *testing.T) {
	root := t.TempDir()
	settings := applyTestSettings()
	planDraft := applyTestDraft("dry-run-plan")
	documents, err := plan.RenderDocuments(planDraft, "00-dry-run-plan", settings.Language, "2026-08-27T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	var raw strings.Builder
	raw.WriteString("---\nplan_name: dry-run-plan\ndescription: a test plan\n---\n")
	raw.WriteString("# GOALS\n\nShip the test plan.\n# SCOPE\n\nThe test scope.\n# CONTEXT\n\nThe test context.\n# PHASES\n\n## PHASE — Foundation\n\n```yaml\nphase: 0\nslug: foundation\nstatus: planned\n```\n\n### Planned Work\n\nBuild the foundation.\n\n### Done When\n\nFoundation tests pass.\n# VERIFICATION\n\ngo test ./...\n# ORDERING\n\nThe first phase comes first.\n# NEXT\n\n```yaml\nnext_phase: 0\n```\n\nImplement the first phase.\n")
	parsed, err := draft.Parse([]byte(raw.String()), "dry-run-plan.md")
	if err != nil {
		t.Fatalf("parse draft: %v", err)
	}
	if _, err := Plan(parsed, settings, root, true, false); err != nil {
		t.Fatalf("Plan(dry-run): %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "plans")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created plans directory; stat error = %v", err)
	}
	if len(documents) == 0 {
		t.Fatal("rendered dry-run documents are empty")
	}
}

func TestEditPhaseBaseHashAndStatusSafety(t *testing.T) {
	root := t.TempDir()
	settings := applyTestSettings()
	planRoot := filepath.Join(root, "plans", "00-checkout-v2")
	if err := plan.Write(planRoot, applyTestDraft("checkout-v2"), "00-checkout-v2", doc.English); err != nil {
		t.Fatal(err)
	}
	phasePath := filepath.Join(planRoot, "phases", "00-foundation.md")
	checkout := editCheckout(t, root, planRoot, "00-checkout-v2", 0, "")
	if !strings.Contains(checkout, "planr_base:") || !strings.Contains(checkout, "planr_target:") {
		t.Fatalf("checkout is missing safety metadata:\n%s", checkout)
	}
	updated := strings.Replace(checkout, "Build the foundation.", "Build the safer foundation.", 1)
	if _, err := Edit([]byte(updated), settings, root, false, false); err != nil {
		t.Fatalf("Edit() unexpected error: %v", err)
	}
	changed, err := os.ReadFile(phasePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(changed), "Build the safer foundation.") {
		t.Fatalf("phase edit was not applied:\n%s", changed)
	}

	stale := strings.Replace(updated, "Build the safer foundation.", "A stale edit.", 1)
	_, err = Edit([]byte(stale), settings, root, false, false)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("stale apply error = %v, want hash mismatch", err)
	}
	if records := validation.Records(err); len(records) != 1 || records[0].Rule != "base_mismatch" {
		t.Fatalf("stale validation records = %#v, want base_mismatch", records)
	}

	statusCheckout := editCheckout(t, root, planRoot, "00-checkout-v2", 0, "")
	statusEdited := strings.Replace(statusCheckout, "status: planned", "status: in-progress", 1)
	_, err = Edit([]byte(statusEdited), settings, root, false, false)
	if err == nil || !strings.Contains(err.Error(), "use `planr phase start`") {
		t.Fatalf("status edit error = %v, want phase start guidance", err)
	}
	if records := validation.Records(err); len(records) != 1 || records[0].Rule != "status_transition" {
		t.Fatalf("status validation records = %#v, want status_transition", records)
	}
}

func TestEditPlanSectionProtectsDerivedChecklist(t *testing.T) {
	root := t.TempDir()
	settings := applyTestSettings()
	planRoot := filepath.Join(root, "plans", "00-checkout-v2")
	if err := plan.Write(planRoot, applyTestDraft("checkout-v2"), "00-checkout-v2", doc.English); err != nil {
		t.Fatal(err)
	}
	checkout := editCheckout(t, root, planRoot, "00-checkout-v2", -1, "plan")
	bad := strings.Replace(checkout, plan.ChecklistPlaceholder, "- [ ] a hand-written checklist", 1)
	if _, err := Edit([]byte(bad), settings, root, false, false); err == nil || !strings.Contains(err.Error(), "derived checklist") {
		t.Fatalf("derived-region apply error = %v", err)
	}
	if _, err := Edit([]byte(checkout), settings, root, false, false); err != nil {
		t.Fatalf("unchanged plan checkout apply: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".planr.lock")); err == nil {
		t.Fatal("unexpected repository-root lock")
	}
}
