package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/ironpark/toolz/cli/planr/cli/phase"
	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/gitrepo"
	"github.com/ironpark/toolz/cli/planr/internal/hooks"
	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
	"github.com/ironpark/toolz/cli/planr/internal/notes"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
)

// seedRepository builds a repository with one commit so notes have a target.
func seedRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	repository, err := git.PlainInit(root, false)
	if err != nil {
		t.Fatalf("git.PlainInit() unexpected error: %v", err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "seed.txt"), []byte("seed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add("seed.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Commit("seed commit", &git.CommitOptions{
		Author: &object.Signature{Name: "planr", Email: "planr@example.com", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}
	return root
}

func testDraft() draft.Draft {
	return draft.Draft{
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
		},
	}
}

func TestFrontmatterOmitsEmptyMetadata(t *testing.T) {
	plansRoot := t.TempDir()
	planRoot := filepath.Join(plansRoot, "00-checkout-v2")
	if err := plan.Write(planRoot, testDraft(), "00-checkout-v2", doc.Korean); err != nil {
		t.Fatalf("plan.Write() unexpected error: %v", err)
	}

	for _, path := range []string{
		filepath.Join(planRoot, "PLAN.md"),
		filepath.Join(planRoot, "phases", "00-api-contract.md"),
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		front, _, err := mdoc.Split(string(raw))
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for key, value := range front {
			if mdoc.IsEmptyMeta(value) {
				t.Errorf("%s kept empty metadata %q: %#v", filepath.Base(path), key, value)
			}
		}
	}

	// The plan had no dependencies and no successors, so those keys are absent.
	raw, _ := os.ReadFile(filepath.Join(planRoot, "PLAN.md"))
	for _, key := range []string{"depends_on", "succeeded_by", "preceded_by"} {
		if strings.Contains(string(raw), key+":") {
			t.Errorf("PLAN.md still writes empty %q", key)
		}
	}
}

func TestCompletionStampsFrontmatter(t *testing.T) {
	plansRoot := t.TempDir()
	planRoot := filepath.Join(plansRoot, "00-checkout-v2")
	if err := plan.Write(planRoot, testDraft(), "00-checkout-v2", doc.Korean); err != nil {
		t.Fatalf("plan.Write() unexpected error: %v", err)
	}

	if _, done, err := updatePhaseStatus([]string{plansRoot}, "checkout-v2", 0, "done"); err != nil || !done {
		t.Fatalf("complete phase: done=%v err=%v", done, err)
	}
	phaseStamp := frontmatterValue(t, filepath.Join(planRoot, "phases", "00-api-contract.md"), "completed_at")
	planStamp := frontmatterValue(t, filepath.Join(planRoot, "PLAN.md"), "completed_at")
	if phaseStamp == "" || planStamp == "" {
		t.Fatalf("completed_at missing: phase=%q plan=%q", phaseStamp, planStamp)
	}

	// Reopening must clear both stamps so a stale date never lingers.
	if _, _, err := updatePhaseStatus([]string{plansRoot}, "checkout-v2", 0, "planned"); err != nil {
		t.Fatalf("reopen phase: %v", err)
	}
	if got := frontmatterValue(t, filepath.Join(planRoot, "phases", "00-api-contract.md"), "completed_at"); got != "" {
		t.Errorf("phase completed_at survived reopening: %q", got)
	}
	if got := frontmatterValue(t, filepath.Join(planRoot, "PLAN.md"), "completed_at"); got != "" {
		t.Errorf("plan completed_at survived reopening: %q", got)
	}
}

func TestPhaseStartRecordsNoteForCurrentHead(t *testing.T) {
	root := seedRepository(t)
	if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), []byte("plans_dir: plan\n"), 0644); err != nil {
		t.Fatal(err)
	}
	planRoot := filepath.Join(root, "plan", "00-checkout-v2")
	if err := plan.Write(planRoot, testDraft(), "00-checkout-v2", doc.English); err != nil {
		t.Fatal(err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	if err := phase.Command(nil).Run(context.Background(), []string{"phase", "start", "checkout-v2", "0"}); err != nil {
		t.Fatalf("phase start failed: %v", err)
	}
	recorded, err := notes.Read(root, "checkout-v2")
	if err != nil {
		t.Fatalf("notes.Read() unexpected error: %v", err)
	}
	if len(recorded) != 1 || recorded[0].Event != hooks.EventStart || recorded[0].Phase != "00" {
		t.Fatalf("start notes = %#v, want one start note for phase 00", recorded)
	}
	jsonNotes := makeNotesJSON(recorded)
	if len(jsonNotes.Notes) != 1 || jsonNotes.Notes[0].Event != hooks.EventStart || jsonNotes.Notes[0].Phase != "00" {
		t.Fatalf("start JSON notes = %#v, want the start event", jsonNotes)
	}
}

func frontmatterValue(t *testing.T, path, key string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	front, _, err := mdoc.Split(string(raw))
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	value, _ := front[key].(string)
	return value
}

func TestRecordAndReadCompletionNotes(t *testing.T) {
	repoRoot := seedRepository(t)

	if err := notes.RecordCompletion(repoRoot, "00-checkout-v2", hooks.EventDone, 3); err != nil {
		t.Fatalf("record phase note: %v", err)
	}
	if err := notes.RecordCompletion(repoRoot, "00-checkout-v2", hooks.EventPlanDone, -1); err != nil {
		t.Fatalf("record plan note: %v", err)
	}
	if err := notes.RecordCompletion(repoRoot, "01-other", hooks.EventPlanDone, -1); err != nil {
		t.Fatalf("record other plan note: %v", err)
	}

	recorded, err := notes.Read(repoRoot, "")
	if err != nil {
		t.Fatalf("notes.Read() unexpected error: %v", err)
	}
	if len(recorded) != 3 {
		t.Fatalf("expected 3 notes, got %d: %#v", len(recorded), recorded)
	}
	for _, note := range recorded {
		if note.Subject != "seed commit" || note.ShortHash == "" {
			t.Errorf("note not linked to the commit: %#v", note)
		}
	}

	filtered, err := notes.Read(repoRoot, "00-checkout-v2")
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 2 {
		t.Fatalf("plan filter returned %d notes, want 2", len(filtered))
	}
	var phase string
	for _, note := range filtered {
		if note.Event == hooks.EventDone {
			phase = note.Phase
		}
	}
	if phase != "03" {
		t.Errorf("phase number not recorded, got %q", phase)
	}
}

// `notes` must accept the same plan names every other command does; the bare
// name previously returned an empty result instead of the plan's records.
func TestReadPlanNotesAcceptsBarePlanName(t *testing.T) {
	repoRoot := seedRepository(t)
	if err := notes.RecordCompletion(repoRoot, "00-checkout-v2", hooks.EventDone, 1); err != nil {
		t.Fatalf("record phase note: %v", err)
	}
	if err := notes.RecordCompletion(repoRoot, "01-other", hooks.EventPlanDone, -1); err != nil {
		t.Fatalf("record other plan note: %v", err)
	}

	for _, filter := range []string{"checkout-v2", "00-checkout-v2"} {
		notes, err := notes.Read(repoRoot, filter)
		if err != nil {
			t.Fatalf("notes.Read(%q) unexpected error: %v", filter, err)
		}
		if len(notes) != 1 || notes[0].Plan != "00-checkout-v2" {
			t.Fatalf("notes.Read(%q) = %#v, want the one checkout-v2 note", filter, notes)
		}
	}
	notes, err := notes.Read(repoRoot, "nonexistent")
	if err != nil {
		t.Fatalf("notes.Read() unexpected error: %v", err)
	}
	if len(notes) != 0 {
		t.Fatalf("notes.Read(\"nonexistent\") = %#v, want none", notes)
	}
}

func TestReadPlanNotesWithoutRecords(t *testing.T) {
	repoRoot := t.TempDir()
	if _, err := git.PlainInit(repoRoot, false); err != nil {
		t.Fatal(err)
	}
	recorded, err := notes.Read(repoRoot, "")
	if err != nil {
		t.Fatalf("empty notes ref should not error: %v", err)
	}
	if len(recorded) != 0 {
		t.Fatalf("expected no notes, got %d", len(recorded))
	}
}

func TestEnsureGitRepository(t *testing.T) {
	repoRoot := seedRepository(t)
	nested := filepath.Join(repoRoot, "plans", "00-demo")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	// A subdirectory of the repository counts as inside it.
	if err := gitrepo.EnsureRepository(nested); err != nil {
		t.Errorf("nested path rejected: %v", err)
	}

	outside := t.TempDir()
	err := gitrepo.EnsureRepository(outside)
	if err == nil {
		t.Fatal("expected an error outside a git repository")
	}
	if !strings.Contains(err.Error(), "git init") {
		t.Errorf("error does not tell the user what to do: %v", err)
	}
}
