package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	configuration "github.com/ironpark/toolz/cli/mohae/internal/config"
	agentdriver "github.com/ironpark/toolz/cli/mohae/internal/driver"
	skillsrc "github.com/ironpark/toolz/cli/mohae/internal/skill"
)

// TrialOptions are the run-time choices a trial takes from the command line
// rather than from the configuration.
type TrialOptions struct {
	// Version identifies mohae to agent integrations and MCP servers.
	Version string
	// ShowDialogue streams the conversation to Out while it runs.
	ShowDialogue bool
	// Out receives the streamed dialogue. A nil Out means os.Stdout.
	Out io.Writer
	// KeepWorkspace leaves the trial's directory behind even when it passed.
	// A failed trial's workspace is always kept: it is the only record of what
	// the agent actually did.
	KeepWorkspace bool
	// Skills fetches the configuration's remote skills. One resolver is shared
	// by every trial in a run so a source is downloaded once rather than once
	// per trial. Nil is a resolver with default settings.
	Skills *skillsrc.Resolver
}

// RunTrial runs one configuration end to end and returns what happened. It does
// not return an error: a failed trial is a result, not a malfunction, and the
// caller decides what a failure means for the exit status.
//
// The order is fixed and is the whole design: prepare an isolated workspace,
// open the agent on it, send the conversation, then grade the workspace from
// outside it. Nothing after the agent stops can change what the agent did.
// The result is a named return so the deferred cleanup can record the trial's
// duration and the workspace it left behind.
func RunTrial(ctx context.Context, config *Config, options TrialOptions) (result TrialResult) {
	started := time.Now()
	result = TrialResult{
		Name:        config.Name,
		Description: config.Description,
		ConfigPath:  config.Path,
		Agent:       config.Agent.Type,
		Model:       config.Agent.Model,
		StartedAt:   started,
	}
	defer func() { result.DurationSeconds = time.Since(started).Seconds() }()

	// The trial-wide limit starts here, before the workspace is even copied: a
	// setup script that never finishes costs the same wall time as an agent
	// that never stops.
	ctx, cancel := config.Limits.Bound(ctx)
	defer cancel()

	workspace, err := PrepareWorkspace(ctx, config, config.Agent.Type, options.Skills)
	if err != nil {
		// Nothing ran, so there is nothing to grade and no workspace to keep.
		result.Error = err.Error()
		result.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		return result
	}
	result.Container = workspace.Container()
	result.Sandbox = workspace.Sandbox()
	// Recorded before anything can fail: which revision of a fetched skill the
	// agent was given is part of what the run measured, whatever the verdict.
	result.Skills = workspace.Skills
	// Unconditional, and before the cleanup below: the workspace may be kept,
	// the container never is. It has nothing left to run once the trial ends,
	// and one left behind per failing trial would accumulate for as long as
	// the machine lives.
	defer func() { _ = workspace.Close() }()
	defer func() {
		// Asked of the verdict rather than re-derived here, so what counts as
		// a disposable workspace and what reads as "pass" cannot drift apart.
		// An ungraded trial is kept: the workspace is all it produced.
		if result.Verdict() == "pass" && !options.KeepWorkspace {
			workspace.Cleanup()
			return
		}
		result.Workspace = workspace.Root
	}()

	servers, err := LoadMCPServers(config, config.Agent.Type)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if len(servers) > 0 {
		// Probed before the agent starts, so a server that never came up is
		// reported as a broken setup rather than read as a failed task.
		result.MCP = ProbeMCPServers(ctx, servers, options.Version)
	}

	out := options.Out
	if out == nil {
		out = os.Stdout
	}
	var onText func(string)
	if options.ShowDialogue {
		onText = func(text string) { fmt.Fprint(out, text) }
	}
	driver, err := agentdriver.New(ctx, newDriverOptions(config, workspace, servers, onText, options.Version))
	if err != nil {
		result.Error = err.Error()
		result.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		return result
	}

	result.Turns, result.Usage, err = runConversation(ctx, config, workspace, driver, options, out, started)
	// End the agent session before exposing the workspace to completion hooks.
	// In particular, a persistent driver must not still be able to write while
	// a hook is finalizing the same files.
	_ = driver.Close()
	if err != nil {
		result.Error = err.Error()
	}
	result.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)

	// Completion hooks run even after a failed or timed-out conversation: the
	// workspace may still need to be finalized before it can be graded. They
	// get a fresh bounded context for the same reason verification does.
	result.Hooks = runAfterHooks(ctx, config, workspace, options, out)

	// Verification runs even after a failed hook or conversation: the
	// workspace is still there to be graded, and "the agent stopped early but
	// the task was done" is a result worth telling apart from "it was not".
	result.Verify = runVerifyCommands(ctx, config, workspace, options, out)
	result.ArtifactDir, result.Artifacts, err = captureArtifacts(config, workspace, started)
	if err != nil {
		result.ArtifactError = err.Error()
	}
	result.Passed = result.Error == "" && result.ArtifactError == "" && !result.TimedOut &&
		result.HooksPassed() == len(result.Hooks) && result.VerifyPassed() == len(result.Verify)
	return result
}

// runConversation walks the prompts in order. A prompt is skipped when the
// prompts it comes after were never sent, or when its own condition is false;
// both are recorded, because a conversation that silently shrank would make two
// different runs look identical.
func runConversation(ctx context.Context, config *Config, workspace *Workspace, driver agentdriver.Driver, options TrialOptions, out io.Writer, started time.Time) ([]TurnResult, TokenUsage, error) {
	turns := make([]TurnResult, 0, len(config.Prompts))
	usage := TokenUsage{}
	sent := map[string]bool{}
	responses := []string{}

	for index := range config.Prompts {
		prompt := &config.Prompts[index]
		turn := TurnResult{Index: index + 1, Name: prompt.Name}
		// Resolved before the skip checks so a skipped turn still records which
		// prompt it was: an index alone does not say what the run left out. A
		// read failure only ends the trial if the prompt is actually sent.
		text, textErr := promptText(config, *prompt)
		turn.Prompt = text

		if !prompt.DependenciesMet(sent) {
			turn.Skipped = "after: " + strings.Join(prompt.After, ", ") + " did not run"
			turns = append(turns, turn)
			continue
		}
		env := configuration.NewPromptEnv(workspace.Root, workspace.Exec())
		env.Turn = index + 1
		env.Responses = responses
		if len(responses) > 0 {
			env.Previous = responses[len(responses)-1]
		}
		env.ElapsedSeconds = time.Since(started).Seconds()
		env.TimedOut = ctx.Err() != nil
		send, err := prompt.ShouldSend(env)
		if err != nil {
			return turns, usage, fmt.Errorf("prompts[%d].when: %w", index, err)
		}
		if !send {
			turn.Skipped = "when: " + prompt.When
			turns = append(turns, turn)
			continue
		}

		if textErr != nil {
			return turns, usage, textErr
		}
		if options.ShowDialogue {
			fmt.Fprintf(out, "\n> %s\n\n", strings.TrimSpace(text))
		}

		turnStarted := time.Now()
		turnCtx, cancel := prompt.TurnContext(ctx)
		response, err := driver.Send(turnCtx, text)
		cancel()

		turn.Sent = true
		turn.Response = response.Text
		turn.Model = response.Model
		turn.Usage = response.Usage
		turn.DurationSeconds = time.Since(turnStarted).Seconds()
		usage.Add(response.Usage)
		if prompt.Name != "" {
			sent[prompt.Name] = true
		}
		responses = append(responses, response.Text)

		if err != nil {
			turn.Error = err.Error()
			turns = append(turns, turn)
			// The conversation stops: a later prompt written as a follow-up to
			// a turn that failed would be answering a question never asked.
			// The prompts that will not be sent are still listed, so the report
			// shows the conversation that was configured and not only the part
			// of it that happened.
			for rest := index + 1; rest < len(config.Prompts); rest++ {
				turns = append(turns, TurnResult{
					Index:   rest + 1,
					Name:    config.Prompts[rest].Name,
					Skipped: fmt.Sprintf("the conversation stopped at turn %d", index+1),
				})
			}
			return turns, usage, fmt.Errorf("turn %d: %w", index+1, err)
		}
		turns = append(turns, turn)
	}
	return turns, usage, nil
}

// promptText resolves a prompt to the text that is actually sent. A file is
// read at the moment its turn comes up, so a long-running trial sends what the
// file said when the trial started rather than a stale copy read at load time.
func promptText(config *Config, prompt Prompt) (string, error) {
	if prompt.File == "" {
		return prompt.Text, nil
	}
	data, err := os.ReadFile(config.Resolve(prompt.File))
	if err != nil {
		return "", fmt.Errorf("reading prompt file: %w", err)
	}
	return string(data), nil
}

// runVerifyCommands grades the finished workspace. The commands run in order
// from a scratch directory outside it, with $MOHAE_WORKSPACE pointing at it, so
// grading cannot leave files behind that would be mistaken for the agent's work
// and the agent cannot have tailored its output to files it could see.
//
// Every command runs even after one fails: a report that stopped at the first
// failure would hide how much of the task was done.
func runVerifyCommands(ctx context.Context, config *Config, workspace *Workspace, options TrialOptions, out io.Writer) []VerifyResult {
	if len(config.Verify.Commands) == 0 {
		return nil
	}
	// Detached from the trial's deadline: a trial that ran out of time still
	// has a workspace worth grading, and grading it under an already-expired
	// context would fail every command for the wrong reason. It gets a fresh
	// deadline of the same length rather than none, so a grading command that
	// hangs — one waiting on stdin, or on a network that never answers — ends
	// the run instead of blocking it forever.
	ctx, cancel := config.Limits.Bound(context.WithoutCancel(ctx))
	defer cancel()
	// The same variables the agent had, resolved once: a grading command that
	// reads $MOHAE_MODEL should not see something different from the trial it
	// grades.
	env := trialEnv(config, workspace, workspace.Exec())
	results := make([]VerifyResult, 0, len(config.Verify.Commands))
	for _, text := range config.Verify.Commands {
		step := runShellStep(ctx, workspace.Exec(), text, workspace.Exec().Path(workspace.Scratch), env)
		if options.ShowDialogue {
			fmt.Fprintf(out, "verify %s: %s\n", VerdictWord(step.Passed), text)
		}
		results = append(results, VerifyResult{
			Command:         text,
			ExitCode:        step.ExitCode,
			Passed:          step.Passed,
			Output:          step.Output,
			DurationSeconds: step.Duration,
		})
	}
	return results
}
