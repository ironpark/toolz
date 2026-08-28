// Package plantest holds draft fixtures shared by tests across packages, so a
// new field on draft.Draft is filled in one place instead of silently drifting
// between near-identical copies.
package plantest

import "github.com/ironpark/toolz/cli/planr/internal/draft"

// CheckoutOption customises the checkout-v2 fixture.
type CheckoutOption func(*draft.Draft)

// DependingOn makes the plan depend on the named plans.
func DependingOn(dependsOn []string) CheckoutOption {
	return func(d *draft.Draft) { d.DependsOn = dependsOn }
}

// WithUIPhase appends the Checkout UI phase, which depends on phase 0.
func WithUIPhase() CheckoutOption {
	return func(d *draft.Draft) {
		d.Phases = append(d.Phases, draft.Phase{
			Title:      "Checkout UI",
			Meta:       draft.Meta{Phase: 1, Slug: "checkout-ui", Status: "planned", DependsOn: []int{0}},
			Planned:    "Add the UI.",
			Completion: "UI tests pass.",
		})
	}
}

// CheckoutDraft returns the checkout-v2 plan draft.
func CheckoutDraft(options ...CheckoutOption) draft.Draft {
	result := draft.Draft{
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
	for _, option := range options {
		option(&result)
	}
	return result
}

// ApplyDraft returns the two-phase plan draft the apply and stdin tests register.
func ApplyDraft(name string) draft.Draft {
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

// OverviewDraft returns the single-phase plan draft the overview tests chain
// together. Its prose differs from CheckoutDraft, so it stays its own fixture.
func OverviewDraft(name string, dependency *string) draft.Draft {
	dependencies := []string{}
	if dependency != nil {
		dependencies = append(dependencies, *dependency)
	}
	return draft.Draft{
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
		Phases: []draft.Phase{{
			Title:      "API Contract",
			Meta:       draft.Meta{Phase: 0, Slug: "api-contract", Status: "planned"},
			Planned:    "Implement the API.",
			Completion: "API tests pass.",
		}},
	}
}
