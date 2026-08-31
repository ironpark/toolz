package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	configuration "github.com/ironpark/toolz/cli/mohae/internal/config"
	"github.com/ironpark/toolz/cli/mohae/internal/container"
)

// containerImage is small, has a shell, and is what a trial's setup and
// grading commands need and no more.
const containerImage = "docker.io/library/alpine:3.20"

// requireRuntime skips when there is nothing to run against. The tests below
// are the only ones that prove the container path works end to end, so they
// are worth having even though they cannot run everywhere.
func requireRuntime(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("container trials pull an image")
	}
	if _, err := container.Detect(""); err != nil {
		t.Skip("no container runtime: " + err.Error())
	}
}

func containerFixture(t *testing.T) *Config {
	t.Helper()
	config := fixtureConfig(t, t.TempDir())
	// Spelled out rather than defaulted: these tests build a Config in code,
	// which does not go through loading.
	config.Container = ContainerConfig{
		Image:   containerImage,
		Runtime: container.Auto,
		Scope:   configuration.ContainerScopeSetup,
		User:    container.UserHost,
	}
	return config
}

func TestContainerTrialRunsSetupAndGradingInside(t *testing.T) {
	requireRuntime(t)
	config := containerFixture(t)
	directory := filepath.Dir(config.Path)
	// The script lives beside the configuration, where the container cannot
	// see it, so this also covers the copy that makes it reachable.
	writeFile(t, filepath.Join(directory, "init.sh"), "#!/bin/sh\nuname -s > kernel\n", 0o755)
	config.Workspace.InitScript = "./init.sh"

	workspace, err := PrepareWorkspace(context.Background(), config, "custom-cli")
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()

	// The setup script ran in the image, not on this machine.
	kernel, err := os.ReadFile(filepath.Join(workspace.Root, "kernel"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(kernel)); got != "Linux" {
		t.Fatalf("workspace.init_script ran on %q, not in the container", got)
	}

	// And it wrote through the bind mount as the user who started the test,
	// which is what lets the workspace be read and then deleted afterwards.
	if !workspace.Exec().Contained() {
		t.Fatal("the trial's executor is not the container")
	}

	config.Verify.Commands = []string{
		// $MOHAE_WORKSPACE has to name the workspace as the command inside
		// sees it, not as the host does.
		"test -f \"$MOHAE_WORKSPACE/kernel\"",
		"test \"$MOHAE_WORKSPACE\" = " + container.MountPoint + "/workspace",
	}
	for _, result := range runVerifyCommands(context.Background(), config, workspace, TrialOptions{}, io_Discard{}) {
		if !result.Passed {
			t.Errorf("verify %q failed (%d): %s", result.Command, result.ExitCode, result.Output)
		}
	}
}

func TestContainerTrialIsRemovedWithTheWorkspace(t *testing.T) {
	requireRuntime(t)
	config := containerFixture(t)
	workspace, err := PrepareWorkspace(context.Background(), config, "custom-cli")
	if err != nil {
		t.Fatal(err)
	}
	base := workspace.base
	if err := workspace.Cleanup(); err != nil {
		t.Fatal(err)
	}
	// A container left running would hold a temporary directory open for as
	// long as the machine lives, and a comparison run starts one per trial.
	if _, err := os.Stat(base); !os.IsNotExist(err) {
		t.Fatalf("the trial directory survived cleanup: %v", err)
	}
	if workspace.Exec().Contained() {
		t.Fatal("the workspace still reports a container after cleanup")
	}
}

func TestContainerTrialFailsWhenTheRuntimeIsMissing(t *testing.T) {
	// Falling back to the host would report an unsandboxed trial as a
	// sandboxed one, which is worse than not running it at all.
	config := containerFixture(t)
	config.Container.Runtime = "containerd"
	if _, err := PrepareWorkspace(context.Background(), config, "custom-cli"); err == nil {
		t.Fatal("a trial ran without the runtime it asked for")
	}
}

func TestFullScopeRunsTheAgentInsideToo(t *testing.T) {
	requireRuntime(t)
	// setup and full differ in exactly one thing, and it is the thing that
	// decides whether the agent is bounded by the container or only starts
	// next to it.
	config := containerFixture(t)
	config.Container.Scope = configuration.ContainerScopeFull
	config.Agent.Command = []string{"sh", "-c", "uname -s > $MOHAE_WORKSPACE/agent-kernel; echo done"}

	result := RunTrial(context.Background(), config, TrialOptions{KeepWorkspace: true})
	if result.Error != "" {
		t.Fatalf("trial failed: %s", result.Error)
	}
	if result.Container != containerImage {
		t.Errorf("result.container = %q, want %q", result.Container, containerImage)
	}
	kernel, err := os.ReadFile(filepath.Join(result.Workspace, "agent-kernel"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(kernel)); got != "Linux" {
		t.Fatalf("the agent ran on %q, not in the container", got)
	}
	// The workspace was deliberately kept; the container must not have been.
	// Its directory is gone from under it once this removes it, so a leak
	// would be invisible until the machine ran out of memory.
	if running := containersFor(t, filepath.Dir(result.Workspace)); running != 0 {
		t.Errorf("%d container(s) survived a trial that kept its workspace", running)
	}
	os.RemoveAll(filepath.Dir(result.Workspace))
}

func TestSetupScopeLeavesTheAgentOnTheHost(t *testing.T) {
	requireRuntime(t)
	config := containerFixture(t)
	config.Agent.Command = []string{"sh", "-c", "uname -s"}

	workspace, err := PrepareWorkspace(context.Background(), config, "custom-cli")
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()

	if workspace.Agent().Contained() {
		t.Fatal("scope setup put the agent in the container")
	}
	// The workspace the agent is handed is a host path, because that is where
	// it runs; the verify commands get the container's path for the same
	// directory.
	if got := workspace.Agent().Path(workspace.Root); got != workspace.Root {
		t.Fatalf("the agent was handed %q, want the host path", got)
	}
}

// containersFor counts the containers still labelled with a trial directory.
func containersFor(t *testing.T, base string) int {
	t.Helper()
	runtime, err := container.Detect("")
	if err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(runtime.Path, "ps", "--all", "--quiet",
		"--filter", "label="+container.BaseLabel+"="+base).Output()
	if err != nil {
		t.Fatal(err)
	}
	return len(strings.Fields(string(output)))
}

func TestCancellingAStepKillsWhatItStartedInside(t *testing.T) {
	requireRuntime(t)
	// Cancelling a step has to reach the work it started, not only the shell
	// that started it: a turn that timed out while the agent was still writing
	// would otherwise leave that write racing the verification of the same
	// files.
	config := containerFixture(t)
	workspace, err := PrepareWorkspace(context.Background(), config, "custom-cli")
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	// A background child outliving its parent shell is the case a plain
	// process kill misses.
	step := runShellStep(ctx, workspace.Exec(),
		"sleep 60 & sleep 60", workspace.Exec().Path(workspace.Root), nil)
	if step.Passed {
		t.Fatal("a cancelled step reported success")
	}

	survivors := workspace.Exec().Command(context.Background(),
		[]string{"sh", "-c", "ps -o args= | grep -c '[s]leep 60' || true"}, "/", nil)
	output, err := survivors.Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(output)); got != "0" {
		t.Fatalf("%s sleep process(es) survived the cancellation", got)
	}
}
