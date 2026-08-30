package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	agentdriver "github.com/ironpark/toolz/cli/mohae/internal/driver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpConfig writes an MCP configuration file and points the config at it.
func mcpConfig(t *testing.T, config *Config, name, content string, agents []string) {
	t.Helper()
	path := filepath.Join(filepath.Dir(config.Path), name)
	writeFile(t, path, content, 0o644)
	config.MCP = append(config.MCP, MCPServerConfig{Config: "./" + name, Agents: agents})
}

func TestLoadMCPServersReadsTheFormatTheAgentCLIsRead(t *testing.T) {
	directory := t.TempDir()
	config := fixtureConfig(t, directory)
	mcpConfig(t, config, "mcp.json", `{
	  "mcpServers": {
	    "files": {"command": "server-files", "args": ["--root", "."], "env": {"TOKEN": "x"}},
	    "hosted": {"type": "sse", "url": "https://example.test/sse"}
	  }
	}`, nil)

	specs, err := LoadMCPServers(config, "custom-cli")
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 {
		t.Fatalf("loaded %d servers, want 2", len(specs))
	}
	// Sorted by name, so a report lists them the same way on every run.
	if specs[0].Name != "files" || specs[1].Name != "hosted" {
		t.Fatalf("names = %q, %q", specs[0].Name, specs[1].Name)
	}
	if specs[0].Command != "server-files" || len(specs[0].Args) != 2 || specs[0].Env["TOKEN"] != "x" {
		t.Errorf("stdio server = %+v", specs[0])
	}
	if specs[1].URL != "https://example.test/sse" || specs[1].Type != "sse" {
		t.Errorf("remote server = %+v", specs[1])
	}
}

func TestLoadMCPServersHonoursTheAgentScope(t *testing.T) {
	directory := t.TempDir()
	config := fixtureConfig(t, directory)
	mcpConfig(t, config, "shared.json", `{"mcpServers": {"shared": {"command": "shared"}}}`, nil)
	mcpConfig(t, config, "claude.json", `{"mcpServers": {"claude-only": {"command": "claude-only"}}}`, []string{"claude-code"})

	specs, err := LoadMCPServers(config, "custom-cli")
	if err != nil {
		t.Fatal(err)
	}
	// Offering it anyway would give the agent a tool the configuration said it
	// does not get, and the comparison the scope exists for would be void.
	if len(specs) != 1 || specs[0].Name != "shared" {
		t.Fatalf("specs = %+v, want only the unscoped server", specs)
	}
	if specs, err := LoadMCPServers(config, "claude-code"); err != nil || len(specs) != 2 {
		t.Fatalf("claude-code got %+v, %v; want both servers", specs, err)
	}
}

func TestLoadMCPServersRejectsAnUnusableSpec(t *testing.T) {
	cases := map[string]string{
		"neither command nor url": `{"mcpServers": {"broken": {}}}`,
		"both command and url":    `{"mcpServers": {"broken": {"command": "x", "url": "https://example.test"}}}`,
		"no servers at all":       `{"mcpServers": {}}`,
		"not json":                `{`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			config := fixtureConfig(t, directory)
			mcpConfig(t, config, "mcp.json", content, nil)
			if _, err := LoadMCPServers(config, "custom-cli"); err == nil {
				// A server silently dropped would be a trial measuring an agent
				// without the tools it was meant to have.
				t.Fatal("expected the configuration to be rejected")
			}
		})
	}
}

func TestMCPServerSpecBuildsTheTransportItsShapeImplies(t *testing.T) {
	ctx := context.Background()
	stdio, err := MCPServerSpec{Name: "files", Command: "server", Args: []string{"--root", "."}}.Transport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	command, ok := stdio.(*mcp.CommandTransport)
	if !ok {
		t.Fatalf("stdio transport = %T", stdio)
	}
	if !strings.HasSuffix(command.Command.Args[0], "server") {
		t.Errorf("command = %v", command.Command.Args)
	}

	http, err := MCPServerSpec{Name: "hosted", URL: "https://example.test/mcp"}.Transport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := http.(*mcp.StreamableClientTransport); !ok {
		t.Errorf("http transport = %T", http)
	}
	sse, err := MCPServerSpec{Name: "hosted", Type: "SSE", URL: "https://example.test/sse"}.Transport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sse.(*mcp.SSEClientTransport); !ok {
		t.Errorf("sse transport = %T", sse)
	}
}

func TestProbeRecordsAServerThatCouldNotBeReached(t *testing.T) {
	// A server that fails to start turns a trial into a measurement of an agent
	// working without its tools, which reads as a failed task unless the report
	// says otherwise. Nothing is launched here: the command does not exist.
	probes := ProbeMCPServers(context.Background(), []MCPServerSpec{{Name: "missing", Command: "mohae-no-such-server"}})
	if len(probes) != 1 {
		t.Fatalf("probes = %+v", probes)
	}
	if probes[0].OK || probes[0].Error == "" {
		t.Errorf("probe = %+v, want a recorded failure", probes[0])
	}
}

func TestDriversAreSelectedByAgentTypeWithoutAnyAgentInstalled(t *testing.T) {
	// The factory is the only place agent types map to implementations. It is
	// checked without launching anything: a test that needed a real claude or
	// codex binary would only pass on a machine that happened to have one.
	for _, agentType := range agentdriver.KnownAgentTypes {
		if agentType == "custom-cli" {
			continue
		}
		if _, err := exec.LookPath(agentType); err == nil {
			t.Skipf("%s is installed; this test is about the machines where it is not", agentType)
		}
	}
	directory := t.TempDir()
	config := fixtureConfig(t, directory)
	workspace, err := PrepareWorkspace(context.Background(), config, "custom-cli")
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Cleanup()

	for _, agentType := range agentdriver.KnownAgentTypes {
		config.Agent.Type = agentType
		driver, err := agentdriver.New(context.Background(), newDriverOptions(config, workspace, nil, nil))
		switch agentType {
		case "custom-cli":
			if err != nil {
				t.Errorf("%s: %v", agentType, err)
				continue
			}
			driver.Close()
		default:
			// The type is known, so the failure has to be about the missing
			// binary rather than about mohae not knowing how to drive it.
			if err == nil {
				driver.Close()
				t.Errorf("%s: expected the missing agent binary to be reported", agentType)
				continue
			}
			if strings.Contains(err.Error(), "no driver for agent type") {
				t.Errorf("%s: has no driver", agentType)
			}
		}
	}
}
