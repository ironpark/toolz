package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

// DefaultConfigName is the file `run` and `verify` fall back to when no path is
// given, so the common case is a bare `mohae run` inside a project.
const DefaultConfigName = "mohae.config.yaml"

// DefaultReportDir keeps reports beside the project rather than in a user-wide
// directory: a trial only means something next to the configuration that
// produced it.
const DefaultReportDir = ".mohae/reports"

// Config is one trial environment: what to run the agent on, what to tell it,
// and how to decide afterwards whether it worked.
//
// Every path is resolved relative to the configuration file, not to the working
// directory, so a config can be run from anywhere.
type Config struct {
	// Path is where this config was read from. It is not part of the file.
	Path string `yaml:"-"`

	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`

	Agent     AgentConfig     `yaml:"agent"`
	Workspace WorkspaceConfig `yaml:"workspace"`
	Prompt    PromptConfig    `yaml:"prompt"`
	MCP       MCPConfig       `yaml:"mcp,omitempty"`
	TargetCLI TargetCLIConfig `yaml:"target_cli,omitempty"`
	Verify    VerifyConfig    `yaml:"verify,omitempty"`
	Limits    LimitsConfig    `yaml:"limits,omitempty"`
	Report    ReportConfig    `yaml:"report,omitempty"`
}

// AgentConfig selects the agent under test. `type` names a built-in driver;
// `command` is only read by the custom-cli driver, which is what lets a tool
// mohae has never heard of be evaluated on the same terms as a built-in one.
type AgentConfig struct {
	Type      string            `yaml:"type"`
	Model     string            `yaml:"model,omitempty"`
	Reasoning string            `yaml:"reasoning,omitempty"`
	Command   []string          `yaml:"command,omitempty"`
	Env       map[string]string `yaml:"env,omitempty"`
}

// WorkspaceConfig describes the repository the agent works in. It is copied to
// an isolated directory before every trial, so a run can never modify the
// source and two runs of the same config start from identical state.
type WorkspaceConfig struct {
	Source     string `yaml:"source"`
	InitScript string `yaml:"init_script,omitempty"`
	// AgentMD is installed under the name the agent expects (AGENTS.md).
	// Keeping it outside the workspace source means one document can be shared
	// by every config instead of copied into each fixture.
	AgentMD string `yaml:"agent_md,omitempty"`
	Git     bool   `yaml:"git,omitempty"`
}

// PromptConfig is the first and only user message. It is deliberately not
// placed in the workspace: the agent has to work from the conversation, not
// from a task file it can re-read on disk.
type PromptConfig struct {
	Text string `yaml:"text,omitempty"`
	File string `yaml:"file,omitempty"`
}

type MCPConfig struct {
	Config string `yaml:"config,omitempty"`
}

// TargetCLIConfig is the tool the agent is expected to reach for. Build runs
// before the trial so the agent gets the current source, not whatever happens
// to be installed on the machine.
type TargetCLIConfig struct {
	Command string `yaml:"command,omitempty"`
	Build   string `yaml:"build,omitempty"`
	BinDir  string `yaml:"bin_dir,omitempty"`
}

// VerifyConfig grades the finished workspace. The script runs outside the
// workspace so grading cannot leave files behind that would be mistaken for the
// agent's work, and it is never copied in, so the agent cannot tailor its
// output to the checks.
type VerifyConfig struct {
	Script string `yaml:"script,omitempty"`
}

type LimitsConfig struct {
	TimeoutSeconds int `yaml:"timeout_seconds,omitempty"`
	MaxTurns       int `yaml:"max_turns,omitempty"`
}

type ReportConfig struct {
	Dir     string   `yaml:"dir,omitempty"`
	Formats []string `yaml:"formats,omitempty"`
}

// KnownAgentTypes are the drivers the runner can select. custom-cli covers any
// agent with a non-interactive command line.
var KnownAgentTypes = []string{"claude-code", "codex", "custom-cli"}

// KnownFormats are the report renderings.
var KnownFormats = []string{"terminal", "json", "markdown", "html"}

// LoadConfig reads and validates one configuration file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	config := &Config{}
	// Strict decoding: a misspelled key is the kind of mistake that would
	// otherwise be discovered as a trial that silently measured the defaults.
	if err := yaml.UnmarshalWithOptions(data, config, yaml.Strict()); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	config.Path = absolute
	config.applyDefaults()
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return config, nil
}

func (c *Config) applyDefaults() {
	if c.Name == "" {
		c.Name = strings.TrimSuffix(filepath.Base(c.Path), filepath.Ext(c.Path))
		c.Name = strings.TrimSuffix(c.Name, ".config")
	}
	if c.Limits.TimeoutSeconds == 0 {
		c.Limits.TimeoutSeconds = DefaultTimeoutSeconds
	}
	if c.Limits.MaxTurns == 0 {
		c.Limits.MaxTurns = DefaultMaxTurns
	}
	if c.Report.Dir == "" {
		c.Report.Dir = DefaultReportDir
	}
	if len(c.Report.Formats) == 0 {
		c.Report.Formats = []string{"terminal"}
	}
}

// Validate reports what a trial cannot proceed without. It checks the shape of
// the configuration only; whether the referenced files exist is `verify`'s job,
// so a config can be written before the scripts it points at.
func (c *Config) Validate() error {
	if c.Agent.Type == "" {
		return fmt.Errorf("agent.type is required (one of: %s)", strings.Join(KnownAgentTypes, ", "))
	}
	if !contains(KnownAgentTypes, c.Agent.Type) {
		return fmt.Errorf("unknown agent.type %q (one of: %s)", c.Agent.Type, strings.Join(KnownAgentTypes, ", "))
	}
	if c.Agent.Type == "custom-cli" && len(c.Agent.Command) == 0 {
		return fmt.Errorf("agent.command is required when agent.type is custom-cli")
	}
	if c.Workspace.Source == "" {
		return fmt.Errorf("workspace.source is required")
	}
	if c.Prompt.Text == "" && c.Prompt.File == "" {
		return fmt.Errorf("prompt.text or prompt.file is required")
	}
	if c.Prompt.Text != "" && c.Prompt.File != "" {
		// Silently preferring one would make a trial measure a prompt nobody
		// meant to send.
		return fmt.Errorf("prompt.text and prompt.file are mutually exclusive")
	}
	for _, format := range c.Report.Formats {
		if !contains(KnownFormats, format) {
			return fmt.Errorf("unknown report format %q (one of: %s)", format, strings.Join(KnownFormats, ", "))
		}
	}
	if c.Limits.TimeoutSeconds < 0 {
		return fmt.Errorf("limits.timeout_seconds must not be negative")
	}
	if c.Limits.MaxTurns < 0 {
		return fmt.Errorf("limits.max_turns must not be negative")
	}
	return nil
}

// Resolve turns a path from the configuration file into an absolute one.
// Relative paths are read against the file's own directory so a config keeps
// working whatever directory `mohae` is invoked from.
func (c *Config) Resolve(path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(filepath.Dir(c.Path), path)
}

// ReferencedPaths lists every file the configuration points at, labelled by the
// field that named it, for `verify` to check and for `run` to report on.
func (c *Config) ReferencedPaths() []LabeledPath {
	candidates := []LabeledPath{
		{"workspace.source", c.Workspace.Source},
		{"workspace.init_script", c.Workspace.InitScript},
		{"workspace.agent_md", c.Workspace.AgentMD},
		{"prompt.file", c.Prompt.File},
		{"mcp.config", c.MCP.Config},
		{"verify.script", c.Verify.Script},
	}
	paths := make([]LabeledPath, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Path == "" {
			continue
		}
		candidate.Path = c.Resolve(candidate.Path)
		paths = append(paths, candidate)
	}
	return paths
}

// LabeledPath is a path together with the configuration field that named it, so
// a diagnostic can say which key to fix rather than only which file is missing.
type LabeledPath struct {
	Field string
	Path  string
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
