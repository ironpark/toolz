package cmd

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ironpark/toolz/cli/planr/internal/plan"
	"github.com/ironpark/toolz/cli/planr/internal/planlock"
	"github.com/ironpark/toolz/cli/planr/internal/plantest"
	"github.com/ironpark/toolz/cli/planr/internal/vfs"
)

// writeDraft writes the checkout fixture into root as <name>.md, registered
// under the given plan name and dependencies, and returns the path it wrote.
// It is the input side of every command-level test that needs a real plan:
// the tests shape planr's input rather than the documents planr generates.
func writeDraft(t *testing.T, root, name string, dependsOn []string) string {
	t.Helper()
	body, err := plantest.DraftBody(plantest.Fixtures(), plantest.CheckoutFixture)
	if err != nil {
		t.Fatalf("plantest.DraftBody() unexpected error: %v", err)
	}
	document, err := plantest.DraftDocument(name, dependsOn, body)
	if err != nil {
		t.Fatalf("plantest.DraftDocument(%s) unexpected error: %v", name, err)
	}
	path := filepath.Join(root, name+".md")
	if err := vfs.WriteFile(path, []byte(document), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeConfig writes root's .planr.yaml through vfs, so a test running against
// a swapped-in filesystem finds it where planr looks.
func writeConfig(t *testing.T, root, body string) {
	t.Helper()
	if err := vfs.WriteFile(filepath.Join(root, ".planr.yaml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

// runRoot runs one command through the real root command and returns what it
// printed, which is how the command-level tests observe planr.
func runRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	return captureOutput(t, func() error {
		return newRootCommand().Run(context.Background(), append([]string{"planr"}, args...))
	})
}

// updatePhaseStatus is a test helper: it resolves a plan, takes its lock, and
// updates one phase's status, matching what the phase command does without the
// hook and dependency machinery around it.
func updatePhaseStatus(planDirectories []string, planArg string, phaseID int, status string) (bool, error) {
	planRoot, planDirectory, err := plan.FindDirectory(planDirectories, planArg)
	if err != nil {
		return false, err
	}
	planLock, err := planlock.AcquirePlan(planRoot)
	if err != nil {
		return false, err
	}
	defer planLock.Close()
	return plan.UpdatePhaseStatusLocked(planRoot, planDirectory, phaseID, status)
}
