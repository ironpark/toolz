// Package agent defines the agent kinds mohae supports and the workspace
// conventions shared by configuration, runners, and drivers.
package agent

import "slices"

const (
	ClaudeCode = "claude-code"
	Codex      = "codex"
	CustomCLI  = "custom-cli"
)

var skillDirs = map[string]string{
	ClaudeCode: ".claude/skills",
	Codex:      ".codex/skills",
	CustomCLI:  ".agent/skills",
}

// KnownTypes is stable for diagnostics and command output.
var KnownTypes = []string{ClaudeCode, Codex, CustomCLI}

// IsKnown reports whether name identifies a supported agent kind.
func IsKnown(name string) bool { return slices.Contains(KnownTypes, name) }

// SkillDir returns the workspace-relative skill directory for an agent kind.
func SkillDir(name string) (string, bool) {
	directory, ok := skillDirs[name]
	return directory, ok
}
