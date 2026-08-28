package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ironpark/toolz/cli/planr/internal/doc"
)

func TestCollectOverviewEntries(t *testing.T) {
	root := t.TempDir()
	plans := filepath.Join(root, "plans-active")
	planRoot := filepath.Join(plans, "00-checkout-v2")
	draft := overviewTestDraft("checkout-v2", nil)
	if err := writePlan(planRoot, draft, "00-checkout-v2", doc.Korean); err != nil {
		t.Fatalf("writePlan() unexpected error: %v", err)
	}

	entries, foundDirectory, err := collectPlanSummaries([]string{plans}, "")
	if err != nil {
		t.Fatalf("collectPlanSummaries() unexpected error: %v", err)
	}
	if !foundDirectory || len(entries) != 1 {
		t.Fatalf("collectPlanSummaries() found=%v entries=%d, want one plan", foundDirectory, len(entries))
	}
	entry := entries[0]
	done, total, next := entry.progress()
	if entry.name != "checkout-v2" || entry.status != "in-progress" || done != 0 || total != 1 {
		t.Fatalf("overview entry = %#v, want checkout-v2 in-progress 0/1", entry)
	}
	if next != "API Contract" {
		t.Fatalf("overview next = %q, want API Contract", next)
	}
}

func TestAnnotateOverviewWait(t *testing.T) {
	root := t.TempDir()
	plans := filepath.Join(root, "plans-active")
	apiRoot := filepath.Join(plans, "00-api-foundation")
	if err := writePlan(apiRoot, overviewTestDraft("api-foundation", nil), "00-api-foundation", doc.Korean); err != nil {
		t.Fatalf("write API plan: %v", err)
	}
	consumerRoot := filepath.Join(plans, "01-checkout-v2")
	dependency := "api-foundation#0"
	if err := writePlan(consumerRoot, overviewTestDraft("checkout-v2", &dependency), "01-checkout-v2", doc.Korean); err != nil {
		t.Fatalf("write consumer plan: %v", err)
	}

	entries, _, err := collectPlanSummaries([]string{plans}, "")
	if err != nil {
		t.Fatalf("collectPlanSummaries() unexpected error: %v", err)
	}
	annotatePlanWaits(entries)
	if len(entries) != 2 || len(entries[1].wait) != 1 || !strings.Contains(entries[1].wait[0], "api-foundation#0") {
		t.Fatalf("overview waits = %#v, want api-foundation#0", entries[1].wait)
	}
}

func overviewTestDraft(name string, dependency *string) draft {
	dependencies := []string{}
	if dependency != nil {
		dependencies = append(dependencies, *dependency)
	}
	return draft{
		Name:         name,
		Description:  "overview test",
		NextPhase:    0,
		NextText:     "Implement the API.",
		Goals:        "Ship the plan.",
		Scope:        "Test scope.",
		Context:      "Test context.",
		Verification: "go test ./...",
		Ordering:     "API first.",
		DependsOn:    dependencies,
		Phases: []draftPhase{{
			Title:      "API Contract",
			Meta:       phaseMeta{Phase: 0, Slug: "api-contract", Status: "planned"},
			Planned:    "Implement the API.",
			Completion: "API tests pass.",
		}},
	}
}
