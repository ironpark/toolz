package runner

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	processutil "github.com/ironpark/toolz/cli/mohae/internal/process"
)

// shellStep is what one `sh -c` command run on a trial's behalf produced. Hooks
// and verification commands both grade on the exit status and keep the output
// verbatim, so they share one runner: a change to how a step is isolated or how
// its exit code is read applies to both by construction.
type shellStep struct {
	ExitCode int
	Passed   bool
	Output   string
	Duration float64
}

// runShellStep runs text with stdout and stderr merged, wherever the trial's
// executor puts it. dir and any path in env are already in the executor's
// namespace: a container's workspace path is not the host's, and the mapping
// belongs to the caller that knows which directory it means.
func runShellStep(ctx context.Context, executor processutil.Executor, text, dir string, env map[string]string) shellStep {
	started := time.Now()
	command := processutil.Shell(ctx, executor, text, dir, env)
	output := &bytes.Buffer{}
	command.Stdout = output
	command.Stderr = output
	err := command.Run()

	step := shellStep{
		Passed:   err == nil,
		Output:   strings.TrimSpace(output.String()),
		Duration: time.Since(started).Seconds(),
	}
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitErr):
		step.ExitCode = exitErr.ExitCode()
	default:
		// A command that could not be started at all: the shell's own failure
		// is the output, since there is none of its own.
		step.ExitCode = -1
		step.Output = strings.TrimSpace(step.Output + "\n" + err.Error())
	}
	return step
}

func verdictWord(passed bool) string {
	if passed {
		return "pass"
	}
	return "fail"
}
