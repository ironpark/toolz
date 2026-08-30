package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/ironpark/toolz/cli/planr/internal/jsonout"
)

// The checkout release scenario: a finished plan, a plan waiting on another
// plan's finished phase, a plan blocked on an open one, a partially finished
// plan, and an unrelated finished plan that `status` hides.  None of those
// states come out of a single `apply`, so the scenario shapes planr's *input*
// -- one draft per plan, with the plan dependency in its frontmatter -- and
// then drives the real phase commands.  Nothing here edits a document planr
// generated: rewriting generated frontmatter would couple the test to an
// internal file format and could stage arrangements the CLI itself refuses,
// such as a phase marked done ahead of its dependencies.
const (
	scenarioAuth     = "auth-foundation"
	scenarioCheckout = "checkout-v2"
	scenarioPayment  = "payment-adapter"
	scenarioLegacy   = "legacy-report"
	scenarioRollout  = "partial-rollout"
)

// scenarioPlans is in registration order, which fixes the numeric prefix of
// each plan directory: auth becomes 00-auth-foundation, and so on.
var scenarioPlans = []string{scenarioAuth, scenarioCheckout, scenarioPayment, scenarioLegacy, scenarioRollout}

// scenarioDependencies are the plan-level dependencies written into each
// draft.  checkout waits on a phase that ends up done, so it never appears in
// `wait` but does keep the finished auth plan visible in `status`; payment
// waits on a phase that stays open, so it is the one blocking entry.
var scenarioDependencies = map[string][]string{
	scenarioCheckout: {scenarioAuth + "#2"},
	scenarioPayment:  {scenarioCheckout + "#1"},
}

// scenarioPhaseCount is the number of phases in the fixture draft; the
// expectations below spell it out as `3/3 phases`, so a thinner fixture fails
// rather than quietly producing a thinner scenario.
const scenarioPhaseCount = 3

func TestCheckoutReleaseScenarioReportsPlanStates(t *testing.T) {
	root := seedRepository(t)
	writeConfig(t, root, "language: ko\nplans_dirs:\n  - plans-active\n  - plans-archive\n")
	withWorkingDirectory(t, root)

	for _, name := range scenarioPlans {
		writeDraft(t, root, name, scenarioDependencies[name])
		if output, err := runRoot(t, "apply", name+".md"); err != nil {
			t.Fatalf("apply %s: %v; output=%q", name, err, output)
		}
	}

	// A finished dependency, and an unrelated finished plan that `status` hides.
	completeScenarioPhases(t, scenarioAuth, scenarioPhaseCount)
	completeScenarioPhases(t, scenarioLegacy, scenarioPhaseCount)
	// Some phases done, some outstanding.
	completeScenarioPhases(t, scenarioRollout, 1)
	// Work under way on the plan whose dependency is already satisfied.
	if output, err := runRoot(t, "phase", "start", scenarioCheckout, "0"); err != nil {
		t.Fatalf("start %s phase 0: %v; output=%q", scenarioCheckout, err, output)
	}

	status, err := runRoot(t, "status")
	if err != nil {
		t.Fatalf("status: %v; output=%q", err, status)
	}
	// The finished auth plan stays visible because checkout still depends on
	// it; the equally finished legacy plan nothing depends on drops out.
	for _, want := range []string{
		"00-auth-foundation: done (3/3 phases done)",
		"01-checkout-v2: in-progress (0/3 phases done)",
		"02-payment-adapter: in-progress (0/3 phases done)",
		"04-partial-rollout: in-progress (1/3 phases done)",
	} {
		if !strings.Contains(status, want) {
			t.Errorf("status is missing %q:\n%s", want, status)
		}
	}
	if strings.Contains(status, scenarioLegacy) {
		t.Errorf("status shows the finished %s plan nothing waits on:\n%s", scenarioLegacy, status)
	}
	// payment waits on checkout phase 1, which is still planned; checkout's own
	// dependency is done, so it waits on nothing.
	if !strings.Contains(status, "- checkout-v2#1 (planned)") {
		t.Errorf("status does not report the blocking dependency:\n%s", status)
	}
	if strings.Contains(status, "- auth-foundation#2") {
		t.Errorf("status reports a satisfied dependency as a wait:\n%s", status)
	}

	overview, err := runRoot(t, "overview")
	if err != nil {
		t.Fatalf("overview: %v; output=%q", err, overview)
	}
	// Unlike `status`, overview lists every plan, finished ones included.
	for _, want := range []string{
		"00-auth-foundation: done (3/3 phases)",
		"01-checkout-v2: in-progress (0/3 phases); next: API Contract",
		"03-legacy-report: done (3/3 phases)",
		"04-partial-rollout: in-progress (1/3 phases); next: Checkout UI",
	} {
		if !strings.Contains(overview, want) {
			t.Errorf("overview is missing %q:\n%s", want, overview)
		}
	}

	// notes is read as JSON: its text form is a tabwriter table whose column
	// widths depend on the longest plan name, so substring matching on it would
	// break when the scenario gains a plan.
	raw, err := runRoot(t, "notes", "--json")
	if err != nil {
		t.Fatalf("notes: %v; output=%q", err, raw)
	}
	var recorded jsonout.NotesOutput
	if err := json.Unmarshal([]byte(raw), &recorded); err != nil {
		t.Fatalf("decode notes: %v; output=%q", err, raw)
	}
	events := map[string]bool{}
	for _, note := range recorded.Notes {
		events[fmt.Sprintf("%s %s %s", note.Plan, note.Event, note.Phase)] = true
	}
	// Seven completed phases, two completed plans and one started phase are
	// recorded against the single seed commit.
	if len(recorded.Notes) != 10 {
		t.Errorf("notes recorded %d events, want 10:\n%s", len(recorded.Notes), raw)
	}
	for _, want := range []string{
		"00-auth-foundation done 02",
		"00-auth-foundation plan_done ",
		"01-checkout-v2 start 00",
		"04-partial-rollout done 00",
	} {
		if !events[want] {
			t.Errorf("notes is missing %q:\n%s", want, raw)
		}
	}
	if events["04-partial-rollout plan_done "] {
		t.Errorf("notes records an unfinished plan as done:\n%s", raw)
	}
}

// completeScenarioPhases completes the first count phases of a plan in order.
// Order matters: planr refuses to complete a phase whose dependencies are
// still open, which is exactly the guarantee the scenario should respect.
func completeScenarioPhases(t *testing.T, planName string, count int) {
	t.Helper()
	for phaseID := range count {
		if output, err := runRoot(t, "phase", "done", planName, fmt.Sprint(phaseID)); err != nil {
			t.Fatalf("done %s phase %d: %v; output=%q", planName, phaseID, err, output)
		}
	}
}
