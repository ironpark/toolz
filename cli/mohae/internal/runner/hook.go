package runner

import (
	"context"
	"fmt"
	"io"
	"slices"

	processutil "github.com/ironpark/toolz/cli/mohae/internal/process"
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
	base := processutil.Env(trialEnv(config, workspace))
	results := make([]HookResult, 0, len(config.Hooks.After))
	for _, hook := range config.Hooks.After {
		directory, location := hookDirectory(hook, workspace)
		// Cmd.Dir changes the process directory but an explicitly supplied
		// environment otherwise retains the parent's PWD. Clipped so the hooks
		// do not append into a shared backing array.
		env := append(slices.Clip(base), "PWD="+directory)
		step := runShellStep(ctx, hook.Run, directory, env)
		if options.ShowDialogue {
			fmt.Fprintf(out, "hook after %s (%s): %s\n", verdictWord(step.Passed), location, hook.Run)
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
