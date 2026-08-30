package app

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

// runShellStep runs text with stdout and stderr merged, in its own process
// group so cancelling the context reaches whatever the command started.
func runShellStep(ctx context.Context, text, dir string, env []string) shellStep {
	started := time.Now()
	command := exec.CommandContext(ctx, "sh", "-c", text)
	command.Dir = dir
	command.Env = env
	processutil.Isolate(command)
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
