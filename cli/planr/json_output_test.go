package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ironpark/toolz/cli/planr/internal/config"
	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/hooks"
)

func TestStatusJSONUsesStableSnakeCaseFields(t *testing.T) {
	phase := 1
	summaries := []planSummary{{
		name:   "checkout-v2",
		label:  "plan/00-checkout-v2",
		status: "in-progress",
		phases: []storedPhase{
			{id: 0, slug: "foundation", title: "Foundation", status: "done"},
			{id: phase, slug: "ui", title: "Checkout UI", status: "planned"},
		},
		wait: []string{"platform-refresh"},
	}}

	raw, err := json.Marshal(makeStatusJSON(summaries))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["plans"]; !ok {
		t.Fatalf("status JSON has no plans field: %s", raw)
	}
	plans := decoded["plans"].([]any)
	plan := plans[0].(map[string]any)
	for _, key := range []string{"name", "directory", "status", "done_phases", "total_phases", "remaining", "wait"} {
		if _, ok := plan[key]; !ok {
			t.Errorf("status JSON missing %q: %s", key, raw)
		}
	}
	if _, ok := plan["donePhases"]; ok {
		t.Error("status JSON used a camelCase field")
	}
	remaining := plan["remaining"].([]any)
	if len(remaining) != 1 || remaining[0].(map[string]any)["phase_number"] != float64(phase) {
		t.Fatalf("remaining phase = %#v, want phase %d", remaining, phase)
	}
}

func TestOverviewAndNotesJSONKeepEmptyArrays(t *testing.T) {
	overview, err := json.Marshal(makeOverviewJSON(nil))
	if err != nil {
		t.Fatal(err)
	}
	if string(overview) != `{"plans":[]}` {
		t.Fatalf("empty overview JSON = %s, want an empty plans array", overview)
	}
	notes, err := json.Marshal(makeNotesJSON(nil))
	if err != nil {
		t.Fatal(err)
	}
	if string(notes) != `{"notes":[]}` {
		t.Fatalf("empty notes JSON = %s, want an empty notes array", notes)
	}

	value, err := json.Marshal(makeNotesJSON([]planNote{{
		at: "2026-08-27T00:00:00Z", plan: "00-demo", event: hooks.EventDone,
		phase: "01", commit: "0123456789abcdef", shortHash: "0123456", subject: "finish phase",
	}}))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string][]map[string]string
	if err := json.Unmarshal(value, &decoded); err != nil {
		t.Fatal(err)
	}
	if got := decoded["notes"][0]["completed_at"]; got != "2026-08-27T00:00:00Z" {
		t.Fatalf("completed_at = %q", got)
	}
	if got := decoded["notes"][0]["short_commit"]; got != "0123456" {
		t.Fatalf("short_commit = %q", got)
	}
}

func TestShowJSONContainsStructuredPhaseContent(t *testing.T) {
	value, err := json.Marshal(makeShowJSON(phaseDetails{
		plan: "checkout-v2", directory: "00-checkout-v2", id: 2,
		slug: "rollout", title: "Gradual Rollout", status: "conditional",
		plannedWork: "- move traffic", doneWhen: "- metrics are stable",
		dependencies: []string{"00-checkout-v2#1"},
		file:         filepath.Join(string(filepath.Separator), "repo", "phases", "02-rollout.md"),
	}))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(value, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"plan", "directory", "phase_number", "slug", "title", "status", "planned_work", "done_when", "depends_on", "file"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("show JSON missing %q: %s", key, value)
		}
	}
	if got := decoded["planned_work"]; got != "- move traffic" {
		t.Errorf("planned_work = %#v", got)
	}
}

func TestDiagnosticJSONUsesStableStructuredFields(t *testing.T) {
	config, err := json.Marshal(makeConfigJSON(config.Config{
		PlansDirs: []string{"plans-active", "plans-archive"},
		Ignore:    []string{"tmp/**"},
		Language:  doc.Korean,
		Hooks: hooks.Config{
			Timeout: 5,
			Before:  []hooks.Rule{{On: []string{hooks.EventDone}, Run: "go test ./..."}},
		},
	}, "/repo"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), `"plans_dirs":["plans-active","plans-archive"]`) ||
		!strings.Contains(string(config), `"timeout":"5ns"`) {
		t.Fatalf("config JSON = %s", config)
	}

	doctor, err := json.Marshal(makeDoctorJSON([]doctorIssue{{location: "plan/PLAN.md", message: "broken checklist"}}))
	if err != nil {
		t.Fatal(err)
	}
	if string(doctor) != `{"issues":[{"location":"plan/PLAN.md","message":"broken checklist"}]}` {
		t.Fatalf("doctor JSON = %s", doctor)
	}
	empty, err := json.Marshal(makeDoctorJSON(nil))
	if err != nil {
		t.Fatal(err)
	}
	if string(empty) != `{"issues":[]}` {
		t.Fatalf("empty doctor JSON = %s", empty)
	}
}
