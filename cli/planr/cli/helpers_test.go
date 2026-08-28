package cli

import (
	"context"
	"testing"

	"github.com/ironpark/toolz/cli/planr/internal/plan"
	"github.com/ironpark/toolz/cli/planr/internal/planlock"
)

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
