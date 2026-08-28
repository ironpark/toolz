package main

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/ironpark/toolz/cli/mohae/internal/codex"
)

// codexDriver drives Codex through the app-server client. One thread holds the
// whole trial, so every prompt in the configuration is a turn in the same
// conversation.
type codexDriver struct {
	client *codex.Client
	thread *codex.Thread
	model  string
	effort string
	onText func(string)
}

func newCodexDriver(ctx context.Context, options DriverOptions) (Driver, error) {
	config := options.Config
	servers := options.MCPServers
	client, err := codex.New(ctx, codex.Options{
		Args: codexArgs(servers),
		Dir:  options.Workspace.Root,
		Env:  append(driverEnvironment(), driverEnv(config, options.Workspace)...),
		// The subprocess's own logging is not part of the trial's transcript.
		Stderr:     io.Discard,
		ClientInfo: codex.ClientInfo{Name: "mohae", Version: buildVersion()},
		// A benchmark has nobody to ask. Approving is the only answer that
		// measures the agent rather than measuring how long it waits, and the
		// workspace is a disposable copy, so there is nothing to protect.
		Approvals: codex.ApprovalFuncs{
			Command: func(context.Context, *codex.CommandApprovalRequest) (codex.Decision, error) {
				return codex.DecisionAccept, nil
			},
			FileChange: func(context.Context, *codex.FileChangeApprovalRequest) (codex.Decision, error) {
				return codex.DecisionAccept, nil
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("codex: %w", err)
	}
	thread, err := client.StartThread(ctx, codex.StartThreadParams{
		Model: config.Agent.Model,
		Cwd:   options.Workspace.Root,
		// The trial's workspace is the only thing the agent may write to, and
		// it is a copy, so full access inside it costs nothing.
		ApprovalPolicy: codex.ApprovalNever,
		SandboxPolicy:  codex.SandboxWorkspaceWrite([]string{options.Workspace.Root}, true, nil),
		ServiceName:    "mohae",
	})
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("codex: %w", err)
	}
	return &codexDriver{
		client: client,
		thread: thread,
		model:  config.Agent.Model,
		effort: config.Agent.Effort,
		onText: options.OnText,
	}, nil
}

// codexArgs launches the app-server with the trial's MCP servers set as config
// overrides. Codex reads its servers from its own configuration file, so the
// only way to give a trial exactly the servers the config named — and not
// whatever the host machine has configured — is to override them on the
// command line.
func codexArgs(specs []MCPServerSpec) []string {
	if len(specs) == 0 {
		return nil
	}
	arguments := []string{}
	for _, spec := range specs {
		key := "mcp_servers." + spec.Name
		switch {
		case spec.URL != "":
			arguments = append(arguments, "-c", key+".url="+spec.URL)
		default:
			arguments = append(arguments, "-c", key+".command="+spec.Command)
			if len(spec.Args) > 0 {
				arguments = append(arguments, "-c", key+".args="+tomlStringList(spec.Args))
			}
		}
		for _, name := range slices.Sorted(maps.Keys(spec.Env)) {
			arguments = append(arguments, "-c", key+".env."+name+"="+spec.Env[name])
		}
	}
	// The subcommand comes last: codex takes its global overrides first.
	return append(arguments, "app-server")
}

// tomlStringList renders an argument list the way codex's `-c` parser reads it.
func tomlStringList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, `"`+strings.ReplaceAll(value, `"`, `\"`)+`"`)
	}
	return "[" + strings.Join(quoted, ",") + "]"
}

func (d *codexDriver) Send(ctx context.Context, prompt string) (Response, error) {
	stream, err := d.client.StartTurn(ctx, d.thread.ID, codex.Text(prompt), &codex.TurnOptions{
		Model:  d.model,
		Effort: d.effort,
	})
	if err != nil {
		return Response{}, fmt.Errorf("codex: %w", err)
	}
	defer stream.Close()

	response := Response{Model: d.model}
	var text strings.Builder
	for event := range stream.Events() {
		switch event.Kind {
		case codex.EventAgentMessageDelta:
			// Deltas are what makes --show-dialogue live; the completed item
			// below is what is kept, so the two never double up.
			if d.onText != nil {
				d.onText(event.Delta)
			}
		case codex.EventItemCompleted:
			if event.Item == nil {
				continue
			}
			if message, ok := event.Item.Item.(codex.AgentMessageItem); ok {
				text.WriteString(message.Text)
				text.WriteString("\n")
			}
		case codex.EventTokenUsageUpdated:
			if event.Usage != nil {
				response.Usage = codexUsage(event.Usage)
			}
		}
	}
	turn, err := stream.Wait(ctx)
	if err != nil {
		return response, fmt.Errorf("codex: %w", err)
	}
	if turn != nil && turn.Usage != nil {
		// The turn's own total supersedes the running updates.
		response.Usage = codexUsage(turn.Usage)
	}
	response.Text = strings.TrimRight(text.String(), "\n")
	if turn != nil && turn.Status == codex.TurnFailed {
		return response, fmt.Errorf("codex: turn failed: %s", turn.Error.Error())
	}
	return response, nil
}

// codexUsage maps codex's counters onto mohae's. Codex reports cached input as
// part of the input total, so the cached share is subtracted back out to keep
// "input" meaning tokens that were actually paid for at full price.
func codexUsage(usage *codex.TokenUsage) TokenUsage {
	input := int(usage.InputTokens - usage.CachedInputTokens)
	if input < 0 {
		input = int(usage.InputTokens)
	}
	return TokenUsage{
		Input:     input,
		Output:    int(usage.OutputTokens),
		CacheRead: int(usage.CachedInputTokens),
	}
}

func (d *codexDriver) Close() error { return d.client.Close() }
