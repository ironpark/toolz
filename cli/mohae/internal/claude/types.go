package claude

import (
	"context"
	"encoding/json"
)

// ---------------------------------------------------------------------------
// Content blocks
// ---------------------------------------------------------------------------

// ContentBlock is one block inside a message's content array. The set of
// implementations is closed: TextBlock, ThinkingBlock, ToolUseBlock,
// ToolResultBlock, ServerToolUseBlock and ServerToolResultBlock.
type ContentBlock interface {
	isContentBlock()
	// BlockType reports the wire discriminator of the block.
	BlockType() string
}

// TextBlock is a plain text content block.
type TextBlock struct {
	Text string `json:"text"`
}

func (*TextBlock) isContentBlock()   {}
func (*TextBlock) BlockType() string { return "text" }

// ThinkingBlock is an extended-thinking content block.
type ThinkingBlock struct {
	Thinking  string `json:"thinking"`
	Signature string `json:"signature"`
}

func (*ThinkingBlock) isContentBlock()   {}
func (*ThinkingBlock) BlockType() string { return "thinking" }

// ToolUseBlock records a tool invocation requested by the model.
type ToolUseBlock struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

func (*ToolUseBlock) isContentBlock()   {}
func (*ToolUseBlock) BlockType() string { return "tool_use" }

// ToolResultBlock carries the result of a tool invocation. The CLI sends the
// content either as a plain string or as a list of nested content dicts, so
// exactly one of ContentText and ContentList is set (both may be nil/empty when
// the CLI omitted the field).
type ToolResultBlock struct {
	ToolUseID   string           `json:"tool_use_id"`
	ContentText *string          `json:"-"`
	ContentList []map[string]any `json:"-"`
	IsError     *bool            `json:"is_error,omitempty"`
}

func (*ToolResultBlock) isContentBlock()   {}
func (*ToolResultBlock) BlockType() string { return "tool_result" }

// ServerToolName enumerates the server-side tools the API may run on the
// model's behalf. Newer CLI versions may report names not listed here.
type ServerToolName = string

// Known server-side tool names.
const (
	ServerToolAdvisor                 ServerToolName = "advisor"
	ServerToolWebSearch               ServerToolName = "web_search"
	ServerToolWebFetch                ServerToolName = "web_fetch"
	ServerToolCodeExecution           ServerToolName = "code_execution"
	ServerToolBashCodeExecution       ServerToolName = "bash_code_execution"
	ServerToolTextEditorCodeExecution ServerToolName = "text_editor_code_execution"
	ServerToolSearchToolRegex         ServerToolName = "tool_search_tool_regex"
	ServerToolSearchToolBM25          ServerToolName = "tool_search_tool_bm25"
)

// ServerToolUseBlock is a server-side tool invocation. The caller never needs
// to return a result for one.
type ServerToolUseBlock struct {
	ID    string         `json:"id"`
	Name  ServerToolName `json:"name"`
	Input map[string]any `json:"input"`
}

func (*ServerToolUseBlock) isContentBlock()   {}
func (*ServerToolUseBlock) BlockType() string { return "server_tool_use" }

// ServerToolResultBlock is the result of a server-side tool call. Content is
// passed through from the API verbatim.
type ServerToolResultBlock struct {
	ToolUseID string         `json:"tool_use_id"`
	Content   map[string]any `json:"content"`
}

func (*ServerToolResultBlock) isContentBlock()   {}
func (*ServerToolResultBlock) BlockType() string { return "server_tool_result" }

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

// Message is one item of the stream produced by Query or Client. The set of
// implementations is closed: UserMessage, AssistantMessage, SystemMessage (and
// its task/hook specializations), ResultMessage, StreamEvent, RateLimitEvent
// and ConversationResetMessage.
type Message interface {
	isMessage()
}

// MessageOriginKind enumerates the known values of MessageOrigin.Kind. Newer
// CLI versions may emit kinds not listed here; treat anything unrecognized as
// "not human".
type MessageOriginKind = string

// Known message origin kinds.
const (
	OriginHuman            MessageOriginKind = "human"
	OriginChannel          MessageOriginKind = "channel"
	OriginPeer             MessageOriginKind = "peer"
	OriginTaskNotification MessageOriginKind = "task-notification"
	OriginCoordinator      MessageOriginKind = "coordinator"
	OriginUnclassified     MessageOriginKind = "unclassified"
	OriginObserver         MessageOriginKind = "observer"
	OriginAutoContinuation MessageOriginKind = "auto-continuation"
	OriginObserverActivity MessageOriginKind = "observer-activity"
)

// MessageOrigin describes the provenance of a user-role message and, on a
// ResultMessage, of the message that triggered the turn. Only Kind is always
// present; which of the remaining fields are set depends on Kind, and Extra
// carries any keys this SDK version does not model.
type MessageOrigin struct {
	// Kind classifies the sender. It falls back to OriginUnclassified when the
	// CLI sends a kind this SDK cannot read as a string.
	Kind MessageOriginKind `json:"kind"`
	// Server names the MCP server that delivered the message ("channel").
	Server string `json:"server,omitempty"`
	// From is the sender's address, such as "agent://name" ("channel", "peer").
	From string `json:"from,omitempty"`
	// Name is the sender's display name, when it has one.
	Name string `json:"name,omitempty"`
	// FromSession is the sender's session ID ("peer", "coordinator").
	FromSession string `json:"fromSession,omitempty"`
	// SenderTaskID is the task that sent the message ("peer",
	// "task-notification").
	SenderTaskID string `json:"senderTaskId,omitempty"`
	// Body is the original message text, when the delivered content wraps it
	// ("channel", "peer").
	Body string `json:"body,omitempty"`
	// VerifiedPeerPID is the OS process ID of the peer, as verified by the CLI
	// ("peer"). It is 0 when the CLI reported no verified PID.
	VerifiedPeerPID int `json:"verifiedPeerPid,omitempty"`
	// Subkind narrows Kind, for kinds that classify further.
	Subkind string `json:"subkind,omitempty"`
	// Extra holds origin keys this SDK version does not model, so a newer CLI
	// loses nothing. It is merged back in on marshal; modeled fields win.
	Extra map[string]any `json:"-"`
}

// originModeledKeys are the origin keys MessageOrigin has a field for.
// Anything else goes to Extra.
var originModeledKeys = map[string]bool{
	"kind": true, "server": true, "from": true, "name": true,
	"fromSession": true, "senderTaskId": true, "body": true,
	"verifiedPeerPid": true, "subkind": true,
}

// MarshalJSON writes the modeled fields with any Extra keys merged alongside.
func (o MessageOrigin) MarshalJSON() ([]byte, error) {
	type alias MessageOrigin
	b, err := json.Marshal(alias(o))
	if err != nil || len(o.Extra) == 0 {
		return b, err
	}
	var merged map[string]any
	if err := json.Unmarshal(b, &merged); err != nil {
		return nil, err
	}
	for k, v := range o.Extra {
		if _, taken := merged[k]; !taken {
			merged[k] = v
		}
	}
	return json.Marshal(merged)
}

// UnmarshalJSON reads the modeled fields and collects the rest into Extra.
func (o *MessageOrigin) UnmarshalJSON(b []byte) error {
	type alias MessageOrigin
	var a alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*o = MessageOrigin(a)
	for k, v := range raw {
		if !originModeledKeys[k] {
			o.putExtra(k, v)
		}
	}
	return nil
}

// putExtra records an unmodeled origin key, allocating Extra on first use.
func (o *MessageOrigin) putExtra(key string, value any) {
	if o.Extra == nil {
		o.Extra = make(map[string]any)
	}
	o.Extra[key] = value
}

// UserMessage is a user-role message. The CLI delivers its content either as a
// plain string or as a block array; ContentText is set for the former and
// Content for the latter.
type UserMessage struct {
	ContentText     *string
	Content         []ContentBlock
	UUID            string
	ParentToolUseID string
	ToolUseResult   map[string]any
	Origin          *MessageOrigin
	SessionID       string
}

func (*UserMessage) isMessage() {}

// AssistantMessageError enumerates the known error kinds on an assistant
// message.
type AssistantMessageError = string

// Known assistant message error kinds.
const (
	AssistantErrorAuthenticationFailed AssistantMessageError = "authentication_failed"
	AssistantErrorBilling              AssistantMessageError = "billing_error"
	AssistantErrorRateLimit            AssistantMessageError = "rate_limit"
	AssistantErrorInvalidRequest       AssistantMessageError = "invalid_request"
	AssistantErrorServer               AssistantMessageError = "server_error"
	AssistantErrorUnknown              AssistantMessageError = "unknown"
)

// AssistantMessage is an assistant-role message with its content blocks.
type AssistantMessage struct {
	Content         []ContentBlock
	Model           string
	ParentToolUseID string
	Error           AssistantMessageError
	Usage           map[string]any
	MessageID       string
	StopReason      string
	SessionID       string
	UUID            string
}

func (*AssistantMessage) isMessage() {}

// SystemMessage is a metadata message. Data holds the full raw payload,
// including fields not modeled by the specialized subtypes below.
type SystemMessage struct {
	Subtype string         `json:"subtype"`
	Data    map[string]any `json:"data"`
}

func (*SystemMessage) isMessage() {}

// TaskUsage reports usage statistics on task progress and notification
// messages.
type TaskUsage struct {
	TotalTokens int `json:"total_tokens"`
	ToolUses    int `json:"tool_uses"`
	DurationMS  int `json:"duration_ms"`
}

// TerminalTaskStatuses lists the task statuses that mean the task has finished.
// It spans both lifecycle vocabularies: task_notification reports "stopped"
// while task_updated reports the raw "killed".
var TerminalTaskStatuses = map[string]bool{
	"completed": true,
	"failed":    true,
	"stopped":   true,
	"killed":    true,
}

// TaskStartedMessage is emitted when a task starts. It embeds SystemMessage, so
// a type switch on *SystemMessage does not match it — switch on the concrete
// type or use Data for a uniform view.
type TaskStartedMessage struct {
	SystemMessage
	TaskID      string
	Description string
	UUID        string
	SessionID   string
	ToolUseID   string
	TaskType    string
}

// TaskProgressMessage is emitted while a task is in progress.
type TaskProgressMessage struct {
	SystemMessage
	TaskID       string
	Description  string
	Usage        TaskUsage
	UUID         string
	SessionID    string
	ToolUseID    string
	LastToolName string
}

// TaskNotificationMessage is emitted when a task completes, fails or is
// stopped. Not every terminal task emits one: a background task may report
// completion only through a TaskUpdatedMessage with a terminal patch status.
type TaskNotificationMessage struct {
	SystemMessage
	TaskID     string
	Status     string
	OutputFile string
	Summary    string
	UUID       string
	SessionID  string
	ToolUseID  string
	Usage      *TaskUsage
}

// TaskUpdatedMessage is emitted when a background task's state changes. Patch
// carries the changed fields.
type TaskUpdatedMessage struct {
	SystemMessage
	TaskID    string
	Patch     map[string]any
	Status    string
	SessionID string
	UUID      string
}

// HookEventMessage is a hook lifecycle event, emitted when
// Options.IncludeHookEvents is set.
type HookEventMessage struct {
	SystemMessage
	HookEventName string
	SessionID     string
	UUID          string
}

// DeferredToolUse is a tool call deferred by a PreToolUse hook that returned
// the "defer" permission decision.
type DeferredToolUse struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

// ModelUsage is the per-model token and cost breakdown reported on a result
// message. Field names follow the CLI's camelCase wire format.
type ModelUsage struct {
	InputTokens              int     `json:"inputTokens"`
	OutputTokens             int     `json:"outputTokens"`
	CacheReadInputTokens     int     `json:"cacheReadInputTokens"`
	CacheCreationInputTokens int     `json:"cacheCreationInputTokens"`
	WebSearchRequests        int     `json:"webSearchRequests"`
	CostUSD                  float64 `json:"costUSD"`
	ContextWindow            int     `json:"contextWindow"`
	MaxOutputTokens          int     `json:"maxOutputTokens"`
	CanonicalModel           string  `json:"canonicalModel,omitempty"`
	Provider                 string  `json:"provider,omitempty"`
}

// ResultMessage terminates a turn and reports its cost, usage and outcome.
type ResultMessage struct {
	Subtype           string
	DurationMS        int
	DurationAPIMS     int
	IsError           bool
	NumTurns          int
	SessionID         string
	StopReason        string
	TotalCostUSD      *float64
	Usage             map[string]any
	Result            string
	StructuredOutput  json.RawMessage
	ModelUsage        map[string]ModelUsage
	PermissionDenials []map[string]any
	DeferredToolUse   *DeferredToolUse
	Errors            []string
	APIErrorStatus    *int
	UUID              string
	TerminalReason    string
	Origin            *MessageOrigin
	// Data is the raw result payload, retained so ResultError can report it.
	Data map[string]any
}

func (*ResultMessage) isMessage() {}

// StreamEvent carries a partial-message update from the streaming API. Event is
// the raw Anthropic API stream event.
type StreamEvent struct {
	UUID            string
	SessionID       string
	Event           map[string]any
	ParentToolUseID string
}

func (*StreamEvent) isMessage() {}

// RateLimitStatus enumerates the rate limit states the CLI reports.
type RateLimitStatus = string

// Known rate limit statuses.
const (
	RateLimitAllowed        RateLimitStatus = "allowed"
	RateLimitAllowedWarning RateLimitStatus = "allowed_warning"
	RateLimitRejected       RateLimitStatus = "rejected"
)

// RateLimitInfo describes the rate limit state at the moment it changed.
type RateLimitInfo struct {
	Status                RateLimitStatus
	ResetsAt              *int64
	RateLimitType         string
	Utilization           *float64
	OverageStatus         RateLimitStatus
	OverageResetsAt       *int64
	OverageDisabledReason string
	// Raw is the full dict from the CLI, including unmodeled fields.
	Raw map[string]any
}

// RateLimitEvent is emitted when the rate limit status transitions.
type RateLimitEvent struct {
	RateLimitInfo RateLimitInfo
	UUID          string
	SessionID     string
}

func (*RateLimitEvent) isMessage() {}

// ConversationResetMessage is emitted when the session's conversation is
// replaced without ending the connection, e.g. after /clear. Running totals on
// subsequent result messages restart from zero.
type ConversationResetMessage struct {
	NewConversationID string
	UUID              string
	SessionID         string
}

func (*ConversationResetMessage) isMessage() {}

// ---------------------------------------------------------------------------
// Permissions
// ---------------------------------------------------------------------------

// PermissionMode selects how the session handles permission prompts.
type PermissionMode = string

// Supported permission modes.
const (
	PermissionModeDefault           PermissionMode = "default"
	PermissionModeAcceptEdits       PermissionMode = "acceptEdits"
	PermissionModePlan              PermissionMode = "plan"
	PermissionModeBypassPermissions PermissionMode = "bypassPermissions"
	PermissionModeDontAsk           PermissionMode = "dontAsk"
	PermissionModeAuto              PermissionMode = "auto"
)

// PermissionUpdateDestination selects where a permission update is persisted.
type PermissionUpdateDestination = string

// Supported permission update destinations.
const (
	DestinationUserSettings    PermissionUpdateDestination = "userSettings"
	DestinationProjectSettings PermissionUpdateDestination = "projectSettings"
	DestinationLocalSettings   PermissionUpdateDestination = "localSettings"
	DestinationSession         PermissionUpdateDestination = "session"
)

// PermissionBehavior is the effect of a permission rule.
type PermissionBehavior = string

// Supported permission behaviors.
const (
	BehaviorAllow PermissionBehavior = "allow"
	BehaviorDeny  PermissionBehavior = "deny"
	BehaviorAsk   PermissionBehavior = "ask"
)

// PermissionRuleValue names a tool and, optionally, the rule content that
// narrows the match (e.g. a Bash command prefix).
type PermissionRuleValue struct {
	ToolName    string  `json:"toolName"`
	RuleContent *string `json:"ruleContent"`
}

// Permission update kinds.
const (
	PermissionUpdateAddRules         = "addRules"
	PermissionUpdateReplaceRules     = "replaceRules"
	PermissionUpdateRemoveRules      = "removeRules"
	PermissionUpdateSetMode          = "setMode"
	PermissionUpdateAddDirectories   = "addDirectories"
	PermissionUpdateRemoveDirectorie = "removeDirectories"
)

// PermissionUpdate is one change to the session's permission configuration.
// Only the fields relevant to Type are put on the wire.
type PermissionUpdate struct {
	Type        string
	Rules       []PermissionRuleValue
	Behavior    PermissionBehavior
	Mode        PermissionMode
	Directories []string
	Destination PermissionUpdateDestination
}

// MarshalJSON emits the control-protocol shape, matching the TypeScript SDK.
func (u PermissionUpdate) MarshalJSON() ([]byte, error) {
	out := map[string]any{"type": u.Type}
	if u.Destination != "" {
		out["destination"] = u.Destination
	}
	switch u.Type {
	case PermissionUpdateAddRules, PermissionUpdateReplaceRules, PermissionUpdateRemoveRules:
		if u.Rules != nil {
			rules := make([]map[string]any, 0, len(u.Rules))
			for _, r := range u.Rules {
				rules = append(rules, map[string]any{
					"toolName":    r.ToolName,
					"ruleContent": r.RuleContent,
				})
			}
			out["rules"] = rules
		}
		if u.Behavior != "" {
			out["behavior"] = u.Behavior
		}
	case PermissionUpdateSetMode:
		if u.Mode != "" {
			out["mode"] = u.Mode
		}
	case PermissionUpdateAddDirectories, PermissionUpdateRemoveDirectorie:
		if u.Directories != nil {
			out["directories"] = u.Directories
		}
	}
	return json.Marshal(out)
}

// UnmarshalJSON reads the control-protocol shape produced by MarshalJSON.
func (u *PermissionUpdate) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type        string                `json:"type"`
		Rules       []PermissionRuleValue `json:"rules"`
		Behavior    string                `json:"behavior"`
		Mode        string                `json:"mode"`
		Directories []string              `json:"directories"`
		Destination string                `json:"destination"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*u = PermissionUpdate{
		Type:        raw.Type,
		Rules:       raw.Rules,
		Behavior:    raw.Behavior,
		Mode:        raw.Mode,
		Directories: raw.Directories,
		Destination: raw.Destination,
	}
	return nil
}

// ToolPermissionContext is the context handed to a CanUseTool callback.
type ToolPermissionContext struct {
	// Suggestions holds permission updates the CLI proposes.
	Suggestions []PermissionUpdate
	// ToolUseID identifies this specific tool call. Always non-empty.
	ToolUseID string
	// AgentID is set when the call originates inside a sub-agent.
	AgentID string
	// BlockedPath is the file path that triggered the request, if any.
	BlockedPath string
	// DecisionReason explains why the request was triggered.
	DecisionReason string
	// Title is the full permission prompt sentence, when provided.
	Title string
	// DisplayName is a short noun phrase for the action.
	DisplayName string
	// Description is a human-readable subtitle for the permission UI.
	Description string
}

// PermissionResult is the answer to a permission request: either
// *PermissionResultAllow or *PermissionResultDeny.
type PermissionResult interface {
	isPermissionResult()
}

// PermissionResultAllow allows the tool call, optionally rewriting its input or
// adding permission rules.
type PermissionResultAllow struct {
	UpdatedInput       map[string]any
	UpdatedPermissions []PermissionUpdate
}

func (*PermissionResultAllow) isPermissionResult() {}

// PermissionResultDeny denies the tool call. Interrupt additionally aborts the
// turn.
type PermissionResultDeny struct {
	Message   string
	Interrupt bool
}

func (*PermissionResultDeny) isPermissionResult() {}

// CanUseTool decides whether a tool call that would otherwise prompt the user
// may proceed. Returning an error surfaces as a control-protocol error to the
// CLI.
type CanUseTool func(ctx context.Context, toolName string, input map[string]any, permCtx ToolPermissionContext) (PermissionResult, error)

// ---------------------------------------------------------------------------
// Hooks
// ---------------------------------------------------------------------------

// HookEvent names a point in the agent lifecycle a hook can observe.
type HookEvent = string

// Supported hook events.
const (
	HookPreToolUse        HookEvent = "PreToolUse"
	HookPostToolUse       HookEvent = "PostToolUse"
	HookPostToolUseFailur HookEvent = "PostToolUseFailure"
	HookUserPromptSubmit  HookEvent = "UserPromptSubmit"
	HookStop              HookEvent = "Stop"
	HookSubagentStop      HookEvent = "SubagentStop"
	HookPreCompact        HookEvent = "PreCompact"
	HookNotification      HookEvent = "Notification"
	HookSubagentStart     HookEvent = "SubagentStart"
	HookPermissionRequest HookEvent = "PermissionRequest"
)

// HookContext carries per-invocation context for a hook callback. It is a
// placeholder for future abort-signal support.
type HookContext struct{}

// HookOutput is what a hook callback returns. A zero value means "no opinion":
// nothing is sent back beyond an empty object.
//
// Field names on the wire follow the CLI's documented JSON schema; Continue and
// Async are emitted as "continue" and "async".
type HookOutput struct {
	// Continue reports whether Claude should proceed. nil leaves it unset
	// (the CLI defaults to true).
	Continue *bool
	// SuppressOutput hides stdout from transcript mode.
	SuppressOutput *bool
	// StopReason is shown to the user when Continue is false.
	StopReason string
	// Decision is "block" to block the action.
	Decision string
	// SystemMessage is a warning displayed to the user.
	SystemMessage string
	// Reason is feedback for Claude about the decision.
	Reason string
	// HookSpecificOutput carries event-specific fields, e.g.
	// {"hookEventName": "PreToolUse", "permissionDecision": "allow"}.
	HookSpecificOutput map[string]any
	// Async defers hook execution; the CLI continues without waiting.
	Async bool
	// AsyncTimeout is the timeout in milliseconds for an async hook.
	AsyncTimeout *int
}

// MarshalJSON emits the CLI wire format, translating Continue and Async to
// their reserved-word key names.
func (h HookOutput) MarshalJSON() ([]byte, error) {
	out := map[string]any{}
	if h.Async {
		out["async"] = true
		if h.AsyncTimeout != nil {
			out["asyncTimeout"] = *h.AsyncTimeout
		}
		return json.Marshal(out)
	}
	if h.Continue != nil {
		out["continue"] = *h.Continue
	}
	if h.SuppressOutput != nil {
		out["suppressOutput"] = *h.SuppressOutput
	}
	if h.StopReason != "" {
		out["stopReason"] = h.StopReason
	}
	if h.Decision != "" {
		out["decision"] = h.Decision
	}
	if h.SystemMessage != "" {
		out["systemMessage"] = h.SystemMessage
	}
	if h.Reason != "" {
		out["reason"] = h.Reason
	}
	if h.HookSpecificOutput != nil {
		out["hookSpecificOutput"] = h.HookSpecificOutput
	}
	return json.Marshal(out)
}

// UnmarshalJSON reads the CLI wire format produced by MarshalJSON.
func (h *HookOutput) UnmarshalJSON(data []byte) error {
	var raw struct {
		Continue           *bool          `json:"continue"`
		SuppressOutput     *bool          `json:"suppressOutput"`
		StopReason         string         `json:"stopReason"`
		Decision           string         `json:"decision"`
		SystemMessage      string         `json:"systemMessage"`
		Reason             string         `json:"reason"`
		HookSpecificOutput map[string]any `json:"hookSpecificOutput"`
		Async              bool           `json:"async"`
		AsyncTimeout       *int           `json:"asyncTimeout"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*h = HookOutput(raw)
	return nil
}

// HookCallback runs for a matching hook event. input is the raw hook input dict
// keyed by the CLI's schema (hook_event_name, tool_name, ...); toolUseID is
// empty when the event is not tool-scoped.
type HookCallback func(ctx context.Context, input map[string]any, toolUseID string, hookCtx HookContext) (HookOutput, error)

// HookMatcher registers callbacks for one hook event.
type HookMatcher struct {
	// Matcher narrows which invocations fire the hooks, e.g. "Bash" or
	// "Write|Edit" for PreToolUse. Empty matches everything.
	Matcher string
	// Hooks are the callbacks to run. The CLI dispatches matchers for one
	// event concurrently.
	Hooks []HookCallback
	// Timeout bounds all hooks in this matcher, in seconds. Zero uses the
	// CLI default of 60.
	Timeout float64
}
