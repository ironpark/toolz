package driver

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ironpark/toolz/cli/mohae/internal/claude"
	processutil "github.com/ironpark/toolz/cli/mohae/internal/process"
)

// claudeDriver drives Claude Code through the in-repo SDK client. One client
// holds the whole trial, so the conversation the configuration describes is one
// session rather than a series of unrelated prompts — a multi-turn trial only
// means something if the agent remembers the earlier turns.
type claudeDriver struct {
	client *claude.Client
	onText func(string)
}

func newClaudeDriver(ctx context.Context, options Options) (Driver, error) {
	servers := options.MCPServers
	sdkOptions := &claude.Options{
		Model:  options.Model,
		Effort: options.Effort,
		// The agent starts in the trial's copy, and nothing outside it is added
		// to the session.
		//
		// Without a container this is where the agent begins, not a boundary
		// it is held to: with permission prompts bypassed below, nothing stops
		// it reading or writing anywhere on the host, and a prompt that does
		// not name a directory has been seen to leave its work outside the
		// workspace and then fail verification for the wrong reason. The codex
		// driver constrains writes with SandboxWorkspaceWrite; this one has no
		// equivalent, so container.scope: full is what makes the two agents
		// measurable under the same rules.
		Cwd: options.Workspace,
		// Nil on the host, so the SDK starts the CLI itself. Set when the
		// trial runs its agent in a container, which is what closes the gap
		// the comment above describes: the workspace is then the only
		// filesystem the agent has.
		Command:    containedClaudeCommand(options),
		Env:        options.Env,
		MCPServers: claudeMCPServers(servers),
		// Only what the trial installed: a server the host happens to have
		// configured would be a tool the configuration never granted, and two
		// machines would then measure different things.
		StrictMCPConfig: len(servers) > 0,
		// A benchmark cannot answer permission prompts, and one left unanswered
		// is a trial that stalls until its timeout instead of producing a result.
		PermissionMode: claude.PermissionModeBypassPermissions,
	}
	client := claude.NewClient(sdkOptions)
	if err := client.Connect(ctx); err != nil {
		return nil, fmt.Errorf("claude-code: %w", err)
	}
	return &claudeDriver{client: client, onText: options.OnText}, nil
}

// containedClaudeCommand builds the CLI somewhere other than this host, or
// returns nil when the agent runs here after all. The SDK builds a full
// environment on top of this process's own; only what it added is forwarded,
// since the container has its own PATH and HOME and inheriting this machine's
// would point the agent at directories that do not exist inside it.
func containedClaudeCommand(options Options) claude.CommandBuilder {
	executor := options.executor()
	if !executor.Contained() {
		return nil
	}
	return func(ctx context.Context, path string, args []string, dir string, env []string) *exec.Cmd {
		return executor.Command(ctx, append([]string{path}, args...), dir, processutil.Overlay(env))
	}
}

// claudeMCPServers translates the parsed specs into the SDK's own shapes. The
// specs come from the same file format the CLI reads, so this is a change of
// type and not of meaning.
func claudeMCPServers(specs []MCPServerSpec) map[string]claude.MCPServerConfig {
	if len(specs) == 0 {
		return nil
	}
	servers := map[string]claude.MCPServerConfig{}
	for _, spec := range specs {
		switch {
		case spec.URL != "" && strings.EqualFold(spec.Type, "sse"):
			servers[spec.Name] = &claude.MCPSSEServerConfig{URL: spec.URL, Headers: spec.Headers}
		case spec.URL != "":
			servers[spec.Name] = &claude.MCPHTTPServerConfig{URL: spec.URL, Headers: spec.Headers}
		default:
			servers[spec.Name] = &claude.MCPStdioServerConfig{Command: spec.Command, Args: spec.Args, Env: spec.Env}
		}
	}
	return servers
}

func (d *claudeDriver) Send(ctx context.Context, prompt string) (Response, error) {
	if err := d.client.Query(ctx, prompt, claude.DefaultSessionID); err != nil {
		return Response{}, fmt.Errorf("claude-code: %w", err)
	}
	response := Response{}
	var text strings.Builder
	// ReceiveResponse ends at the turn's result message, which is also where
	// the usage the report needs is reported.
	for message, err := range d.client.ReceiveResponse(ctx) {
		if err != nil {
			return response, fmt.Errorf("claude-code: %w", err)
		}
		switch message := message.(type) {
		case *claude.AssistantMessage:
			for _, block := range message.Content {
				block, ok := block.(*claude.TextBlock)
				if !ok {
					// Tool calls and thinking are part of how the agent worked,
					// not part of what it answered; the transcript records the
					// reply, and verification judges the workspace.
					continue
				}
				text.WriteString(block.Text)
				text.WriteString("\n")
				if d.onText != nil {
					d.onText(block.Text + "\n")
				}
			}
			if message.Model != "" {
				response.Model = message.Model
			}
		case *claude.ResultMessage:
			response.Usage = claudeUsage(message)
			if message.IsError {
				return response, fmt.Errorf("claude-code: turn failed: %s", strings.Join(message.Errors, "; "))
			}
		}
	}
	response.Text = strings.TrimRight(text.String(), "\n")
	return response, nil
}

// claudeUsage reads the token counts off a result message. ModelUsage is
// preferred over the raw usage map because it is already typed and already
// separates cached reads from fresh input, which is the distinction
// --detailed-tokens exists to show.
func claudeUsage(message *claude.ResultMessage) TokenUsage {
	usage := TokenUsage{}
	for _, model := range message.ModelUsage {
		usage.Input += model.InputTokens
		usage.Output += model.OutputTokens
		usage.CacheRead += model.CacheReadInputTokens
		usage.CacheWrite += model.CacheCreationInputTokens
	}
	if message.TotalCostUSD != nil {
		usage.CostUSD = *message.TotalCostUSD
	}
	if usage.Total() == 0 {
		// Older payloads report only the untyped map.
		usage.Input = intField(message.Usage, "input_tokens")
		usage.Output = intField(message.Usage, "output_tokens")
		usage.CacheRead = intField(message.Usage, "cache_read_input_tokens")
		usage.CacheWrite = intField(message.Usage, "cache_creation_input_tokens")
	}
	return usage
}

// intField reads a number out of a decoded JSON map, where every number is a
// float64 whatever it was written as.
func intField(values map[string]any, key string) int {
	switch value := values[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}

func (d *claudeDriver) Close() error { return d.client.Disconnect() }
