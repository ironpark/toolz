package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateHooks(t *testing.T) {
	valid := Config{
		Before: []Rule{{On: []string{EventAdd, EventDone}, Run: "echo check"}},
	}
	if err := Validate(valid); err != nil {
		t.Fatalf("Validate(valid) unexpected error: %v", err)
	}
	invalid := Config{After: []Rule{{On: []string{"unknown"}, Run: "echo check"}}}
	if err := Validate(invalid); err == nil || !strings.Contains(err.Error(), "unknown event") {
		t.Fatalf("Validate(invalid) error = %v, want unknown event", err)
	}
}

// runHookCapture runs one hook command in a scratch repository and returns
// whatever it wrote to hook.out, so each hook environment test is only its
// command and its expectation.
func runHookCapture(t *testing.T, event, command string, phaseID int, status string) string {
	t.Helper()
	root := t.TempDir()
	if err := runOne(root, command, "after "+event+" hook #1", event, "00-checkout-v2", phaseID, status, DefaultTimeout); err != nil {
		t.Fatalf("runOne() unexpected error: %v", err)
	}
	output, err := os.ReadFile(filepath.Join(root, "hook.out"))
	if err != nil {
		t.Fatalf("read hook output: %v", err)
	}
	return string(output)
}

func TestRunHookEnvironment(t *testing.T) {
	command := `printf '%s:%s:%s:%s' "$PLANR_EVENT" "$PLANR_PLAN" "$PLANR_PHASE" "$PLANR_STATUS" > hook.out`
	if got, want := runHookCapture(t, EventDone, command, 2, "done"), "done:00-checkout-v2:2:done"; got != want {
		t.Fatalf("hook output = %q, want %q", got, want)
	}
}

func TestRunHookExportsAgentEnvironment(t *testing.T) {
	// CLAUDE_CODE_CHILD_SESSION is the first marker Detect checks, so it wins
	// over whatever agent environment the test itself is running under.
	t.Setenv("CLAUDE_CODE_CHILD_SESSION", "1")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "session-7")
	command := `printf '%s:%s:%s' "$PLANR_AGENT" "$PLANR_AGENT_SESSION" "$PLANR_AGENT_LEVEL" > hook.out`
	if got, want := runHookCapture(t, EventDone, command, 2, "done"), "claude-code:session-7:direct"; got != want {
		t.Fatalf("hook output = %q, want %q", got, want)
	}
}

func TestRunHookPlanEventHasEmptyPhase(t *testing.T) {
	command := `printf '<%s>' "$PLANR_PHASE" > hook.out`
	if got, want := runHookCapture(t, EventAdd, command, -1, "registered"), "<>"; got != want {
		t.Fatalf("hook output = %q, want %q", got, want)
	}
}

func TestRunConfiguredHooksPreservesRuleOrder(t *testing.T) {
	root := t.TempDir()
	settings := Config{After: []Rule{
		{On: []string{EventAdd, EventDone}, Run: `printf 'one' >> hook.out`},
		{On: []string{EventDone}, Run: `printf 'two' >> hook.out`},
	}}
	if err := Run(root, settings, false, "after", EventDone, "00-checkout-v2", -1, "done"); err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	output, err := os.ReadFile(filepath.Join(root, "hook.out"))
	if err != nil {
		t.Fatalf("read hook output: %v", err)
	}
	if got, want := string(output), "onetwo"; got != want {
		t.Fatalf("hook output = %q, want %q", got, want)
	}
}

func TestRunConfiguredHooksUsesConfiguredTimeout(t *testing.T) {
	root := t.TempDir()
	settings := Config{
		Timeout: 20 * time.Millisecond,
		After:   []Rule{{On: []string{EventDone}, Run: "sleep 1"}},
	}
	started := time.Now()
	err := Run(root, settings, false, "after", EventDone, "00-checkout-v2", -1, "done")
	if err == nil || !strings.Contains(err.Error(), "timed out after 20ms") {
		t.Fatalf("Run() error = %v, want configured timeout", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("configured timeout took too long: %s", elapsed)
	}
}
