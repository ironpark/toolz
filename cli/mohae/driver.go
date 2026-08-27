package main

import (
	"context"
	"fmt"
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
	// OnText receives the agent's output as it arrives, for --show-dialogue.
	// It may be nil, and drivers whose transport reports only finished turns
	// call it once with the whole reply rather than not at all.
	OnText func(string)
}

// NewDriver opens the driver named by the configuration's agent type. It is the
// only place agent types are mapped to implementations; Config.Validate has
// already rejected any type not listed here.
func NewDriver(ctx context.Context, options DriverOptions) (Driver, error) {
	switch options.Config.Agent.Type {
	case "custom-cli":
		return newCustomDriver(options)
	case "claude-code", "codex":
		return nil, notImplemented(options.Config.Agent.Type + " driver")
	default:
		return nil, fmt.Errorf("no driver for agent type %q", options.Config.Agent.Type)
	}
}

// driverEnv is the environment every driver adds to the agent's own: what the
// trial selected, so an agent command can read its model and effort without
// mohae having to know that command's flags.
func driverEnv(config *Config, workspace *Workspace) []string {
	env := []string{
		"MOHAE_WORKSPACE=" + workspace.Root,
		"MOHAE_TRIAL=" + config.Name,
	}
	if config.Agent.Model != "" {
		env = append(env, "MOHAE_MODEL="+config.Agent.Model)
	}
	if config.Agent.Effort != "" {
		env = append(env, "MOHAE_EFFORT="+config.Agent.Effort)
	}
	// Configured last so a config can override anything above deliberately.
	for _, key := range sortedEnvKeys(config.Agent.Env) {
		env = append(env, key+"="+config.Agent.Env[key])
	}
	return env
}

// sortedEnvKeys keeps the environment deterministic, so two runs of the same
// configuration differ in the agent's behaviour and not in their inputs.
func sortedEnvKeys(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
