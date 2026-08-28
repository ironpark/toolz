package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	"github.com/urfave/cli/v3"
)

func TestPhaseRemoveRefusesDependentsAndLeavesNumberGapWithForce(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), []byte("plans_dir: plan\n"), 0644); err != nil {
		t.Fatal(err)
	}
	planRoot := filepath.Join(root, "plan", "00-checkout-v2")
	planDraft := dependentTestDraft(nil)
	planDraft.Phases = append(planDraft.Phases, draft.Phase{
		Title:   "Rollout",
		Meta:    draft.Meta{Phase: 2, Slug: "rollout", Status: "planned", DependsOn: []int{1}},
		Planned: "Roll out.", Completion: "Rollout is stable.",
	})
	if err := plan.Write(planRoot, planDraft, "00-checkout-v2", doc.English); err != nil {
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
	remove := func(args ...string) error {
		return (&cli.Command{
			Name:   "rm",
			Flags:  []cli.Flag{&cli.BoolFlag{Name: "force"}},
			Action: phaseRemoveCommand,
		}).Run(context.Background(), append([]string{"rm"}, args...))
	}
	if err := remove("checkout-v2", "1"); err == nil || !strings.Contains(err.Error(), "depend on it") {
		t.Fatalf("phase rm error = %v, want dependent-phase refusal", err)
	}
	if err := remove("checkout-v2", "1", "--force"); err != nil {
		t.Fatalf("forced phase rm failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(planRoot, "phases", "01-checkout-ui.md")); !os.IsNotExist(err) {
		t.Fatalf("removed phase still exists; stat error = %v", err)
	}
	planRaw, err := os.ReadFile(filepath.Join(planRoot, "PLAN.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(planRaw), "Phase 01:") || !strings.Contains(string(planRaw), "Phase 02: Rollout") {
		t.Fatalf("PLAN.md did not preserve the numbering gap:\n%s", planRaw)
	}
}
