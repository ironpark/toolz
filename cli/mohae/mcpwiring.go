package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"

	agentdriver "github.com/ironpark/toolz/cli/mohae/internal/driver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPServerSpec is one server as an agent CLI's configuration file describes
// it. Both shapes are kept because both are in use: a stdio server is a command
// mohae launches, an HTTP or SSE server is an endpoint it connects to.
type MCPServerSpec struct {
	Name string `json:"-"`

	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`

	// Type is the transport named by the file ("stdio", "http", "sse"). It is
	// optional: a spec with a URL is HTTP and one with a command is stdio, so
	// only an SSE endpoint has to say so.
	Type string `json:"type,omitempty"`
	URL  string `json:"url,omitempty"`

	// Headers are sent with every HTTP request, which is how a hosted server is
	// authenticated.
	Headers map[string]string `json:"headers,omitempty"`
}

func driverMCPServers(specs []MCPServerSpec) []agentdriver.MCPServerSpec {
	converted := make([]agentdriver.MCPServerSpec, len(specs))
	for index, spec := range specs {
		converted[index] = agentdriver.MCPServerSpec{
			Name: spec.Name, Command: spec.Command, Args: spec.Args, Env: spec.Env,
			Type: spec.Type, URL: spec.URL, Headers: spec.Headers,
		}
	}
	return converted
}

// mcpConfigFile is the format the agent CLIs already read. mohae reads the same
// file rather than inventing its own so one server configuration can be handed
// to the agent unchanged — what mohae connects to and what the agent connects
// to are then the same thing by construction.
type mcpConfigFile struct {
	MCPServers map[string]MCPServerSpec `json:"mcpServers"`
	// Servers is the spelling some tools use; accepted so a working file does
	// not have to be rewritten to be measured.
	Servers map[string]MCPServerSpec `json:"servers"`
}

// LoadMCPServers reads the MCP configurations enabled for one agent type.
// Servers scoped to other agents are left out here rather than filtered later,
// so a driver is handed exactly what its agent is meant to see.
//
// Later files win on a name collision: the configuration lists them in order,
// and a run that silently kept the first would be hard to explain.
func LoadMCPServers(config *Config, agentType string) ([]MCPServerSpec, error) {
	byName := map[string]MCPServerSpec{}
	for index, server := range config.MCP {
		if !server.EnabledFor(agentType) {
			continue
		}
		path := config.Resolve(server.Config)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("mcp[%d].config: %w", index, err)
		}
		file := mcpConfigFile{}
		if err := json.Unmarshal(data, &file); err != nil {
			return nil, fmt.Errorf("mcp[%d].config: %s: %w", index, path, err)
		}
		entries := file.MCPServers
		if len(entries) == 0 {
			entries = file.Servers
		}
		if len(entries) == 0 {
			return nil, fmt.Errorf("mcp[%d].config: %s defines no servers", index, path)
		}
		for name, spec := range entries {
			spec.Name = name
			// A config entry may name the one server it means; that name then
			// labels every server the file defines only when the file has one,
			// which is the case the field exists for.
			if server.Name != "" && len(entries) == 1 {
				spec.Name = server.Name
			}
			if err := spec.validate(); err != nil {
				return nil, fmt.Errorf("mcp[%d].config: %s: %w", index, path, err)
			}
			byName[spec.Name] = spec
		}
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	specs := make([]MCPServerSpec, 0, len(names))
	for _, name := range names {
		specs = append(specs, byName[name])
	}
	return specs, nil
}

func (s MCPServerSpec) validate() error {
	if s.Command == "" && s.URL == "" {
		return fmt.Errorf("server %q has neither a command nor a url", s.Name)
	}
	if s.Command != "" && s.URL != "" {
		// Which one wins would decide what the agent is actually measured
		// against, so the file has to say.
		return fmt.Errorf("server %q has both a command and a url", s.Name)
	}
	return nil
}

// Transport builds the go-sdk transport for this server. mohae uses the SDK
// rather than its own client so what it can reach is what a compliant MCP
// client can reach: a server mohae connects to here is one the agent will be
// able to use, and a failure here is a real one rather than a quirk of a
// hand-written probe.
func (s MCPServerSpec) Transport(ctx context.Context) (mcp.Transport, error) {
	if s.URL != "" {
		if strings.EqualFold(s.Type, "sse") {
			return &mcp.SSEClientTransport{Endpoint: s.URL}, nil
		}
		return &mcp.StreamableClientTransport{Endpoint: s.URL}, nil
	}
	command := exec.CommandContext(ctx, s.Command, s.Args...)
	command.Env = processEnv(s.Env)
	return &mcp.CommandTransport{Command: command}, nil
}

// MCPProbe is what one server answered when the trial connected to it.
//
// It is recorded in the report because a server that failed to start turns a
// trial into a measurement of an agent working without the tools it was meant
// to have — a result that looks like a failed task rather than a broken setup.
type MCPProbe struct {
	Name  string   `json:"name"`
	OK    bool     `json:"ok"`
	Tools []string `json:"tools,omitempty"`
	Error string   `json:"error,omitempty"`
}

// ProbeMCPServers connects to each server, lists its tools and disconnects.
// A failure is recorded rather than returned: the trial can still run, and
// whether it should have is a judgement the report leaves to its reader.
func ProbeMCPServers(ctx context.Context, specs []MCPServerSpec) []MCPProbe {
	// Concurrently: each probe spawns its own subprocess or session, they share
	// nothing, and probing sits inside the trial's timeout budget — serialising
	// them would charge the run the sum of every server's start-up.
	probes := make([]MCPProbe, len(specs))
	var wait sync.WaitGroup
	for index, spec := range specs {
		wait.Add(1)
		go func() {
			defer wait.Done()
			probes[index] = spec.probe(ctx)
		}()
	}
	wait.Wait()
	return probes
}

func (s MCPServerSpec) probe(ctx context.Context) MCPProbe {
	probe := MCPProbe{Name: s.Name}
	transport, err := s.Transport(ctx)
	if err != nil {
		probe.Error = err.Error()
		return probe
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "mohae", Version: buildVersion()}, nil)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		probe.Error = err.Error()
		return probe
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		probe.Error = err.Error()
		return probe
	}
	probe.OK = true
	for _, tool := range tools.Tools {
		probe.Tools = append(probe.Tools, tool.Name)
	}
	sort.Strings(probe.Tools)
	return probe
}
