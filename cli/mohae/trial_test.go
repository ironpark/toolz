package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// trialConfig builds a runnable configuration whose agent is a shell stub, so a
// whole trial can be exercised without any real agent or network.
func trialConfig(t *testing.T, script string) *Config {
	t.Helper()
	directory := t.TempDir()
	config := fixtureConfig(t, directory)
	config.Agent.Command = []string{stubAgent(t, script)}
	return config
}

func TestRunTrialPassesWhenEveryVerifyCommandPasses(t *testing.T) {
	// The stub writes the file the grading command looks for, which is the
	// whole shape of a trial: the agent changes the workspace, and the
	// verification judges the workspace rather than what the agent said.
	config := trialConfig(t, "echo done\ntouch delivered.txt\n")
	config.Verify.Commands = []string{`test -f "$MOHAE_WORKSPACE/delivered.txt"`}

	result := RunTrial(context.Background(), config, TrialOptions{})
	if !result.Passed {
		t.Fatalf("result = %+v, want a pass", result)
	}
	if len(result.Turns) != 1 || result.Turns[0].Response != "done" {
		t.Errorf("turns = %+v", result.Turns)
	}
	if result.VerifyPassed() != 1 {
		t.Errorf("verify = %+v", result.Verify)
	}
	// A passing trial's workspace is disposable, and leaving them behind would
	// fill the machine over a long benchmark run.
	if result.Workspace != "" {
		t.Errorf("workspace %q was kept after a pass", result.Workspace)
	}
}

func TestRunTrialFailsAndKeepsTheWorkspaceWhenVerificationFails(t *testing.T) {
	config := trialConfig(t, "echo i did nothing\n")
	config.Verify.Commands = []string{
		`test -f "$MOHAE_WORKSPACE/delivered.txt"`,
		"echo second check ran",
	}

	result := RunTrial(context.Background(), config, TrialOptions{})
	if result.Passed {
		t.Fatal("a failed verification must fail the trial")
	}
	// Every command runs: stopping at the first failure would hide how much of
	// the task was done.
	if len(result.Verify) != 2 || !result.Verify[1].Passed {
		t.Fatalf("verify = %+v, want both commands to have run", result.Verify)
	}
	if result.Verify[0].ExitCode == 0 {
		t.Errorf("the failing command reported exit code %d", result.Verify[0].ExitCode)
	}
	if result.Verify[1].Output != "second check ran" {
		t.Errorf("output = %q, want it recorded verbatim", result.Verify[1].Output)
	}
	// The workspace is the only record of what the agent actually did.
	if result.Workspace == "" {
		t.Fatal("a failed trial's workspace was not kept")
	}
	defer os.RemoveAll(filepath.Dir(result.Workspace))
	if _, err := os.Stat(result.Workspace); err != nil {
		t.Errorf("the kept workspace is gone: %v", err)
	}
}

func TestRunTrialGradesFromOutsideTheWorkspace(t *testing.T) {
	config := trialConfig(t, "echo done\n")
	config.Verify.Commands = []string{"touch grading-artifact"}

	result := RunTrial(context.Background(), config, TrialOptions{KeepWorkspace: true})
	defer os.RemoveAll(filepath.Dir(result.Workspace))
	// A check that ran inside the workspace would leave files that later
	// commands — or a human reading the result — would read as the agent's work.
	if _, err := os.Stat(filepath.Join(result.Workspace, "grading-artifact")); err == nil {
		t.Error("a verify command wrote into the workspace")
	}
}

func TestRunTrialSkipsAPromptWhoseConditionIsFalse(t *testing.T) {
	config := trialConfig(t, "echo turn\n")
	config.Prompts = []Prompt{
		{Text: "first"},
		{Text: "only when the build is broken", When: `sh("exit 1") == 0`},
		{Text: "third"},
	}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}

	result := RunTrial(context.Background(), config, TrialOptions{})
	if len(result.Turns) != 3 {
		t.Fatalf("turns = %+v, want every prompt recorded", result.Turns)
	}
	// The skipped prompt is still recorded: a conversation that silently shrank
	// would make two different runs look identical.
	if result.Turns[1].Sent || result.Turns[1].Skipped == "" {
		t.Errorf("turn 2 = %+v, want it skipped with a reason", result.Turns[1])
	}
	if !result.Turns[0].Sent || !result.Turns[2].Sent {
		t.Errorf("the unconditional prompts did not run: %+v", result.Turns)
	}
}

func TestRunTrialSkipsAPromptWhoseDependencyNeverRan(t *testing.T) {
	config := trialConfig(t, "echo turn\n")
	config.Prompts = []Prompt{
		{Text: "first"},
		{Name: "fix-build", Text: "fix it", When: `sh("exit 1") == 0`},
		{Text: "add a regression test", After: []string{"fix-build"}},
	}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}

	result := RunTrial(context.Background(), config, TrialOptions{})
	// A follow-up to a turn that never happened would arrive without its
	// context and measure the agent on a question nobody asked.
	if result.Turns[2].Sent {
		t.Errorf("the dependent prompt was sent without its dependency: %+v", result.Turns[2])
	}
	if !strings.Contains(result.Turns[2].Skipped, "fix-build") {
		t.Errorf("skip reason = %q, want it to name the dependency", result.Turns[2].Skipped)
	}
}

func TestRunTrialSeesThePreviousResponseInAConditionAndAggregatesUsage(t *testing.T) {
	config := trialConfig(t, "echo the build is broken\n")
	config.Prompts = []Prompt{
		{Text: "first"},
		{Text: "fix it", When: `previous contains "broken"`},
		{Text: "never", When: `previous contains "green"`},
	}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}

	result := RunTrial(context.Background(), config, TrialOptions{})
	if !result.Turns[1].Sent {
		t.Errorf("a condition on the previous response did not fire: %+v", result.Turns[1])
	}
	if result.Turns[2].Sent {
		t.Errorf("a condition that does not hold sent its prompt: %+v", result.Turns[2])
	}
	if result.Sent() != 2 {
		t.Errorf("sent = %d, want 2", result.Sent())
	}
}

func TestRunTrialStopsTheConversationWhenATurnFails(t *testing.T) {
	config := trialConfig(t, "echo trying\nexit 4\n")
	config.Prompts = []Prompt{{Text: "first"}, {Text: "second"}}
	config.Verify.Commands = []string{"true"}

	result := RunTrial(context.Background(), config, TrialOptions{KeepWorkspace: true})
	defer os.RemoveAll(filepath.Dir(result.Workspace))
	if result.Passed || result.Error == "" {
		t.Fatalf("result = %+v, want a recorded failure", result)
	}
	if result.Turns[1].Sent {
		// A follow-up written for a turn that failed would be answering a
		// question that was never asked.
		t.Error("the conversation continued after a failed turn")
	}
	// The workspace is still worth grading: the agent may have finished the
	// task before it fell over.
	if len(result.Verify) != 1 {
		t.Errorf("verification did not run after the failure: %+v", result.Verify)
	}
}

func TestRunTrialReportsItsOwnTimeoutAndStillGrades(t *testing.T) {
	config := trialConfig(t, "sleep 30\n")
	config.Limits.TimeoutSeconds = 1
	config.Verify.Commands = []string{"true"}

	result := RunTrial(context.Background(), config, TrialOptions{KeepWorkspace: true})
	defer os.RemoveAll(filepath.Dir(result.Workspace))
	if !result.TimedOut {
		t.Fatalf("result = %+v, want the timeout recorded", result)
	}
	if result.Passed {
		t.Error("a trial that ran out of time must not pass")
	}
	if result.Verdict() != "timeout" {
		t.Errorf("verdict = %q", result.Verdict())
	}
	// Grading is detached from the trial's expired clock, so a workspace that
	// was finished before the agent hung is still gradable.
	if len(result.Verify) != 1 || !result.Verify[0].Passed {
		t.Errorf("verify = %+v, want it to have run despite the timeout", result.Verify)
	}
	if result.DurationSeconds > 20 {
		t.Errorf("the trial took %.1fs; the limit did not stop it", result.DurationSeconds)
	}
}

func TestRunTrialHonoursAPerTurnTimeout(t *testing.T) {
	config := trialConfig(t, "sleep 30\n")
	config.Prompts = []Prompt{{Text: "slow", TimeoutSeconds: 1}}
	config.Limits.TimeoutSeconds = 60

	result := RunTrial(context.Background(), config, TrialOptions{KeepWorkspace: true})
	defer os.RemoveAll(filepath.Dir(result.Workspace))
	if result.Turns[0].Error == "" {
		t.Fatalf("turn = %+v, want the cancelled turn recorded", result.Turns[0])
	}
	// The turn's own limit fired, not the trial's.
	if result.TimedOut {
		t.Error("a per-turn timeout was reported as a trial timeout")
	}
	if result.DurationSeconds > 20 {
		t.Errorf("the trial took %.1fs; the turn's limit did not stop it", result.DurationSeconds)
	}
}

func TestRunTrialSendsAPromptFileAsItsTurn(t *testing.T) {
	config := trialConfig(t, "echo \"got: $(cat)\"\n")
	writeFile(t, filepath.Join(filepath.Dir(config.Path), "PROMPT.md"), "from the file\n", 0o644)
	config.Prompts = []Prompt{{File: "./PROMPT.md"}}

	result := RunTrial(context.Background(), config, TrialOptions{})
	if !strings.Contains(result.Turns[0].Response, "got: from the file") {
		t.Errorf("response = %q", result.Turns[0].Response)
	}
}

func TestRunTrialStreamsTheDialogueWhenAsked(t *testing.T) {
	config := trialConfig(t, "echo working on it\n")
	config.Verify.Commands = []string{"true"}
	out := &bytes.Buffer{}

	RunTrial(context.Background(), config, TrialOptions{ShowDialogue: true, Out: out})
	text := out.String()
	// Both halves of the conversation, so the transcript on screen reads as one.
	for _, want := range []string{"do the thing", "working on it", "verify pass"} {
		if !strings.Contains(text, want) {
			t.Errorf("dialogue = %q, want it to contain %q", text, want)
		}
	}
}

func TestRunTrialReportsASetupFailureWithoutRunningTheAgent(t *testing.T) {
	config := trialConfig(t, "echo should not run\ntouch agent-ran\n")
	writeFile(t, filepath.Join(filepath.Dir(config.Path), "init.sh"), "#!/bin/sh\nexit 1\n", 0o755)
	config.Workspace.InitScript = "./init.sh"

	result := RunTrial(context.Background(), config, TrialOptions{})
	if result.Passed || result.Error == "" {
		t.Fatalf("result = %+v, want the setup failure reported", result)
	}
	if len(result.Turns) != 0 {
		// Sending prompts into a workspace that was never set up would spend
		// tokens measuring the wrong thing.
		t.Errorf("turns = %+v, want none", result.Turns)
	}
}
