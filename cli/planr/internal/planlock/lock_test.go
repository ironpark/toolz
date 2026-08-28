package planlock_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ironpark/toolz/cli/planr/internal/planlock"
)

func TestAcquirePlanTimesOutWithClearError(t *testing.T) {
	root := t.TempDir()
	held, err := planlock.AcquirePlan(root)
	if err != nil {
		t.Fatalf("acquire first plan lock: %v", err)
	}
	t.Cleanup(func() { _ = held.Close() })

	_, err = planlock.AcquirePlan(root, planlock.WithTimeout(15*time.Millisecond))
	if err == nil || !strings.Contains(err.Error(), "cannot acquire plan lock") || !strings.Contains(err.Error(), "within") {
		t.Fatalf("second acquire error = %v, want a clear timeout", err)
	}
}
