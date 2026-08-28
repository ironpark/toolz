package jsonout

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/ironpark/toolz/cli/planr/internal/apply"
	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/doctor"
	"github.com/ironpark/toolz/cli/planr/internal/hooks"
	"github.com/ironpark/toolz/cli/planr/internal/notes"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	"github.com/ironpark/toolz/cli/planr/internal/validation"
)

// update regenerates the golden files instead of comparing against them:
//
//	go test ./internal/jsonout -update
var update = flag.Bool("update", false, "regenerate the golden output files")

// The `--json` schema is a public contract, so every output type is encoded
// from a fixed input and compared byte for byte. A renamed field, a changed
// JSON tag, a reordered struct field or a slice that starts encoding as null
// instead of [] all fail here rather than reaching a consumer.
func TestOutputSchemaGolden(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		value any
	}{
		{name: "status", value: Status(goldenSummaries())},
		{name: "overview", value: Overview(goldenSummaries())},
		{name: "show", value: Show(goldenPhaseDetails())},
		{name: "show_all", value: ShowAll(goldenDetails())},
		{name: "show_section", value: ShowSectionOutput{
			Plan:      "checkout-v2",
			Directory: "plan/00-checkout-v2",
			Section:   "GOALS",
			Content:   "Ship checkout.\n",
			File:      "plan/00-checkout-v2/GOALS.md",
		}},
		{name: "template", value: Template(apply.KindPlan, "checkout-v2", "---\nplan_name: checkout-v2\n---\n# plan\n")},
		{name: "edit", value: EditOutput{
			Kind:     apply.KindEdit,
			Selector: "checkout-v2#0",
			Section:  "GOALS",
			Target:   "plan/00-checkout-v2/phases/00-foundation.md",
			Base:     "0000000000000000000000000000000000000000000000000000000000000000",
			Document: "---\nplanr_edit: checkout-v2#0\n---\n# phase\n",
		}},
		{name: "apply", value: Apply(goldenOperation())},
		{name: "apply_failure", value: ApplyFailureOutput{Ok: false, Errors: Validation(goldenRecords())}},
		{name: "validation", value: Validation(goldenRecords())},
		{name: "config", value: Config(goldenConfig(), "/repo", "claude")},
		{name: "init", value: Init(
			"/repo/.planr.yaml", "/repo", doc.English,
			[]string{"plan"}, []string{"plan"}, []string{".planr.yaml"},
		)},
		{name: "doctor", value: Doctor([]doctor.Issue{{Location: "plan/00-checkout-v2/PLAN.md", Message: "missing phase file"}})},
		{name: "notes", value: Notes([]notes.Note{{
			Commit:    "1111111111111111111111111111111111111111",
			ShortHash: "1111111",
			Subject:   "finish the foundation",
			Plan:      "checkout-v2",
			Event:     "done",
			Phase:     "0",
			At:        "2024-01-02T03:04:05Z",
		}})},
		// Empty inputs pin the empty-slice-vs-null shape of every collection.
		{name: "status_empty", value: Status(nil)},
		{name: "overview_empty", value: Overview(nil)},
		{name: "apply_empty", value: Apply(apply.Operation{Action: "plan", Selector: "checkout-v2"})},
		{name: "notes_empty", value: Notes(nil)},
		{name: "doctor_empty", value: Doctor(nil)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var buffer bytes.Buffer
			encoder := json.NewEncoder(&buffer)
			encoder.SetIndent("", "  ")
			if err := encoder.Encode(testCase.value); err != nil {
				t.Fatalf("encode %s: %v", testCase.name, err)
			}
			assertGolden(t, testCase.name+".json", buffer.Bytes())
		})
	}
}

func assertGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file (rerun with -update to create it): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s does not match the golden file.\ngot:\n%s\nwant:\n%s", name, got, want)
	}
}

func goldenSummaries() []plan.Summary {
	return []plan.Summary{
		{
			Name:   "checkout-v2",
			Label:  "plan/00-checkout-v2",
			Status: "in-progress",
			Phases: []plan.StoredPhase{
				{ID: 0, Slug: "foundation", Title: "Foundation", Status: "done"},
				{ID: 1, Slug: "checkout-ui", Title: "Checkout UI", Status: "planned", Dependencies: []string{"foundation"}},
			},
			Wait: []string{"api-foundation#0"},
		},
		{
			Name:   "api-foundation",
			Label:  "plan/01-api-foundation",
			Status: "",
			Phases: []plan.StoredPhase{{ID: 0, Slug: "contract", Title: "Contract", Status: "done"}},
		},
	}
}

func goldenPhaseDetails() plan.PhaseDetails {
	return plan.PhaseDetails{
		Plan:         "checkout-v2",
		Directory:    "plan/00-checkout-v2",
		ID:           1,
		Slug:         "checkout-ui",
		Title:        "Checkout UI",
		Status:       "planned",
		PlannedWork:  "Add the UI.",
		DoneWhen:     "UI tests pass.",
		Dependencies: []string{"foundation"},
		File:         "plan/00-checkout-v2/phases/01-checkout-ui.md",
	}
}

func goldenDetails() plan.Details {
	return plan.Details{
		Plan:        "checkout-v2",
		Directory:   "plan/00-checkout-v2",
		Status:      "in-progress",
		Description: "checkout flow refresh",
		DependsOn:   []string{"api-foundation"},
		Phases:      []plan.PhaseDetails{goldenPhaseDetails()},
		Documents: map[string]string{
			"GOALS.md":                 "Ship checkout.\n",
			"CONTEXT.md":               "Existing checkout.\n",
			"PLAN.md":                  "---\nplan_name: checkout-v2\n---\n# plan\n",
			"phases/01-checkout-ui.md": "# Checkout UI\n",
		},
	}
}

func goldenOperation() apply.Operation {
	return apply.Operation{
		Action:    "plan",
		Selector:  "checkout-v2",
		DryRun:    true,
		Changed:   true,
		Documents: map[string]string{"plan/00-checkout-v2/PLAN.md": "# plan\n"},
		Diffs: []apply.Diff{{
			Path:   "plan/00-checkout-v2/PLAN.md",
			Before: "",
			After:  "# plan\n",
		}},
	}
}

func goldenRecords() []validation.Record {
	phase := 1
	return []validation.Record{
		{Rule: "placeholder", Section: "PHASES", Phase: &phase, Line: 42, Detail: "unfilled placeholder"},
		{Rule: "dependency_cycle", Phases: []int{0, 1}, Detail: "cycle detected"},
	}
}

func goldenConfig() config.Config {
	return config.Config{
		Path:      "/repo/.planr.yaml",
		PlansDirs: []string{"plan"},
		Ignore:    []string{"vendor"},
		Language:  doc.English,
		Hooks: hooks.Config{
			Timeout: hooks.DefaultTimeout,
			Before:  []hooks.Rule{{On: []string{"done"}, Run: "go test ./..."}},
			After:   []hooks.Rule{{On: []string{"new"}, Run: "echo created"}},
		},
	}
}
