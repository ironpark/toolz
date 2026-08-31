package runner

import (
	"context"
	"fmt"
	"io"
	"maps"
)

// runAfterHooks finalizes the workspace after the agent session and before it
// is verified. A hook may run inside the workspace to finalize its state, or
// outside it when it only needs to inspect the result without changing it.
func runAfterHooks(ctx context.Context, config *Config, workspace *Workspace, options TrialOptions, out io.Writer) []HookResult {
	if len(config.Hooks.After) == 0 {
		return nil
	}
	ctx, cancel := config.Limits.Bound(context.WithoutCancel(ctx))
	defer cancel()
	base := trialEnv(config, workspace, workspace.Exec())
	results := make([]HookResult, 0, len(config.Hooks.After))
	for _, hook := range config.Hooks.After {
		directory, location := hookDirectory(hook, workspace)
		directory = workspace.Exec().Path(directory)
		// Cmd.Dir changes the process directory but an explicitly supplied
		// environment otherwise retains the parent's PWD. Copied so one hook's
		// directory does not leak into the next.
		env := maps.Clone(base)
		env["PWD"] = directory
		step := runShellStep(ctx, workspace.Exec(), hook.Run, directory, env)
		if options.ShowDialogue {
			fmt.Fprintf(out, "hook after %s (%s): %s\n", VerdictWord(step.Passed), location, hook.Run)
		}
		results = append(results, HookResult{
			Command:         hook.Run,
			Scope:           location,
			ExitCode:        step.ExitCode,
			Passed:          step.Passed,
			Output:          step.Output,
			DurationSeconds: step.Duration,
		})
	}
	return results
}

func hookDirectory(hook HookCommand, workspace *Workspace) (string, string) {
	if hook.Scope == HookScopeOutside {
		return workspace.Scratch, HookScopeOutside
	}
	return workspace.Root, HookScopeWorkspace
}
