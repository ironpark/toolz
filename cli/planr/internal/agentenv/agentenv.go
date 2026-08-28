// Package agentenv identifies the AI coding agent, if any, that spawned the
// current process. planr uses it to describe the execution environment in its
// diagnostics and to expose it to repository hooks.
package agentenv

import (
	"os"
	"strings"
)

// Agent identifies an AI coding agent.
type Agent string

const (
	AgentUnknown Agent = ""

	AgentClaudeCode Agent = "claude-code"
	AgentCodex      Agent = "codex"
	AgentCursor     Agent = "cursor"
	AgentGeminiCLI  Agent = "gemini-cli"
	AgentGoose      Agent = "goose"
	AgentCopilot    Agent = "github-copilot"
	AgentAmp        Agent = "amp"
	AgentQwenCode   Agent = "qwen-code"
	AgentCline      Agent = "cline"
	AgentRooCode    Agent = "roo-code"
	AgentAugment    Agent = "augment"
)

// DetectionLevel describes the strength of an agent detection.
type DetectionLevel uint8

const (
	DetectionNone DetectionLevel = iota

	// DetectionAmbient means the process is running in an environment
	// associated with an agent, but may not have been spawned directly
	// by the agent's command execution tool.
	DetectionAmbient

	// DetectionDirect means a vendor-specific marker indicates that the
	// process was spawned by the agent's command execution environment.
	DetectionDirect
)

// String returns the canonical level name, which is also the value planr
// exports to hooks as PLANR_AGENT_LEVEL.
func (l DetectionLevel) String() string {
	switch l {
	case DetectionAmbient:
		return "ambient"
	case DetectionDirect:
		return "direct"
	default:
		return ""
	}
}

// Detection describes the detected AI-agent execution environment.
type Detection struct {
	Agent     Agent
	SessionID string
	Signal    string
	Level     DetectionLevel
}

// Detect inspects the current process environment and returns the strongest
// AI-agent detection available.
//
// Vendor-specific direct markers are preferred over generic and ambient ones.
func Detect() Detection {
	return DetectEnv(os.Getenv)
}

// DetectEnv performs detection using getenv.
//
// This is useful for tests and for callers that maintain their own
// environment source.
func DetectEnv(getenv func(string) string) Detection {
	// Claude Code.
	if getenv("CLAUDE_CODE_CHILD_SESSION") == "1" {
		return Detection{
			Agent:     AgentClaudeCode,
			SessionID: getenv("CLAUDE_CODE_SESSION_ID"),
			Signal:    "CLAUDE_CODE_CHILD_SESSION",
			Level:     DetectionDirect,
		}
	}

	// OpenAI Codex.
	if id := getenv("CODEX_THREAD_ID"); id != "" {
		return Detection{
			Agent:     AgentCodex,
			SessionID: id,
			Signal:    "CODEX_THREAD_ID",
			Level:     DetectionDirect,
		}
	}

	if getenv("CODEX_CI") == "1" {
		return Detection{
			Agent:  AgentCodex,
			Signal: "CODEX_CI",
			Level:  DetectionDirect,
		}
	}

	// Cursor.
	if getenv("CURSOR_AGENT") != "" {
		return Detection{
			Agent:     AgentCursor,
			SessionID: getenv("CURSOR_TRACE_ID"),
			Signal:    "CURSOR_AGENT",
			Level:     DetectionDirect,
		}
	}

	// Gemini CLI.
	if getenv("GEMINI_CLI") == "1" {
		return Detection{
			Agent:  AgentGeminiCLI,
			Signal: "GEMINI_CLI",
			Level:  DetectionDirect,
		}
	}

	// Goose.
	if getenv("GOOSE_TERMINAL") == "1" {
		return Detection{
			Agent:     AgentGoose,
			SessionID: getenv("AGENT_SESSION_ID"),
			Signal:    "GOOSE_TERMINAL",
			Level:     DetectionDirect,
		}
	}

	// GitHub Copilot CLI.
	if id := getenv("COPILOT_AGENT_SESSION_ID"); id != "" {
		return Detection{
			Agent:     AgentCopilot,
			SessionID: id,
			Signal:    "COPILOT_AGENT_SESSION_ID",
			Level:     DetectionDirect,
		}
	}

	// Amp.
	if id := getenv("AMP_CURRENT_THREAD_ID"); id != "" {
		return Detection{
			Agent:     AgentAmp,
			SessionID: id,
			Signal:    "AMP_CURRENT_THREAD_ID",
			Level:     DetectionDirect,
		}
	}

	// Qwen Code.
	if getenv("QWEN_CODE") != "" {
		return Detection{
			Agent:     AgentQwenCode,
			SessionID: getenv("QWEN_CODE_SESSION_ID"),
			Signal:    "QWEN_CODE",
			Level:     DetectionDirect,
		}
	}

	// Cline.
	if getenv("CLINE_ACTIVE") != "" {
		return Detection{
			Agent:     AgentCline,
			SessionID: getenv("CLINE_TASK_ID"),
			Signal:    "CLINE_ACTIVE",
			Level:     DetectionDirect,
		}
	}

	// Roo Code.
	if id := getenv("ROO_CODE_TASK_ID"); id != "" {
		return Detection{
			Agent:     AgentRooCode,
			SessionID: id,
			Signal:    "ROO_CODE_TASK_ID",
			Level:     DetectionDirect,
		}
	}

	// Augment.
	if getenv("AUGMENT_AGENT") != "" {
		return Detection{
			Agent:  AgentAugment,
			Signal: "AUGMENT_AGENT",
			Level:  DetectionDirect,
		}
	}

	// Generic conventions.
	//
	// These are deliberately lower priority because their propagation
	// semantics vary between agent implementations.
	if value := strings.TrimSpace(getenv("AI_AGENT")); value != "" {
		return Detection{
			Agent:     parseAgent(value),
			SessionID: genericSessionID(getenv),
			Signal:    "AI_AGENT",
			Level:     DetectionAmbient,
		}
	}

	if value := strings.TrimSpace(getenv("AGENT")); value != "" {
		return Detection{
			Agent:     parseAgent(value),
			SessionID: getenv("AGENT_SESSION_ID"),
			Signal:    "AGENT",
			Level:     DetectionAmbient,
		}
	}

	// CLAUDECODE has broader inheritance semantics than
	// CLAUDE_CODE_CHILD_SESSION, so treat it as ambient.
	if getenv("CLAUDECODE") == "1" {
		return Detection{
			Agent:     AgentClaudeCode,
			SessionID: getenv("CLAUDE_CODE_SESSION_ID"),
			Signal:    "CLAUDECODE",
			Level:     DetectionAmbient,
		}
	}

	return Detection{}
}

// Detected reports whether an AI-agent environment was detected.
func (d Detection) Detected() bool {
	return d.Level != DetectionNone
}

// Direct reports whether the current process appears to have been spawned
// directly by an agent's command execution environment.
func (d Detection) Direct() bool {
	return d.Level == DetectionDirect
}

// Ambient reports whether an agent environment was detected without a strong
// direct-execution marker.
func (d Detection) Ambient() bool {
	return d.Level == DetectionAmbient
}

// String returns the canonical agent name.
func (d Detection) String() string {
	return string(d.Agent)
}

func parseAgent(value string) Agent {
	value = strings.TrimSpace(value)

	// Support values such as:
	//
	//   goose@1.2.3
	//   claude-code@2.1.0
	if name, _, ok := strings.Cut(value, "@"); ok {
		value = name
	}

	value = strings.ToLower(strings.TrimSpace(value))

	switch value {
	case "claude", "claude-code":
		return AgentClaudeCode

	case "codex", "openai-codex":
		return AgentCodex

	case "cursor", "cursor-cli", "cursor-agent":
		return AgentCursor

	case "gemini", "gemini-cli":
		return AgentGeminiCLI

	case "goose":
		return AgentGoose

	case "copilot", "github-copilot", "github-copilot-cli":
		return AgentCopilot

	case "amp":
		return AgentAmp

	case "qwen", "qwen-code":
		return AgentQwenCode

	case "cline":
		return AgentCline

	case "roo", "roo-code":
		return AgentRooCode

	case "augment":
		return AgentAugment

	default:
		return Agent(value)
	}
}

func genericSessionID(getenv func(string) string) string {
	for _, key := range [...]string{
		"AGENT_SESSION_ID",
		"CLAUDE_CODE_SESSION_ID",
		"CODEX_THREAD_ID",
		"COPILOT_AGENT_SESSION_ID",
		"AMP_CURRENT_THREAD_ID",
		"QWEN_CODE_SESSION_ID",
		"CLINE_TASK_ID",
		"ROO_CODE_TASK_ID",
	} {
		if value := getenv(key); value != "" {
			return value
		}
	}

	return ""
}
