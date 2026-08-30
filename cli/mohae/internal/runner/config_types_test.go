package runner

import "github.com/ironpark/toolz/cli/mohae/internal/config"

type AgentConfig = config.AgentConfig
type WorkspaceConfig = config.WorkspaceConfig
type SkillConfig = config.SkillConfig
type MCPServerConfig = config.MCPServerConfig
type PromptEnv = config.PromptEnv
type LabeledPath = config.LabeledPath

const (
	DefaultConfigName = config.DefaultConfigName
	DefaultReportDir  = config.DefaultReportDir
)

func NewPromptEnv(workspace string) PromptEnv { return config.NewPromptEnv(workspace) }
