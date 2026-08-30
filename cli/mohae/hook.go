package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	processutil "github.com/ironpark/toolz/cli/mohae/internal/process"
)

// runAfterHooks finalizes the workspace after the agent session and before it
// is verified. A hook may run inside the workspace to finalize its state, or
// outside it when it only needs to inspect the result without changing it.
func runAfterHooks(ctx context.Context, config *Config, workspace *Workspace, options TrialOptions, out io.Writer) []HookResult {
	if len(config.Hooks.After) == 0 {
		return nil
	}
	ctx, cancel := config.Limits.bound(context.WithoutCancel(ctx))
	defer cancel()
	results := make([]HookResult, 0, len(config.Hooks.After))
	for _, hook := range config.Hooks.After {
		directory, location := hook.directory(workspace)
		env := processEnv(trialEnv(config, workspace))
		// Cmd.Dir changes the process directory but an explicitly supplied
		// environment otherwise retains the parent's PWD.
		env = append(env, "PWD="+directory)
		started := time.Now()
		command := exec.CommandContext(ctx, "sh", "-c", hook.Run)
		command.Dir = directory
		command.Env = env
		processutil.Isolate(command)
		output := &bytes.Buffer{}
		command.Stdout = output
		command.Stderr = output
		err := command.Run()

		result := HookResult{
			Command:         hook.Run,
			Scope:           location,
			Passed:          err == nil,
			Output:          strings.TrimSpace(output.String()),
			DurationSeconds: time.Since(started).Seconds(),
		}
		var exitErr *exec.ExitError
		switch {
		case err == nil:
		case errors.As(err, &exitErr):
			result.ExitCode = exitErr.ExitCode()
		default:
			result.ExitCode = -1
			result.Output = strings.TrimSpace(result.Output + "\n" + err.Error())
		}
		if options.ShowDialogue {
			fmt.Fprintf(out, "hook after %s (%s): %s\n", verdictWord(result.Passed), location, hook.Run)
		}
		results = append(results, result)
	}
	return results
}
