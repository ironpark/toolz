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
	// Prompts is the conversation, in order. More than one makes the trial
	// multi-turn; each entry may carry a `when` condition, so the same
	// configuration can describe follow-ups that only some runs need.
	Prompts []Prompt          `yaml:"prompts"`
	Skills  []SkillConfig     `yaml:"skills,omitempty"`
	MCP     []MCPServerConfig `yaml:"mcp,omitempty"`
	Verify  VerifyConfig      `yaml:"verify,omitempty"`
	Limits  LimitsConfig `yaml:"limits,omitempty"`
	Report  ReportConfig `yaml:"report,omitempty"`
}

// AgentConfig selects the agent under test. `type` names a built-in driver;
// `command` is only read by the custom-cli driver, which is what lets a tool
// mohae has never heard of be evaluated on the same terms as a built-in one.
type AgentConfig struct {
	Type      string            `yaml:"type"`
	Model     string            `yaml:"model,omitempty"`
	Effort    string            `yaml:"effort,omitempty"`
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

// SkillConfig installs one skill into the workspace before the trial starts.
// Agents limits which agent types see it; an empty list enables it for all,
// so the common single-agent config never has to repeat the agent's name.
type SkillConfig struct {
	Path   string   `yaml:"path"`
	Agents []string `yaml:"agents,omitempty"`
}

// EnabledFor reports whether this skill applies to the given agent type.
func (s SkillConfig) EnabledFor(agentType string) bool {
	return len(s.Agents) == 0 || contains(s.Agents, agentType)
}

// MCPServerConfig connects one MCP server to the trial. Agents limits which
// agent types it is offered to; an empty list offers it to all.
type MCPServerConfig struct {
	Name   string   `yaml:"name,omitempty"`
	Config string   `yaml:"config"`
	Agents []string `yaml:"agents,omitempty"`
}

// EnabledFor reports whether this server applies to the given agent type.
func (m MCPServerConfig) EnabledFor(agentType string) bool {
	return len(m.Agents) == 0 || contains(m.Agents, agentType)
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
	if len(c.Prompts) == 0 {
		return fmt.Errorf("prompts is required and must list at least one prompt")
	}
	// Dependencies may only name prompts defined earlier: the conversation is
	// sent in order, so a forward or self reference could never be satisfied
	// and would silently skip the prompt on every run.
	ids := map[string]int{}
	for index := range c.Prompts {
		field := fmt.Sprintf("prompts[%d]", index)
		if err := c.Prompts[index].Validate(field); err != nil {
			return err
		}
		for _, id := range c.Prompts[index].DependsOn {
			if _, ok := ids[id]; !ok {
				return fmt.Errorf("%s.depends_on: %q does not name an earlier prompt's id", field, id)
			}
		}
		if id := c.Prompts[index].ID; id != "" {
			if previous, ok := ids[id]; ok {
				return fmt.Errorf("%s.id: %q is already used by prompts[%d]", field, id, previous)
			}
			ids[id] = index
		}
	}
	for index, skill := range c.Skills {
		if skill.Path == "" {
			return fmt.Errorf("skills[%d].path is required", index)
		}
		if err := validateAgents(fmt.Sprintf("skills[%d]", index), skill.Agents); err != nil {
			return err
		}
	}
	for index, server := range c.MCP {
		if server.Config == "" {
			return fmt.Errorf("mcp[%d].config is required", index)
		}
		if err := validateAgents(fmt.Sprintf("mcp[%d]", index), server.Agents); err != nil {
			return err
		}
	}
	for _, format := range c.Report.Formats {
		if !contains(KnownFormats, format) {
			return fmt.Errorf("unknown report format %q (one of: %s)", format, strings.Join(KnownFormats, ", "))
		}
	}
	if c.Limits.TimeoutSeconds < 0 {
		return fmt.Errorf("limits.timeout_seconds must not be negative")
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
		{"verify.script", c.Verify.Script},
	}
	for index, skill := range c.Skills {
		candidates = append(candidates, LabeledPath{fmt.Sprintf("skills[%d].path", index), skill.Path})
	}
	for index, server := range c.MCP {
		candidates = append(candidates, LabeledPath{fmt.Sprintf("mcp[%d].config", index), server.Config})
	}
	for index, prompt := range c.Prompts {
		candidates = append(candidates, LabeledPath{fmt.Sprintf("prompts[%d].file", index), prompt.File})
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

// validateAgents rejects an agents list naming a driver that does not exist,
// which would otherwise read as an item silently enabled for nobody.
func validateAgents(field string, agents []string) error {
	for _, agent := range agents {
		if !contains(KnownAgentTypes, agent) {
			return fmt.Errorf("%s.agents: unknown agent type %q (one of: %s)", field, agent, strings.Join(KnownAgentTypes, ", "))
		}
	}
	return nil
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
