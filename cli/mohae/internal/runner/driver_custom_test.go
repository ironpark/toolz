package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/toolz/cli/mohae/internal/agent"
	agentdriver "github.com/ironpark/toolz/cli/mohae/internal/driver"

	processutil "github.com/ironpark/toolz/cli/mohae/internal/process"
)

// stubAgent writes an executable standing in for an agent CLI. Every driver
// test uses one: a test that needed a real claude or codex binary would only
// pass on a machine that happened to have it, and would spend tokens to do it.
func stubAgent(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stub-agent")
	writeFile(t, path, "#!/bin/sh\n"+script, 0o755)
	return path
}

// openCustomDriver prepares a workspace and opens the custom-cli driver on it.
func openCustomDriver(t *testing.T, command []string, onText func(string)) (agentdriver.Driver, *Workspace) {
	t.Helper()
	directory := t.TempDir()
	config := fixtureConfig(t, directory)
	config.Agent.Command = command
	workspace, err := PrepareWorkspace(context.Background(), config, config.Agent.Type)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { workspace.Cleanup() })
	driver, err := agentdriver.New(context.Background(), newDriverOptions(config, workspace, nil, onText, "test"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { driver.Close() })
	return driver, workspace
}

func TestCustomDriverSendsThePromptOnStdinAndReturnsTheReply(t *testing.T) {
	agent := stubAgent(t, "echo \"replying to: $(cat)\"\n")
	driver, _ := openCustomDriver(t, []string{agent}, nil)

	response, err := driver.Send(context.Background(), "build it")
	if err != nil {
		t.Fatal(err)
	}
	if response.Text != "replying to: build it" {
		t.Errorf("text = %q", response.Text)
	}
}

func TestCustomDriverSubstitutesThePromptPlaceholder(t *testing.T) {
	// An agent CLI that takes its prompt as an argument must not also be handed
	// it on stdin, or it would receive the turn twice.
	agent := stubAgent(t, "echo \"argument: $1\"\necho \"stdin: [$(cat)]\"\n")
	driver, _ := openCustomDriver(t, []string{agent, agentdriver.PromptPlaceholder}, nil)

	response, err := driver.Send(context.Background(), "build it")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(response.Text, "argument: build it") {
		t.Errorf("the placeholder was not substituted: %q", response.Text)
	}
	if !strings.Contains(response.Text, "stdin: []") {
		t.Errorf("the prompt was delivered twice: %q", response.Text)
	}
}

func TestCustomDriverRunsInTheWorkspaceWithTheTrialEnvironment(t *testing.T) {
	agent := stubAgent(t, "pwd\necho \"$MOHAE_MODEL/$MOHAE_EFFORT/$CUSTOM\"\ntouch made-by-agent\n")
	directory := t.TempDir()
	config := fixtureConfig(t, directory)
	config.Agent.Command = []string{agent}
	config.Agent.Model = "stub-1"
	config.Agent.Effort = "high"
	config.Agent.Env = map[string]string{"CUSTOM": "yes"}
	workspace, err := PrepareWorkspace(context.Background(), config, config.Agent.Type)
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()
	driver, err := agentdriver.New(context.Background(), newDriverOptions(config, workspace, nil, nil, "test"))
	if err != nil {
		t.Fatal(err)
	}
	defer driver.Close()

	response, err := driver.Send(context.Background(), "go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(response.Text, "stub-1/high/yes") {
		t.Errorf("the trial environment did not reach the agent: %q", response.Text)
	}
	// The agent works in the copy, never anywhere else.
	if _, err := os.Stat(filepath.Join(workspace.Root, "made-by-agent")); err != nil {
		t.Errorf("the agent did not run in the workspace: %v", err)
	}
}

func TestCustomDriverStreamsTheReplyAsItArrives(t *testing.T) {
	// --show-dialogue is only worth having if the text appears while the agent
	// is still working, so the callback has to fire before Send returns.
	agent := stubAgent(t, "echo first\nsleep 0.2\necho second\n")
	streamed := make(chan string, 4)
	driver, _ := openCustomDriver(t, []string{agent}, func(text string) { streamed <- text })

	done := make(chan struct{})
	go func() {
		defer close(done)
		driver.Send(context.Background(), "go")
	}()
	select {
	case text := <-streamed:
		if strings.TrimSpace(text) != "first" {
			t.Errorf("streamed %q first", text)
		}
	case <-done:
		t.Fatal("the whole turn finished before anything was streamed")
	case <-time.After(5 * time.Second):
		t.Fatal("nothing was streamed")
	}
	<-done
}

func TestCustomDriverReportsAFailedCommandWithItsStderr(t *testing.T) {
	agent := stubAgent(t, "echo partial\necho 'the model refused' >&2\nexit 2\n")
	driver, _ := openCustomDriver(t, []string{agent}, nil)

	response, err := driver.Send(context.Background(), "go")
	if err == nil {
		t.Fatal("expected a failing agent command to be an error")
	}
	if !strings.Contains(err.Error(), "the model refused") {
		t.Errorf("err = %v, want it to carry the agent's stderr", err)
	}
	// Whatever the agent did say is still worth reporting.
	if response.Text != "partial" {
		t.Errorf("text = %q, want the output produced before the failure", response.Text)
	}
}

func TestCustomDriverStopsTheAgentWhenTheTurnRunsOut(t *testing.T) {
	agent := stubAgent(t, "sleep 30\n")
	driver, _ := openCustomDriver(t, []string{agent}, nil)

	prompt := Prompt{Text: "go", TimeoutSeconds: 1}
	ctx, cancel := prompt.TurnContext(context.Background())
	defer cancel()

	started := time.Now()
	if _, err := driver.Send(ctx, "go"); err == nil {
		t.Fatal("expected the turn to end in an error when its timeout fired")
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		// A driver that only stopped waiting, without killing the process,
		// would leave an agent running for the rest of the run.
		t.Errorf("the turn took %s; the agent was not stopped", elapsed)
	}
}

func TestNewDriverRejectsAnAgentTypeItCannotDrive(t *testing.T) {
	directory := t.TempDir()
	config := fixtureConfig(t, directory)
	config.Agent.Type = "nonesuch"
	workspace, err := PrepareWorkspace(context.Background(), config, "custom-cli")
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()
	if _, err := agentdriver.New(context.Background(), newDriverOptions(config, workspace, nil, nil, "test")); err == nil {
		t.Fatal("expected an unknown agent type to be refused")
	}
}

// TestDriverEnvIsResolvedForEveryAgentType guards the contract that made
// claude-code the odd one out once: the trial's variables are resolved in
// newDriverOptions, so a driver cannot quietly open a session without them and
// leave two agents reading a different environment for the same configuration.
func TestDriverEnvIsResolvedForEveryAgentType(t *testing.T) {
	directory := t.TempDir()
	for _, agentType := range agent.KnownTypes {
		config := fixtureConfig(t, directory)
		config.Name = "env-trial"
		config.Agent.Type = agentType
		config.Agent.Model = "a-model"
		config.Agent.Effort = "high"
		config.Agent.Env = map[string]string{"EXTRA": "1"}
		workspace := &Workspace{Root: filepath.Join(directory, agentType)}

		env := trialEnv(config, workspace, workspace.Exec())
		for key, want := range map[string]string{
			"MOHAE_WORKSPACE": workspace.Root,
			"MOHAE_TRIAL":     "env-trial",
			"MOHAE_MODEL":     "a-model",
			"MOHAE_EFFORT":    "high",
			"EXTRA":           "1",
		} {
			if env[key] != want {
				t.Errorf("%s: env[%s] = %q, want %q", agentType, key, env[key], want)
			}
		}
	}
}

// TestDriverEnvLetsTheConfigurationWin keeps agent.env the last word: a
// configuration that deliberately overrides a MOHAE_ variable must not have
// mohae's own value put back on top of it.
func TestDriverEnvLetsTheConfigurationWin(t *testing.T) {
	config := fixtureConfig(t, t.TempDir())
	config.Agent.Model = "configured"
	config.Agent.Env = map[string]string{"MOHAE_MODEL": "overridden"}
	env := trialEnv(config, &Workspace{Root: "/tmp/ws"}, processutil.Host{})
	if got := env["MOHAE_MODEL"]; got != "overridden" {
		t.Fatalf("MOHAE_MODEL = %q, want %q", got, "overridden")
	}
	found := false
	for _, entry := range processutil.Env(env) {
		if entry == "MOHAE_MODEL=overridden" {
			found = true
		}
	}
	if !found {
		t.Error("environ() did not carry the configured override")
	}
}
