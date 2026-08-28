package main

import (
	"context"
	"fmt"
	"maps"
	"os"
	"slices"
)

// Driver is one agent under test, already opened on a trial's workspace. The
// runner talks to every agent through this interface and knows nothing about
// which one it drives, which is what makes a claude-code trial and a custom-cli
// trial comparable: they differ in the driver and in nothing else.
type Driver interface {
	// Send delivers one prompt and returns the agent's reply once it stops
	// producing output for it. The context bounds the turn; a cancelled context
	// must stop the agent rather than leave it running.
	Send(ctx context.Context, prompt string) (Response, error)
	// Close releases the session. It is called once, after the last turn.
	Close() error
}

// Response is what one turn produced.
type Response struct {
	// Text is the agent's reply. It is what a `when` condition sees as
	// `previous`, and what the report shows.
	Text string
	// Model is what actually answered, which need not be what was configured:
	// a fallback model is exactly the kind of thing a benchmark has to record.
	Model string
	Usage TokenUsage
}

// TokenUsage is a turn's token spend. The categories are kept apart rather than
// summed because a cached read and a fresh input token cost different amounts,
// and `--detailed-tokens` exists to show that difference.
type TokenUsage struct {
	Input      int
	Output     int
	CacheRead  int
	CacheWrite int
	CostUSD    float64
}

// Total is every token the turn touched, cached or not.
func (u TokenUsage) Total() int {
	return u.Input + u.Output + u.CacheRead + u.CacheWrite
}

// Add accumulates another turn's usage into this one.
func (u *TokenUsage) Add(other TokenUsage) {
	u.Input += other.Input
	u.Output += other.Output
	u.CacheRead += other.CacheRead
	u.CacheWrite += other.CacheWrite
	u.CostUSD += other.CostUSD
}

// DriverOptions is everything a driver needs to open a session.
type DriverOptions struct {
	Config    *Config
	Workspace *Workspace
	// Env is what the trial adds to the agent's environment: what it selected,
	// so an agent command can read its model and effort without mohae having to
	// know that command's flags. NewDriver resolves it; a driver reads it and
	// never rebuilds it, so every agent sees the same variables and the trials
	// stay comparable.
	Env map[string]string
	// MCPServers are the servers the trial resolved from the configuration,
	// already filtered to this agent type. The runner loads them once — it
	// probes them before the agent starts — and every driver translates the
	// same list, so a driver cannot end up wired to a different set of tools
	// than the one the report says was reachable.
	MCPServers []MCPServerSpec
	// OnText receives the agent's output as it arrives, for --show-dialogue.
	// It may be nil, and drivers whose transport reports only finished turns
	// call it once with the whole reply rather than not at all.
	OnText func(string)
}

// agentTypes is the one place an agent type is defined: how to open it and
// where it reads the skills a workspace installs. Adding an agent is one entry
// here — validation, driver selection and workspace preparation all read this
// table, so a type cannot be accepted by the config and then found to have no
// driver, or run a trial whose skills were installed where it never looks.
var agentTypes = map[string]struct {
	// skillDir is the path, relative to the workspace root, that this agent
	// reads skills from. A skill dropped anywhere else would be a trial that
	// silently measured the agent without it.
	skillDir string
	open     func(context.Context, DriverOptions) (Driver, error)
}{
	"claude-code": {".claude/skills", newClaudeDriver},
	"codex":       {".codex/skills", newCodexDriver},
	"custom-cli":  {".agent/skills", newCustomDriver},
}

// KnownAgentTypes are the drivers the runner can select, in a stable order for
// error messages. custom-cli covers any agent with a non-interactive command
// line.
var KnownAgentTypes = slices.Sorted(maps.Keys(agentTypes))

// NewDriver opens the driver named by the configuration's agent type, with the
// trial environment already resolved so no driver assembles it itself.
func NewDriver(ctx context.Context, options DriverOptions) (Driver, error) {
	agent, ok := agentTypes[options.Config.Agent.Type]
	if !ok {
		return nil, fmt.Errorf("no driver for agent type %q", options.Config.Agent.Type)
	}
	options.Env = driverEnv(options.Config, options.Workspace)
	return agent.open(ctx, options)
}

// driverEnv resolves the trial's environment. See DriverOptions.Env.
func driverEnv(config *Config, workspace *Workspace) map[string]string {
	env := map[string]string{
		"MOHAE_WORKSPACE": workspace.Root,
		"MOHAE_TRIAL":     config.Name,
	}
	if config.Agent.Model != "" {
		env["MOHAE_MODEL"] = config.Agent.Model
	}
	if config.Agent.Effort != "" {
		env["MOHAE_EFFORT"] = config.Agent.Effort
	}
	// Copied last so a config can override anything above deliberately.
	maps.Copy(env, config.Agent.Env)
	return env
}

// environ is the environment a driver's subprocess starts from.
func (o DriverOptions) environ() []string { return processEnv(o.Env) }

// processEnv puts extra on top of this process's environment. The parent's is
// inherited because an agent CLI needs its own credentials and PATH, and
// stripping them would only mean measuring an agent that cannot log in.
//
// extra comes last and in sorted order, so a configuration's setting wins and
// two runs of it differ in the agent's behaviour rather than in their inputs.
func processEnv(extra map[string]string) []string {
	environ := os.Environ()
	for _, key := range slices.Sorted(maps.Keys(extra)) {
		environ = append(environ, key+"="+extra[key])
	}
	return environ
}
