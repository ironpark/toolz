package runner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	configuration "github.com/ironpark/toolz/cli/mohae/internal/config"
)

func skipUnlessSandboxed(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("sandbox is macOS-only")
	}
}

func TestSandboxedTrialConfinesItsShellStepsToTheWorkspace(t *testing.T) {
	skipUnlessSandboxed(t)
	directory := t.TempDir()
	config := fixtureConfig(t, directory)
	config.Sandbox = configuration.SandboxConfig{Enabled: true, Scope: configuration.SandboxScopeFull}

	workspace, err := PrepareWorkspace(context.Background(), config, "custom-cli", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()

	environment := trialEnv(config, workspace, workspace.Exec())
	inside := runShellStep(context.Background(), workspace.Exec(),
		"echo ok > $MOHAE_WORKSPACE/wrote.txt", workspace.Root, environment)
	if !inside.Passed {
		t.Fatalf("writing inside the workspace failed: %+v", inside)
	}

	// The escape the sandbox exists to stop: an agent that leaves its work
	// outside the workspace fails verification for the wrong reason.
	escape := filepath.Join(directory, "escaped.txt")
	outside := runShellStep(context.Background(), workspace.Exec(),
		"echo no > "+escape, workspace.Root, environment)
	if outside.Passed {
		t.Error("writing outside the workspace was allowed")
	}
	if _, err := os.Stat(escape); !os.IsNotExist(err) {
		t.Errorf("the file was created outside the workspace: %v", err)
	}
}

func TestSandboxedTrialStillReachesTheHostToolchain(t *testing.T) {
	skipUnlessSandboxed(t)
	directory := t.TempDir()
	config := fixtureConfig(t, directory)
	config.Sandbox = configuration.SandboxConfig{Enabled: true, Scope: configuration.SandboxScopeFull}

	workspace, err := PrepareWorkspace(context.Background(), config, "custom-cli", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()

	// The whole reason to prefer this over a container: no image has to carry
	// the toolchain, because the trial uses the one installed here.
	step := runShellStep(context.Background(), workspace.Exec(), "command -v sh", workspace.Root, nil)
	if !step.Passed {
		t.Errorf("the host toolchain was not reachable: %+v", step)
	}
}

func TestSandboxScopeSetupLeavesTheAgentOnTheHost(t *testing.T) {
	skipUnlessSandboxed(t)
	directory := t.TempDir()
	config := fixtureConfig(t, directory)
	config.Sandbox = configuration.SandboxConfig{Enabled: true, Scope: configuration.SandboxScopeSetup}

	workspace, err := PrepareWorkspace(context.Background(), config, "custom-cli", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()

	// setup confines what mohae runs, not what it measures.
	if !workspace.Exec().Isolated() {
		t.Error("the setup executor is not confined")
	}
	if workspace.Agent().Isolated() {
		t.Error("the agent was confined at scope setup")
	}
	if got := workspace.Sandbox(); got != configuration.SandboxScopeSetup {
		t.Errorf("Sandbox() = %q, want %q", got, configuration.SandboxScopeSetup)
	}
}

func TestSandboxedTrialNeedsNoScriptCopying(t *testing.T) {
	skipUnlessSandboxed(t)
	directory := t.TempDir()
	config := fixtureConfig(t, directory)
	config.Sandbox = configuration.SandboxConfig{Enabled: true, Scope: configuration.SandboxScopeFull}

	workspace, err := PrepareWorkspace(context.Background(), config, "custom-cli", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()

	// Unlike a container, the sandbox shares the host's filesystem, so a script
	// beside the configuration file is already reachable where it lies.
	script := filepath.Join(directory, "setup.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	reachable, err := workspace.reachable(script)
	if err != nil {
		t.Fatal(err)
	}
	if reachable != script {
		t.Errorf("reachable() = %q, want the script left where it is (%q)", reachable, script)
	}
}

func TestSandboxedTrialGetsItsOwnTempDir(t *testing.T) {
	skipUnlessSandboxed(t)
	directory := t.TempDir()
	config := fixtureConfig(t, directory)
	config.Sandbox = configuration.SandboxConfig{Enabled: true, Scope: configuration.SandboxScopeFull}

	workspace, err := PrepareWorkspace(context.Background(), config, "custom-cli", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()

	// The machine's shared temporary directory contains the workspace on macOS,
	// so the trial is pointed at one of its own and can write there.
	step := runShellStep(context.Background(), workspace.Exec(),
		`echo ok > "$TMPDIR/probe" && printf '%s' "$TMPDIR"`, workspace.Root, nil)
	if !step.Passed {
		t.Fatalf("writing to TMPDIR failed: %+v", step)
	}
	if !strings.Contains(step.Output, "tmp") {
		t.Errorf("TMPDIR = %q, want the trial's own", step.Output)
	}
}
