package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
	"github.com/ironpark/toolz/cli/planr/internal/plantest"
)

func TestCollectOverviewEntries(t *testing.T) {
	root := t.TempDir()
	plans := filepath.Join(root, "plans-active")
	planRoot := filepath.Join(plans, "00-checkout-v2")
	planDraft := plantest.OverviewDraft("checkout-v2")
	if err := plan.Write(planRoot, planDraft, "00-checkout-v2", doc.Korean); err != nil {
		t.Fatalf("plan.Write() unexpected error: %v", err)
	}

	entries, foundDirectory, err := plan.CollectSummaries([]string{plans}, "")
	if err != nil {
		t.Fatalf("plan.CollectSummaries() unexpected error: %v", err)
	}
	if !foundDirectory || len(entries) != 1 {
		t.Fatalf("plan.CollectSummaries() found=%v entries=%d, want one plan", foundDirectory, len(entries))
	}
	entry := entries[0]
	done, total, next := entry.Progress()
	if entry.Name != "checkout-v2" || entry.Status != "in-progress" || done != 0 || total != 1 {
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
	if err := plan.Write(apiRoot, plantest.OverviewDraft("api-foundation"), "00-api-foundation", doc.Korean); err != nil {
		t.Fatalf("write API plan: %v", err)
	}
	consumerRoot := filepath.Join(plans, "01-checkout-v2")
	dependency := "api-foundation#0"
	if err := plan.Write(consumerRoot, plantest.OverviewDraft("checkout-v2", dependency), "01-checkout-v2", doc.Korean); err != nil {
		t.Fatalf("write consumer plan: %v", err)
	}

	entries, _, err := plan.CollectSummaries([]string{plans}, "")
	if err != nil {
		t.Fatalf("plan.CollectSummaries() unexpected error: %v", err)
	}
	plan.AnnotateWaits(entries)
	if len(entries) != 2 || len(entries[1].Wait) != 1 || !strings.Contains(entries[1].Wait[0], "api-foundation#0") {
		t.Fatalf("overview waits = %#v, want api-foundation#0", entries[1].Wait)
	}
}
