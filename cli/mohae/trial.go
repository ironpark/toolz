package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// TrialOptions are the run-time choices a trial takes from the command line
// rather than from the configuration.
type TrialOptions struct {
	// ShowDialogue streams the conversation to Out while it runs.
	ShowDialogue bool
	// Out receives the streamed dialogue. A nil Out means os.Stdout.
	Out io.Writer
	// KeepWorkspace leaves the trial's directory behind even when it passed.
	// A failed trial's workspace is always kept: it is the only record of what
	// the agent actually did.
	KeepWorkspace bool
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
	if config.Limits.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(config.Limits.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	workspace, err := PrepareWorkspace(ctx, config, config.Agent.Type)
	if err != nil {
		// Nothing ran, so there is nothing to grade and no workspace to keep.
		result.Error = err.Error()
		result.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		return result
	}
	defer func() {
		if result.Passed && !options.KeepWorkspace {
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
		result.MCP = ProbeMCPServers(ctx, servers)
	}

	out := options.Out
	if out == nil {
		out = os.Stdout
	}
	var onText func(string)
	if options.ShowDialogue {
		onText = func(text string) { fmt.Fprint(out, text) }
	}
	driver, err := NewDriver(ctx, DriverOptions{Config: config, Workspace: workspace, MCPServers: servers, OnText: onText})
	if err != nil {
		result.Error = err.Error()
		result.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		return result
	}
	defer driver.Close()

	result.Turns, result.Usage, err = runConversation(ctx, config, workspace, driver, options, out, started)
	if err != nil {
		result.Error = err.Error()
	}
	for _, turn := range result.Turns {
		if turn.Model != "" {
			result.ModelUsed = turn.Model
			break
		}
	}
	result.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)

	// Verification runs even after a failed or timed-out conversation: the
	// workspace is still there to be graded, and "the agent stopped early but
	// the task was done" is a result worth telling apart from "it was not".
	result.Verify = runVerifyCommands(ctx, config, workspace, options, out)
	result.Passed = result.Error == "" && !result.TimedOut && result.VerifyPassed() == len(result.Verify)
	return result
}

// runConversation walks the prompts in order. A prompt is skipped when the
// prompts it comes after were never sent, or when its own condition is false;
// both are recorded, because a conversation that silently shrank would make two
// different runs look identical.
func runConversation(ctx context.Context, config *Config, workspace *Workspace, driver Driver, options TrialOptions, out io.Writer, started time.Time) ([]TurnResult, TokenUsage, error) {
	turns := make([]TurnResult, 0, len(config.Prompts))
	usage := TokenUsage{}
	sent := map[string]bool{}
	responses := []string{}

	for index := range config.Prompts {
		prompt := &config.Prompts[index]
		turn := TurnResult{Index: index + 1, Name: prompt.Name}

		if !prompt.DependenciesMet(sent) {
			turn.Skipped = "after: " + strings.Join(prompt.After, ", ") + " did not run"
			turns = append(turns, turn)
			continue
		}
		env := NewPromptEnv(workspace.Root)
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

		text, err := promptText(config, *prompt)
		if err != nil {
			return turns, usage, err
		}
		turn.Prompt = text
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
	// context would fail every command for the wrong reason.
	ctx = context.WithoutCancel(ctx)
	results := make([]VerifyResult, 0, len(config.Verify.Commands))
	for _, text := range config.Verify.Commands {
		started := time.Now()
		command := exec.CommandContext(ctx, "sh", "-c", text)
		command.Dir = workspace.Scratch
		command.Env = append(os.Environ(), "MOHAE_WORKSPACE="+workspace.Root)
		isolateProcess(command)
		output := &bytes.Buffer{}
		command.Stdout = output
		command.Stderr = output
		err := command.Run()

		result := VerifyResult{
			Command:         text,
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
			// A command that could not be started at all: the shell's own
			// failure is the output, since there is none of its own.
			result.ExitCode = -1
			result.Output = strings.TrimSpace(result.Output + "\n" + err.Error())
		}
		if options.ShowDialogue {
			fmt.Fprintf(out, "verify %s: %s\n", verdictWord(result.Passed), text)
		}
		results = append(results, result)
	}
	return results
}

func verdictWord(passed bool) string {
	if passed {
		return "pass"
	}
	return "fail"
}
