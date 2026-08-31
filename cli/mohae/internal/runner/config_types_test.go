package runner

import (
	"github.com/ironpark/toolz/cli/mohae/internal/config"
	processutil "github.com/ironpark/toolz/cli/mohae/internal/process"
)

type AgentConfig = config.AgentConfig
type WorkspaceConfig = config.WorkspaceConfig
type MCPServerConfig = config.MCPServerConfig
type PromptEnv = config.PromptEnv
type LabeledPath = config.LabeledPath

const (
	DefaultConfigName = config.DefaultConfigName
	DefaultReportDir  = config.DefaultReportDir
)

// NewPromptEnv keeps the tests reading the host, which is what a trial with
// no container configured uses.
func NewPromptEnv(workspace string) PromptEnv {
	return config.NewPromptEnv(workspace, processutil.Host{})
}
