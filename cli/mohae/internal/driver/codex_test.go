package driver

import (
	"strings"
	"testing"
)

func TestCodexArgsOverrideTheHostsMCPConfiguration(t *testing.T) {
	arguments := codexArgs([]MCPServerSpec{
		{Name: "files", Command: "server-files", Args: []string{"--root", "."}, Env: map[string]string{"TOKEN": "x"}},
	})
	joined := strings.Join(arguments, " ")
	for _, want := range []string{
		"mcp_servers.files.command=server-files",
		`mcp_servers.files.args=["--root","."]`,
		"mcp_servers.files.env.TOKEN=x",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args = %q, want it to contain %q", joined, want)
		}
	}
	if arguments[len(arguments)-1] != "app-server" {
		t.Errorf("args = %q, want the subcommand last", joined)
	}
	if len(codexArgs(nil)) != 0 {
		t.Error("a trial with no servers should not override anything")
	}
}
