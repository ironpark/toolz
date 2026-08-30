package main

import (
	"fmt"
	"sort"
	"strings"
)

// Profile is a named subset of a configuration. Selecting one overwrites the
// base config with the sections the profile declares, so one file can describe
// several variants of the same trial — a different agent, tighter limits — and
// `--profile` picks between them instead of the file being edited per run.
//
// A section present in the profile replaces the base section wholesale. Merging
// field by field was considered and rejected: a zero value ("", false, empty
// list) would be indistinguishable from an omitted one, so a profile could
// never turn a setting off.
type Profile struct {
	Agent     *AgentConfig      `yaml:"agent,omitempty"`
	Workspace *WorkspaceConfig  `yaml:"workspace,omitempty"`
	Prompts   []Prompt          `yaml:"prompts,omitempty"`
	Skills    []SkillConfig     `yaml:"skills,omitempty"`
	MCP       []MCPServerConfig `yaml:"mcp,omitempty"`
	Hooks     *HooksConfig      `yaml:"hooks,omitempty"`
	Verify    *VerifyConfig     `yaml:"verify,omitempty"`
	Artifacts []string          `yaml:"artifacts,omitempty"`
	Limits    *LimitsConfig     `yaml:"limits,omitempty"`
	Report    *ReportConfig     `yaml:"report,omitempty"`
}

// ApplyProfile overwrites the configuration with the named profile's sections.
// Applying several profiles in order layers them, later ones winning.
func (c *Config) ApplyProfile(name string) error {
	profile, ok := c.Profiles[name]
	if !ok {
		names := make([]string, 0, len(c.Profiles))
		for known := range c.Profiles {
			names = append(names, known)
		}
		sort.Strings(names)
		if len(names) == 0 {
			return fmt.Errorf("%s: no profiles are defined, so --profile %s selects nothing", c.Path, name)
		}
		return fmt.Errorf("%s: unknown profile %q (one of: %s)", c.Path, name, strings.Join(names, ", "))
	}
	if profile.Agent != nil {
		c.Agent = *profile.Agent
	}
	if profile.Workspace != nil {
		c.Workspace = *profile.Workspace
	}
	if profile.Prompts != nil {
		c.Prompts = profile.Prompts
	}
	if profile.Skills != nil {
		c.Skills = profile.Skills
	}
	if profile.MCP != nil {
		c.MCP = profile.MCP
	}
	if profile.Hooks != nil {
		c.Hooks = *profile.Hooks
	}
	if profile.Verify != nil {
		c.Verify = *profile.Verify
	}
	if profile.Artifacts != nil {
		c.Artifacts = profile.Artifacts
	}
	if profile.Limits != nil {
		c.Limits = *profile.Limits
	}
	if profile.Report != nil {
		c.Report = *profile.Report
	}
	// A profile may leave defaults empty again (a replaced limits section with
	// no timeout, a report section with no dir), so they are refilled the same
	// way loading does.
	c.applyDefaults()
	return nil
}
