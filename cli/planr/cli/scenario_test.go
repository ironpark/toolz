package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ironpark/toolz/cli/planr/internal/plantest"
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

// scenarioPhaseCount is the number of phases in the fixture draft. The
// scenario completes phases by number, so a fixture with fewer phases has to
// fail rather than quietly produce a thinner scenario.
const scenarioPhaseCount = 3

func TestCheckoutReleaseScenarioReportsPlanStates(t *testing.T) {
	root := seedRepository(t)
	if err := os.WriteFile(filepath.Join(root, ".planr.yaml"), []byte("language: ko\nplans_dirs:\n  - plans-active\n  - plans-archive\n"), 0644); err != nil {
		t.Fatal(err)
	}
	withWorkingDirectory(t, root)

	body, err := plantest.DraftBody(plantest.Fixtures(), plantest.CheckoutFixture)
	if err != nil {
		t.Fatalf("plantest.DraftBody() unexpected error: %v", err)
	}
	for _, name := range scenarioPlans {
		document := plantest.DraftDocument(name, scenarioDependencies[name], body)
		draftPath := filepath.Join(root, name+".md")
		if err := os.WriteFile(draftPath, []byte(document), 0644); err != nil {
			t.Fatal(err)
		}
		if output, err := runScenarioCommand(t, "apply", name+".md"); err != nil {
			t.Fatalf("apply %s: %v; output=%q", name, err, output)
		}
	}

	// A finished dependency, and an unrelated finished plan that `status` hides.
	completeScenarioPhases(t, scenarioAuth, scenarioPhaseCount)
	completeScenarioPhases(t, scenarioLegacy, scenarioPhaseCount)
	// Some phases done, some outstanding.
	completeScenarioPhases(t, scenarioRollout, 1)
	// Work under way on the plan whose dependency is already satisfied.
	if output, err := runScenarioCommand(t, "phase", "start", scenarioCheckout, "0"); err != nil {
		t.Fatalf("start %s phase 0: %v; output=%q", scenarioCheckout, err, output)
	}

	status, err := runScenarioCommand(t, "status")
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

	overview, err := runScenarioCommand(t, "overview")
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

	notes, err := runScenarioCommand(t, "notes")
	if err != nil {
		t.Fatalf("notes: %v; output=%q", err, notes)
	}
	// Seven completed phases, two completed plans and one started phase are
	// recorded against the single seed commit.
	for _, want := range []string{
		"00-auth-foundation  done 02",
		"00-auth-foundation  plan_done",
		"01-checkout-v2      start 00",
		"04-partial-rollout  done 00",
	} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes is missing %q:\n%s", want, notes)
		}
	}
	if strings.Contains(notes, "04-partial-rollout  plan_done") {
		t.Errorf("notes records an unfinished plan as done:\n%s", notes)
	}
}

// completeScenarioPhases completes the first count phases of a plan in order.
// Order matters: planr refuses to complete a phase whose dependencies are
// still open, which is exactly the guarantee the scenario should respect.
func completeScenarioPhases(t *testing.T, planName string, count int) {
	t.Helper()
	for phaseID := range count {
		if output, err := runScenarioCommand(t, "phase", "done", planName, fmt.Sprint(phaseID)); err != nil {
			t.Fatalf("done %s phase %d: %v; output=%q", planName, phaseID, err, output)
		}
	}
}

func runScenarioCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	return captureOutput(t, func() error {
		return newRootCommand().Run(context.Background(), append([]string{"planr"}, args...))
	})
}

func TestScenarioDependenciesNameRegisteredPlans(t *testing.T) {
	// A dependency on a plan the scenario never registers would show up as
	// `(not found)` in the output instead of the wait it is meant to show.
	registered := map[string]bool{}
	for _, name := range scenarioPlans {
		registered[name] = true
	}
	for planName, dependencies := range scenarioDependencies {
		if !registered[planName] {
			t.Errorf("dependency declared for unregistered plan %q", planName)
		}
		for _, dependency := range dependencies {
			target, _, _ := strings.Cut(dependency, "#")
			if !registered[target] {
				t.Errorf("%s depends on unregistered plan %q", planName, target)
			}
		}
	}
}
