package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/toolz/cli/planr/internal/doc"
	"github.com/ironpark/toolz/cli/planr/internal/draft"
	"github.com/ironpark/toolz/cli/planr/internal/mdoc"
	"github.com/ironpark/toolz/cli/planr/internal/plan"
)

func TestNormalizeDescription(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		required    bool
		want        string
		wantErrText string
	}{
		{name: "trims surrounding whitespace", value: "  short plan  ", required: true, want: "short plan"},
		{name: "allows 200 unicode characters", value: strings.Repeat("가", 200), required: true, want: strings.Repeat("가", 200)},
		{name: "rejects empty required description", value: "   ", required: true, wantErrText: "requires --description"},
		{name: "rejects more than 200 characters", value: strings.Repeat("a", 201), required: true, wantErrText: "200 characters or fewer"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := plan.NormalizeDescription(test.value, test.required)
			if test.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErrText) {
					t.Fatalf("plan.NormalizeDescription() error = %v, want text %q", err, test.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("plan.NormalizeDescription() unexpected error: %v", err)
			}
			if got != test.want {
				t.Fatalf("plan.NormalizeDescription() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestNormalizePlanDependencies(t *testing.T) {
	got, err := plan.NormalizeDependencies([]string{"platform-refresh", "api-foundation#2"}, "checkout-v2")
	if err != nil {
		t.Fatalf("plan.NormalizeDependencies() unexpected error: %v", err)
	}
	want := []string{"platform-refresh", "api-foundation#2"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("plan.NormalizeDependencies() = %#v, want %#v", got, want)
	}

	for _, test := range []struct {
		name         string
		dependencies []string
		wantErrText  string
	}{
		{name: "self plan", dependencies: []string{"checkout-v2"}, wantErrText: "cannot depend on itself"},
		{name: "self phase", dependencies: []string{"checkout-v2#1"}, wantErrText: "cannot depend on itself"},
		{name: "duplicate", dependencies: []string{"platform-refresh", "platform-refresh"}, wantErrText: "duplicate"},
		{name: "invalid phase", dependencies: []string{"platform-refresh#phase"}, wantErrText: "non-negative phase number"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := plan.NormalizeDependencies(test.dependencies, "checkout-v2")
			if err == nil || !strings.Contains(err.Error(), test.wantErrText) {
				t.Fatalf("plan.NormalizeDependencies() error = %v, want text %q", err, test.wantErrText)
			}
		})
	}
}

func TestWritePlanStoresDescriptionAndRegisteredAt(t *testing.T) {
	root := t.TempDir()
	planDraft := draft.Draft{
		Name:         "checkout-v2",
		Description:  "checkout flow refresh",
		DependsOn:    []string{"platform-refresh#2"},
		Goals:        "Ship the checkout refresh.",
		Scope:        "Checkout only.",
		Context:      "Existing checkout flow.",
		Verification: "go test ./...",
		Ordering:     "API before UI.",
		NextText:     "Implement the API contract.",
		NextPhase:    0,
		Phases: []draft.Phase{
			{
				Title:   "API Contract",
				Meta:    draft.Meta{Phase: 0, Slug: "api-contract", Status: "planned"},
				Planned: "Add the API contract.", Completion: "The contract test passes.",
			},
		},
	}

	if err := writePlan(root, planDraft, "00-checkout-v2", doc.Korean); err != nil {
		t.Fatalf("writePlan() unexpected error: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "PLAN.md"))
	if err != nil {
		t.Fatalf("read PLAN.md: %v", err)
	}
	front, _, err := mdoc.Split(string(raw))
	if err != nil {
		t.Fatalf("mdoc.Split() unexpected error: %v", err)
	}
	if got, _ := front["description"].(string); got != planDraft.Description {
		t.Fatalf("description = %q, want %q", got, planDraft.Description)
	}
	if got, _ := front["registered_at"].(string); got == "" {
		t.Fatal("registered_at is empty")
	} else if _, err := time.Parse(time.RFC3339, got); err != nil {
		t.Fatalf("registered_at = %q is not RFC3339: %v", got, err)
	}
}
