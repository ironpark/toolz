package claude

import "encoding/json"

// ---------------------------------------------------------------------------
// System prompt
// ---------------------------------------------------------------------------

// SystemPrompt configures the session's system prompt. Implementations are
// SystemPromptText, SystemPromptPreset and SystemPromptFile.
type SystemPrompt interface {
	isSystemPrompt()
}

// SystemPromptText replaces the system prompt with a custom string.
type SystemPromptText string

func (SystemPromptText) isSystemPrompt() {}

// SystemPromptPreset uses Claude Code's default system prompt, optionally with
// appended instructions.
type SystemPromptPreset struct {
	// Preset names the preset; empty means "claude_code".
	Preset string
	// Append is appended to the preset prompt.
	Append string
	// ExcludeDynamicSections strips per-user dynamic sections (working
	// directory, auto-memory, git status) so the prompt stays cacheable.
	ExcludeDynamicSections bool
}

func (*SystemPromptPreset) isSystemPrompt() {}

// SystemPromptFile loads the system prompt from a file.
type SystemPromptFile struct {
	Path string
}

func (*SystemPromptFile) isSystemPrompt() {}

// ---------------------------------------------------------------------------
// Tools and skills
// ---------------------------------------------------------------------------

// ToolsConfig selects the base set of built-in tools. Implementations are
// ToolList and ToolsPreset.
type ToolsConfig interface {
	isToolsConfig()
}

// ToolList enables exactly the named built-in tools. An empty, non-nil list
// disables all of them.
type ToolList []string

func (ToolList) isToolsConfig() {}

// ToolsPreset enables all default Claude Code tools.
type ToolsPreset struct{}

func (ToolsPreset) isToolsConfig() {}

// SkillsConfig selects which skills are enabled. Implementations are SkillList
// and SkillsAll.
type SkillsConfig interface {
	isSkillsConfig()
}

// SkillList enables exactly the named skills. An empty, non-nil list hides
// every skill.
type SkillList []string

func (SkillList) isSkillsConfig() {}

// SkillsAll enables every discovered skill.
type SkillsAll struct{}

func (SkillsAll) isSkillsConfig() {}

// ---------------------------------------------------------------------------
// MCP server configuration
// ---------------------------------------------------------------------------

// MCPServerConfig configures one MCP server. Implementations are
// MCPStdioServerConfig, MCPSSEServerConfig, MCPHTTPServerConfig and
// MCPSDKServerConfig.
type MCPServerConfig interface {
	isMCPServerConfig()
}

// MCPStdioServerConfig launches an MCP server as a subprocess.
type MCPStdioServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

func (*MCPStdioServerConfig) isMCPServerConfig() {}

// MarshalJSON emits the stdio server shape with its type discriminator.
func (c *MCPStdioServerConfig) MarshalJSON() ([]byte, error) {
	type alias MCPStdioServerConfig
	return json.Marshal(struct {
		Type string `json:"type"`
		*alias
	}{"stdio", (*alias)(c)})
}

// MCPSSEServerConfig connects to an MCP server over server-sent events.
type MCPSSEServerConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

func (*MCPSSEServerConfig) isMCPServerConfig() {}

// MarshalJSON emits the SSE server shape with its type discriminator.
func (c *MCPSSEServerConfig) MarshalJSON() ([]byte, error) {
	type alias MCPSSEServerConfig
	return json.Marshal(struct {
		Type string `json:"type"`
		*alias
	}{"sse", (*alias)(c)})
}

// MCPHTTPServerConfig connects to an MCP server over streamable HTTP.
type MCPHTTPServerConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

func (*MCPHTTPServerConfig) isMCPServerConfig() {}

// MarshalJSON emits the HTTP server shape with its type discriminator.
func (c *MCPHTTPServerConfig) MarshalJSON() ([]byte, error) {
	type alias MCPHTTPServerConfig
	return json.Marshal(struct {
		Type string `json:"type"`
		*alias
	}{"http", (*alias)(c)})
}

// MCPSDKServerConfig serves an in-process MCP server to the CLI over the
// control protocol. Build one with NewSDKMCPServer; only its name reaches the
// CLI configuration, the instance stays in this process.
type MCPSDKServerConfig struct {
	Name string
	// Instance is the in-process server that answers mcp_message requests.
	Instance *MCPServer
}

func (*MCPSDKServerConfig) isMCPServerConfig() {}

// MarshalJSON emits only the serializable fields; the instance is not sent.
func (c *MCPSDKServerConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"type": "sdk", "name": c.Name})
}

// ---------------------------------------------------------------------------
// Misc option value types
// ---------------------------------------------------------------------------

// SettingSource names one filesystem settings layer.
type SettingSource = string

// Supported setting sources.
const (
	SettingSourceUser    SettingSource = "user"
	SettingSourceProject SettingSource = "project"
	SettingSourceLocal   SettingSource = "local"
)

// EffortLevel controls how much effort the model puts into a response.
type EffortLevel = string

// Supported effort levels.
const (
	EffortLow    EffortLevel = "low"
	EffortMedium EffortLevel = "medium"
	EffortHigh   EffortLevel = "high"
	EffortXHigh  EffortLevel = "xhigh"
	EffortMax    EffortLevel = "max"
)

// SDKBeta names a beta feature header the CLI should send.
type SDKBeta = string

// SDKBetaContext1M enables the 1M token context window (Sonnet 4/4.5 only).
const SDKBetaContext1M SDKBeta = "context-1m-2025-08-07"

// ThinkingConfig controls the model's extended thinking behavior.
type ThinkingConfig struct {
	// Type is "adaptive", "enabled" or "disabled".
	Type string `json:"type"`
	// BudgetTokens sets a fixed thinking budget when Type is "enabled".
	BudgetTokens *int `json:"budget_tokens,omitempty"`
}

// TaskBudget is the API-side task budget in tokens.
type TaskBudget struct {
	Total int `json:"total"`
}

// PluginConfig loads a local plugin into the session.
type PluginConfig struct {
	// Type is always "local" today.
	Type string `json:"type"`
	Path string `json:"path"`
}

// AgentDefinition programmatically defines a subagent invokable via the Agent
// tool. Field names follow the CLI's camelCase wire format.
type AgentDefinition struct {
	Description     string   `json:"description"`
	Prompt          string   `json:"prompt"`
	Tools           []string `json:"tools,omitempty"`
	DisallowedTools []string `json:"disallowedTools,omitempty"`
	// Model is a model alias ("sonnet", "opus", "haiku", "inherit") or a
	// full model ID.
	Model  string   `json:"model,omitempty"`
	Skills []string `json:"skills,omitempty"`
	Memory string   `json:"memory,omitempty"`
	// MCPServers holds server names or inline {name: config} objects.
	MCPServers     []any  `json:"mcpServers,omitempty"`
	InitialPrompt  string `json:"initialPrompt,omitempty"`
	MaxTurns       *int   `json:"maxTurns,omitempty"`
	Background     *bool  `json:"background,omitempty"`
	Effort         any    `json:"effort,omitempty"`
	PermissionMode string `json:"permissionMode,omitempty"`
}

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

// DefaultMaxBufferSize is the default cap on a single line of CLI stdout.
const DefaultMaxBufferSize = 1024 * 1024

// Options configures a Query or a Client. The zero value is usable: it starts
// a default session with the CLI's own defaults for everything.
type Options struct {
	// Tools selects the base set of built-in tools. nil leaves the CLI
	// default in place.
	Tools ToolsConfig

	// AllowedTools names tools that run without prompting for permission.
	AllowedTools []string

	// DisallowedTools names tools removed from the model's context.
	DisallowedTools []string

	// SystemPrompt configures the system prompt. nil leaves the CLI default.
	SystemPrompt SystemPrompt

	// MCPServers configures MCP servers by name.
	MCPServers map[string]MCPServerConfig

	// MCPConfigPath points at an MCP config JSON file, used instead of
	// MCPServers when set.
	MCPConfigPath string

	// StrictMCPConfig ignores every MCP configuration the CLI would
	// otherwise load, using only MCPServers.
	StrictMCPConfig bool

	// PermissionMode selects how permission prompts are handled.
	PermissionMode PermissionMode

	// PermissionPromptToolName routes permission prompts through an MCP
	// tool. Mutually exclusive with CanUseTool.
	PermissionPromptToolName string

	// ContinueConversation resumes the most recent conversation in Cwd.
	ContinueConversation bool

	// Resume is the session ID to resume.
	Resume string

	// ResumeSessionAt truncates the resumed conversation after the entry
	// with this UUID.
	ResumeSessionAt string

	// ResumeDropsTurn is the UUID of the user prompt whose turn a
	// truncating resume intends to discard; the CLI validates it.
	ResumeDropsTurn string

	// ForkSession makes a resumed session fork to a new session ID.
	ForkSession bool

	// SessionID pins the session ID. Must be a valid UUID.
	SessionID string

	// MaxTurns caps the number of conversation turns.
	MaxTurns *int

	// MaxBudgetUSD caps the spend of the query.
	MaxBudgetUSD *float64

	// Model is the model to use; empty uses the CLI default.
	Model string

	// FallbackModel is used when the primary model is unavailable.
	FallbackModel string

	// Betas enables beta features.
	Betas []SDKBeta

	// Cwd is the working directory of the CLI subprocess.
	Cwd string

	// CLIPath is the path to the claude executable. Empty triggers
	// discovery on PATH and in the usual install locations.
	CLIPath string

	// Settings is a path to an extra settings JSON file, or an inline JSON
	// settings object, passed through to --settings.
	Settings string

	// SettingSources selects which filesystem settings layers to load. nil
	// loads all of them; a non-nil empty slice loads none.
	SettingSources *[]string

	// AddDirs are additional directories the CLI may access.
	AddDirs []string

	// Env holds extra environment variables for the subprocess.
	Env map[string]string

	// ExtraArgs passes additional CLI flags through. Keys omit the leading
	// dashes; a nil value makes the entry a boolean flag.
	ExtraArgs map[string]*string

	// MaxBufferSize caps a single line of CLI stdout. Zero means
	// DefaultMaxBufferSize.
	MaxBufferSize int

	// Stderr receives the subprocess's standard error, line by line.
	Stderr func(line string)

	// CanUseTool answers permission requests the CLI would otherwise show
	// to a user. Mutually exclusive with PermissionPromptToolName.
	CanUseTool CanUseTool

	// Hooks registers hook callbacks per event.
	Hooks map[HookEvent][]HookMatcher

	// Agents defines subagents invokable via the Agent tool.
	Agents map[string]AgentDefinition

	// User is an optional user identifier for the session.
	User string

	// IncludePartialMessages emits StreamEvent messages while the assistant
	// is streaming.
	IncludePartialMessages bool

	// IncludeHookEvents emits hook lifecycle events into the message
	// stream.
	IncludeHookEvents bool

	// ForwardSubagentText forwards subagent text and thinking blocks as
	// messages.
	ForwardSubagentText bool

	// Skills selects which skills are enabled. nil applies no SDK
	// configuration.
	Skills SkillsConfig

	// Sandbox holds sandbox settings, passed through as raw JSON.
	Sandbox json.RawMessage

	// Plugins loads local plugins.
	Plugins []PluginConfig

	// MaxThinkingTokens caps thinking tokens.
	//
	// Deprecated: use Thinking.
	MaxThinkingTokens *int

	// Thinking controls extended thinking. Takes precedence over
	// MaxThinkingTokens.
	Thinking *ThinkingConfig

	// Effort guides thinking depth.
	Effort EffortLevel

	// OutputFormat requests structured output, e.g.
	// {"type": "json_schema", "schema": {...}}.
	OutputFormat map[string]any

	// EnableFileCheckpointing lets Client.RewindFiles restore files to
	// their state at an earlier user message.
	EnableFileCheckpointing bool

	// TaskBudget makes the model aware of a remaining token budget.
	TaskBudget *TaskBudget

	// Transport replaces the CLI subprocess. It is meant for tests and for
	// embedding the SDK in a host that already owns the session; when nil,
	// a subprocess transport is built from these options.
	Transport Transport
}

// bufferSize reports the effective stdout line cap.
func (o *Options) bufferSize() int {
	if o == nil || o.MaxBufferSize <= 0 {
		return DefaultMaxBufferSize
	}
	return o.MaxBufferSize
}
