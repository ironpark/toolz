package runner

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	processutil "github.com/ironpark/toolz/cli/mohae/internal/process"
)

// mappedExecutor stands in for a container: it runs everything on the host, so
// the tests need no runtime installed, but it reports a different namespace
// for paths. That is the part worth testing — a trial whose commands ran in
// one place and whose $MOHAE_WORKSPACE named another would fail every check
// for a reason that has nothing to do with the agent.
type mappedExecutor struct {
	base   string
	target string
	dirs   []string
}

func (m *mappedExecutor) Command(ctx context.Context, argv []string, dir string, env map[string]string) *exec.Cmd {
	m.dirs = append(m.dirs, dir)
	// Run on the host from the host's copy of dir, so the step still does
	// something observable.
	return processutil.Host{}.Command(ctx, argv, m.unmap(dir), env)
}

func (m *mappedExecutor) Path(host string) string {
	if !strings.HasPrefix(host, m.base) {
		return host
	}
	return m.target + strings.TrimPrefix(host, m.base)
}

func (m *mappedExecutor) unmap(mapped string) string {
	if !strings.HasPrefix(mapped, m.target) {
		return mapped
	}
	return m.base + strings.TrimPrefix(mapped, m.target)
}

func (m *mappedExecutor) Contained() bool { return true }
func (m *mappedExecutor) Isolated() bool  { return true }

func TestTrialEnvNamesTheWorkspaceAsTheExecutorSeesIt(t *testing.T) {
	workspace := &Workspace{Root: "/host/base/workspace"}
	config := fixtureConfig(t, t.TempDir())

	host := trialEnv(config, workspace, processutil.Host{})
	if host["MOHAE_WORKSPACE"] != "/host/base/workspace" {
		t.Fatalf("host MOHAE_WORKSPACE = %q", host["MOHAE_WORKSPACE"])
	}

	contained := trialEnv(config, workspace, &mappedExecutor{base: "/host/base", target: "/mohae"})
	if contained["MOHAE_WORKSPACE"] != "/mohae/workspace" {
		t.Fatalf("contained MOHAE_WORKSPACE = %q", contained["MOHAE_WORKSPACE"])
	}
}

func TestVerificationRunsInTheScratchDirectoryTheExecutorSees(t *testing.T) {
	// Grading happens outside the workspace so a check cannot leave files
	// behind that would be mistaken for the agent's work. The directory it
	// runs from has to be named in the executor's own namespace, or the shell
	// would start somewhere that does not exist.
	config := fixtureConfig(t, t.TempDir())
	workspace, err := PrepareWorkspace(context.Background(), config, "custom-cli", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()

	executor := &mappedExecutor{base: workspace.base, target: "/mohae"}
	workspace.exec = executor
	config.Verify.Commands = []string{"true"}

	results := runVerifyCommands(context.Background(), config, workspace, TrialOptions{}, io_Discard{})
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("verification did not run: %+v", results)
	}
	if got := executor.dirs[0]; got != "/mohae/scratch" {
		t.Fatalf("verification ran from %q, want the mapped scratch directory", got)
	}
}

// io_Discard is io.Discard as a Writer the options struct accepts without the
// tests importing io for one use.
type io_Discard struct{}

func (io_Discard) Write(p []byte) (int, error) { return len(p), nil }
