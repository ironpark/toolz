package planlock

import (
	"strings"
	"testing"
	"time"
)

func TestAcquirePlanTimesOutWithClearError(t *testing.T) {
	root := t.TempDir()
	held, err := AcquirePlan(root)
	if err != nil {
		t.Fatalf("acquire first plan lock: %v", err)
	}
	t.Cleanup(func() { _ = held.Close() })

	_, err = acquireAdvisoryLock(root, "plan", false, 15*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "cannot acquire plan lock") || !strings.Contains(err.Error(), "within") {
		t.Fatalf("second acquire error = %v, want a clear timeout", err)
	}
}
