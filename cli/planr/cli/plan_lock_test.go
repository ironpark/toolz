package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/ironpark/toolz/cli/planr/internal/plan"
)

func TestPlanLockTimesOutWithClearError(t *testing.T) {
	root := t.TempDir()
	held, err := plan.AcquireLock(root)
	if err != nil {
		t.Fatalf("acquire first plan lock: %v", err)
	}
	t.Cleanup(func() { _ = held.Close() })

	oldTimeout := plan.LockTimeout
	plan.LockTimeout = 15 * time.Millisecond
	t.Cleanup(func() { plan.LockTimeout = oldTimeout })
	_, err = plan.AcquireLock(root)
	if err == nil || !strings.Contains(err.Error(), "cannot acquire plan lock") || !strings.Contains(err.Error(), "within") {
		t.Fatalf("second acquire error = %v, want a clear timeout", err)
	}
}
