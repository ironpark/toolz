package driver

import (
	"context"
	"fmt"

	"github.com/ironpark/toolz/cli/mohae/internal/agent"
	"github.com/ironpark/toolz/cli/mohae/internal/process"
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
//
// The JSON names are part of the report file format, so they are spelled out in
// the same snake_case the rest of the document uses.
type TokenUsage struct {
	Input      int     `json:"input"`
	Output     int     `json:"output"`
	CacheRead  int     `json:"cache_read"`
	CacheWrite int     `json:"cache_write"`
	CostUSD    float64 `json:"cost_usd"`
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

// Options is everything a driver needs to open a session. It deliberately
// contains values rather than the runner's Config or Workspace types, keeping
// this internal package independent from package main.
type Options struct {
	Type      string
	Model     string
	Effort    string
	Command   []string
	Workspace string
	Version   string
	// Env is the complete trial overlay, including MOHAE_* and agent.env.
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

// MCPServerSpec is the driver-facing representation of one configured MCP
// server. The runner owns loading and probing; drivers only translate this
// value into the selected agent CLI's configuration.
type MCPServerSpec struct {
	Name    string
	Command string
	Args    []string
	Env     map[string]string
	Type    string
	URL     string
	Headers map[string]string
}

// agentTypes binds each shared agent kind to its concrete session opener.
var agentTypes = map[string]func(context.Context, Options) (Driver, error){
	agent.ClaudeCode: newClaudeDriver,
	agent.Codex:      newCodexDriver,
	agent.CustomCLI:  newCustomDriver,
}

// New opens the selected driver.
func New(ctx context.Context, options Options) (Driver, error) {
	open, ok := agentTypes[options.Type]
	if !ok {
		return nil, fmt.Errorf("no driver for agent type %q", options.Type)
	}
	return open(ctx, options)
}

// environ is the environment a driver's subprocess starts from.
func (o Options) environ() []string { return process.Env(o.Env) }
