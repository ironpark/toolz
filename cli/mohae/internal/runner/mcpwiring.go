package runner

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
	processutil "github.com/ironpark/toolz/cli/mohae/internal/process"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPServerSpec is owned by driver; the alias keeps runner's API readable
// while preserving that single source of truth.
type MCPServerSpec = agentdriver.MCPServerSpec

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
			if err := validateSpec(spec); err != nil {
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

func validateSpec(s MCPServerSpec) error {
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

// SpecTransport builds the go-sdk transport for this server. mohae uses the SDK
// rather than its own client so what it can reach is what a compliant MCP
// client can reach: a server mohae connects to here is one the agent will be
// able to use, and a failure here is a real one rather than a quirk of a
// hand-written probe.
func SpecTransport(ctx context.Context, s MCPServerSpec) (mcp.Transport, error) {
	if s.URL != "" {
		if strings.EqualFold(s.Type, "sse") {
			return &mcp.SSEClientTransport{Endpoint: s.URL}, nil
		}
		return &mcp.StreamableClientTransport{Endpoint: s.URL}, nil
	}
	command := exec.CommandContext(ctx, s.Command, s.Args...)
	command.Env = processutil.Env(s.Env)
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
func ProbeMCPServers(ctx context.Context, specs []MCPServerSpec, version string) []MCPProbe {
	// Concurrently: each probe spawns its own subprocess or session, they share
	// nothing, and probing sits inside the trial's timeout budget — serialising
	// them would charge the run the sum of every server's start-up.
	probes := make([]MCPProbe, len(specs))
	var wait sync.WaitGroup
	for index, spec := range specs {
		wait.Add(1)
		go func() {
			defer wait.Done()
			probes[index] = probeSpec(ctx, spec, version)
		}()
	}
	wait.Wait()
	return probes
}

func probeSpec(ctx context.Context, s MCPServerSpec, version string) MCPProbe {
	probe := MCPProbe{Name: s.Name}
	transport, err := SpecTransport(ctx, s)
	if err != nil {
		probe.Error = err.Error()
		return probe
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "mohae", Version: version}, nil)
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
