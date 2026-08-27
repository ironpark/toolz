package main

import (
	"strings"
	"testing"
	"time"
)

func TestPlanLockTimesOutWithClearError(t *testing.T) {
	root := t.TempDir()
	held, err := acquirePlanLock(root)
	if err != nil {
		t.Fatalf("acquire first plan lock: %v", err)
	}
	t.Cleanup(func() { _ = held.close() })

	oldTimeout := planLockTimeout
	planLockTimeout = 15 * time.Millisecond
	t.Cleanup(func() { planLockTimeout = oldTimeout })
	_, err = acquirePlanLock(root)
	if err == nil || !strings.Contains(err.Error(), "cannot acquire plan lock") || !strings.Contains(err.Error(), "within") {
		t.Fatalf("second acquire error = %v, want a clear timeout", err)
	}
}
