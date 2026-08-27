package codex

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// ---------------------------------------------------------------------------
// Handshake
// ---------------------------------------------------------------------------

// ClientInfo identifies the integration to the app-server. The name is also
// used by the OpenAI Compliance Logs Platform to identify the client.
type ClientInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version,omitempty"`
}

// ClientCapabilities are the optional capabilities advertised at initialize.
type ClientCapabilities struct {
	// ExperimentalApi opts into experimental methods and fields.
	ExperimentalApi bool `json:"experimentalApi,omitempty"`
	// OptOutNotificationMethods lists exact notification method names to
	// suppress for this connection.
	OptOutNotificationMethods []string `json:"optOutNotificationMethods,omitempty"`
	// RequestAttestation opts into the server-initiated attestation/generate
	// request.
	RequestAttestation bool `json:"requestAttestation,omitempty"`
	// McpServerOpenaiFormElicitation allows the OpenAI extended-form variant of
	// mcpServer/elicitation/request.
	McpServerOpenaiFormElicitation bool `json:"mcpServerOpenaiFormElicitation,omitempty"`
}

// InitializeParams are the parameters of the initialize request.
type InitializeParams struct {
	ClientInfo   ClientInfo          `json:"clientInfo"`
	Capabilities *ClientCapabilities `json:"capabilities,omitempty"`
}

// InitializeResult describes the connected app-server.
type InitializeResult struct {
	// UserAgent is the user agent the server presents to upstream services.
	UserAgent string `json:"userAgent"`
	// PlatformFamily describes the runtime target family.
	PlatformFamily string `json:"platformFamily"`
	// PlatformOs describes the runtime operating system.
	PlatformOs string `json:"platformOs"`
}

// ---------------------------------------------------------------------------
// Policies
// ---------------------------------------------------------------------------

// ApprovalPolicy controls when Codex asks the client for approval.
type ApprovalPolicy string

// Approval policies accepted by thread/start and turn/start.
const (
	ApprovalNever         ApprovalPolicy = "never"
	ApprovalOnRequest     ApprovalPolicy = "onRequest"
	ApprovalOnFailure     ApprovalPolicy = "onFailure"
	ApprovalUnlessTrusted ApprovalPolicy = "unlessTrusted"
)

// Sandbox policy type discriminators.
const (
	SandboxTypeReadOnly         = "readOnly"
	SandboxTypeWorkspaceWrite   = "workspaceWrite"
	SandboxTypeDangerFullAccess = "dangerFullAccess"
	SandboxTypeExternalSandbox  = "externalSandbox"
)

// Network access values for the externalSandbox policy.
const (
	NetworkAccessRestricted = "restricted"
	NetworkAccessEnabled    = "enabled"
)

// ReadOnlyAccess describes which roots a sandboxed session may read.
type ReadOnlyAccess struct {
	// Type is "fullAccess" or "restricted".
	Type string `json:"type"`
	// IncludePlatformDefaults appends a curated platform-default policy for
	// restricted-read sessions.
	IncludePlatformDefaults bool `json:"includePlatformDefaults,omitempty"`
	// ReadableRoots lists the absolute roots readable under "restricted".
	ReadableRoots []string `json:"readableRoots,omitempty"`
}

// FullReadAccess returns the default unrestricted read access.
func FullReadAccess() *ReadOnlyAccess { return &ReadOnlyAccess{Type: "fullAccess"} }

// RestrictedReadAccess returns read access limited to the given roots.
func RestrictedReadAccess(includePlatformDefaults bool, roots ...string) *ReadOnlyAccess {
	return &ReadOnlyAccess{
		Type:                    "restricted",
		IncludePlatformDefaults: includePlatformDefaults,
		ReadableRoots:           roots,
	}
}

// SandboxPolicy is the tagged sandbox configuration shared by thread/start,
// turn/start, and command/exec.
type SandboxPolicy struct {
	// Type is one of the SandboxType* constants.
	Type string `json:"type"`
	// Access applies to the readOnly policy.
	Access *ReadOnlyAccess `json:"access,omitempty"`
	// WritableRoots applies to the workspaceWrite policy.
	WritableRoots []string `json:"writableRoots,omitempty"`
	// ReadOnlyAccess applies to the workspaceWrite policy.
	ReadOnlyAccess *ReadOnlyAccess `json:"readOnlyAccess,omitempty"`
	// NetworkAccess is a bool for workspaceWrite and one of the
	// NetworkAccess* strings for externalSandbox.
	NetworkAccess any `json:"networkAccess,omitempty"`
}

// SandboxReadOnly returns a readOnly sandbox policy. Pass nil access for the
// server default (full read access).
func SandboxReadOnly(access *ReadOnlyAccess) *SandboxPolicy {
	return &SandboxPolicy{Type: SandboxTypeReadOnly, Access: access}
}

// SandboxWorkspaceWrite returns a workspaceWrite sandbox policy.
func SandboxWorkspaceWrite(writableRoots []string, networkAccess bool, readOnlyAccess *ReadOnlyAccess) *SandboxPolicy {
	return &SandboxPolicy{
		Type:           SandboxTypeWorkspaceWrite,
		WritableRoots:  writableRoots,
		ReadOnlyAccess: readOnlyAccess,
		NetworkAccess:  networkAccess,
	}
}

// SandboxDangerFullAccess returns the unsandboxed policy.
func SandboxDangerFullAccess() *SandboxPolicy {
	return &SandboxPolicy{Type: SandboxTypeDangerFullAccess}
}

// SandboxExternal returns the externalSandbox policy for hosts that already
// sandbox the server process. networkAccess is NetworkAccessRestricted or
// NetworkAccessEnabled.
func SandboxExternal(networkAccess string) *SandboxPolicy {
	return &SandboxPolicy{Type: SandboxTypeExternalSandbox, NetworkAccess: networkAccess}
}

// ---------------------------------------------------------------------------
// Input items
// ---------------------------------------------------------------------------

// InputItem is one element of a turn's user input.
type InputItem interface {
	inputItem()
}

// TextInput is plain user text.
type TextInput struct {
	Text string `json:"text"`
}

// ImageInput references a remote image by URL.
type ImageInput struct {
	URL string `json:"url"`
}

// LocalImageInput references an image on the local filesystem.
type LocalImageInput struct {
	Path string `json:"path"`
}

// SkillInput attaches a skill so the server injects its full instructions.
type SkillInput struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// MentionInput references an app or file by path, such as "app://demo-app".
type MentionInput struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func (TextInput) inputItem()       {}
func (ImageInput) inputItem()      {}
func (LocalImageInput) inputItem() {}
func (SkillInput) inputItem()      {}
func (MentionInput) inputItem()    {}

// MarshalJSON emits the tagged text input item.
func (i TextInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"type": "text", "text": i.Text})
}

// MarshalJSON emits the tagged image input item.
func (i ImageInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"type": "image", "url": i.URL})
}

// MarshalJSON emits the tagged local image input item.
func (i LocalImageInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"type": "localImage", "path": i.Path})
}

// MarshalJSON emits the tagged skill input item.
func (i SkillInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"type": "skill", "name": i.Name, "path": i.Path})
}

// MarshalJSON emits the tagged mention input item.
func (i MentionInput) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"type": "mention", "name": i.Name, "path": i.Path})
}

// Text is shorthand for a single text input item.
func Text(text string) []InputItem { return []InputItem{TextInput{Text: text}} }

// ---------------------------------------------------------------------------
// Threads and turns
// ---------------------------------------------------------------------------

// Thread status type discriminators.
const (
	ThreadStatusNotLoaded   = "notLoaded"
	ThreadStatusIdle        = "idle"
	ThreadStatusSystemError = "systemError"
	ThreadStatusActive      = "active"
)

// ThreadStatus is a loaded thread's runtime status.
type ThreadStatus struct {
	// Type is one of the ThreadStatus* constants.
	Type string `json:"type"`
	// ActiveFlags describes what an active thread is waiting on.
	ActiveFlags []string `json:"activeFlags,omitempty"`
}

// GitInfo is persisted Git metadata for a stored thread.
type GitInfo struct {
	SHA       *string `json:"sha,omitempty"`
	Branch    *string `json:"branch,omitempty"`
	OriginURL *string `json:"originUrl,omitempty"`
}

// Thread is a conversation between a user and the Codex agent.
type Thread struct {
	ID string `json:"id"`
	// SessionID identifies the live session tree root; forked threads keep the
	// session id of the root they came from.
	SessionID string `json:"sessionId,omitempty"`
	// Name is the user-facing thread title, when one has been set.
	Name *string `json:"name,omitempty"`
	// Preview is a short excerpt of the thread's first user message.
	Preview string `json:"preview,omitempty"`
	// Ephemeral reports an in-memory thread that is not listed in storage.
	Ephemeral bool `json:"ephemeral,omitempty"`
	// ForkedFromID is the source thread of a fork, when available.
	ForkedFromID string `json:"forkedFromId,omitempty"`
	// ModelProvider is the provider backing the thread, such as "openai".
	ModelProvider string `json:"modelProvider,omitempty"`
	// IsPinned is the persisted pin state.
	IsPinned bool `json:"isPinned,omitempty"`
	// CreatedAt and UpdatedAt are Unix timestamps in seconds.
	CreatedAt int64 `json:"createdAt,omitempty"`
	UpdatedAt int64 `json:"updatedAt,omitempty"`
	// Status is the runtime status, present on read and list responses.
	Status *ThreadStatus `json:"status,omitempty"`
	// GitInfo is persisted Git metadata.
	GitInfo *GitInfo `json:"gitInfo,omitempty"`
	// Turns is populated when the caller asked for turn history.
	Turns []Turn `json:"turns,omitempty"`
	// InstructionSources lists loaded instruction-file paths.
	InstructionSources []string `json:"instructionSources,omitempty"`
}

// Turn statuses reported by turn/start and turn/completed.
const (
	TurnInProgress  = "inProgress"
	TurnCompleted   = "completed"
	TurnInterrupted = "interrupted"
	TurnFailed      = "failed"
)

// Turn is a single user request and the agent work that follows.
type Turn struct {
	ID string `json:"id"`
	// ThreadID is present on turn payloads that carry it.
	ThreadID string `json:"threadId,omitempty"`
	// Status is one of TurnInProgress, TurnCompleted, TurnInterrupted, or
	// TurnFailed.
	Status string `json:"status"`
	// Items holds the turn's items. turn/diff/updated and turn/plan/updated
	// carry an empty list; use item/* notifications as the source of truth.
	Items []ThreadItem `json:"items,omitempty"`
	// Error is set when Status is TurnFailed.
	Error *TurnError `json:"error,omitempty"`
	// Usage is the token usage recorded for the turn, when reported.
	Usage *TokenUsage `json:"usage,omitempty"`
}

// IsTerminal reports whether the turn has reached a final status.
func (t *Turn) IsTerminal() bool {
	switch t.Status {
	case TurnCompleted, TurnInterrupted, TurnFailed:
		return true
	default:
		return false
	}
}

// TurnError describes why a turn failed.
type TurnError struct {
	Message string `json:"message"`
	// CodexErrorInfo is a tagged union whose "type" is one of the ErrorInfo*
	// constants; it can also carry an httpStatusCode.
	CodexErrorInfo json.RawMessage `json:"codexErrorInfo,omitempty"`
	// AdditionalDetails carries free-form server detail.
	AdditionalDetails json.RawMessage `json:"additionalDetails,omitempty"`
}

// Error implements the error interface.
func (e *TurnError) Error() string {
	if kind := e.Kind(); kind != "" {
		return fmt.Sprintf("codex: turn failed (%s): %s", kind, e.Message)
	}
	return "codex: turn failed: " + e.Message
}

// Kind returns the codexErrorInfo discriminator, which the server sends either
// as a bare string or as an object with a "type" field. It returns the empty
// string when no error info is present.
func (e *TurnError) Kind() string {
	if len(e.CodexErrorInfo) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(e.CodexErrorInfo, &s); err == nil {
		return s
	}
	var obj struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(e.CodexErrorInfo, &obj); err == nil {
		return obj.Type
	}
	return ""
}

// HTTPStatusCode returns the upstream HTTP status forwarded by the server, if
// any.
func (e *TurnError) HTTPStatusCode() (int, bool) {
	if len(e.CodexErrorInfo) == 0 {
		return 0, false
	}
	var obj struct {
		HTTPStatusCode *int `json:"httpStatusCode"`
	}
	if err := json.Unmarshal(e.CodexErrorInfo, &obj); err != nil || obj.HTTPStatusCode == nil {
		return 0, false
	}
	return *obj.HTTPStatusCode, true
}

// TokenUsage reports token consumption for a thread or turn.
type TokenUsage struct {
	InputTokens         int64 `json:"inputTokens,omitempty"`
	CachedInputTokens   int64 `json:"cachedInputTokens,omitempty"`
	OutputTokens        int64 `json:"outputTokens,omitempty"`
	ReasoningTokens     int64 `json:"reasoningOutputTokens,omitempty"`
	TotalTokens         int64 `json:"totalTokens,omitempty"`
	ContextWindow       int64 `json:"contextWindow,omitempty"`
	ContextWindowUsed   int64 `json:"contextWindowUsed,omitempty"`
	ContextWindowRemain int64 `json:"contextWindowRemaining,omitempty"`
}

// ---------------------------------------------------------------------------
// Thread items
// ---------------------------------------------------------------------------

// Item type discriminators carried in ThreadItem.
const (
	ItemUserMessage       = "userMessage"
	ItemAgentMessage      = "agentMessage"
	ItemReasoning         = "reasoning"
	ItemCommandExecution  = "commandExecution"
	ItemFileChange        = "fileChange"
	ItemMcpToolCall       = "mcpToolCall"
	ItemWebSearch         = "webSearch"
	ItemPlan              = "plan"
	ItemEnteredReviewMode = "enteredReviewMode"
	ItemExitedReviewMode  = "exitedReviewMode"
	ItemContextCompaction = "contextCompaction"
)

// Item is one unit of input or output inside a turn.
type Item interface {
	// ItemID returns the item id that deltas refer to.
	ItemID() string
	// ItemType returns the item's type discriminator.
	ItemType() string
}

// UserMessageItem is user-supplied input recorded in the thread.
type UserMessageItem struct {
	ID string `json:"id"`
	// Content holds the raw user input items as sent by the client.
	Content []json.RawMessage `json:"content,omitempty"`
}

// AgentMessageItem is the accumulated agent reply.
type AgentMessageItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	// Phase uses Responses API wire values ("commentary", "final_answer").
	Phase string `json:"phase,omitempty"`
}

// ReasoningItem holds streamed reasoning summaries and raw reasoning blocks.
type ReasoningItem struct {
	ID      string   `json:"id"`
	Summary []string `json:"summary,omitempty"`
	Content []string `json:"content,omitempty"`
}

// Command execution and file change item statuses.
const (
	ItemStatusInProgress = "inProgress"
	ItemStatusCompleted  = "completed"
	ItemStatusFailed     = "failed"
	ItemStatusDeclined   = "declined"
)

// CommandExecutionItem describes a command the agent runs.
type CommandExecutionItem struct {
	ID               string          `json:"id"`
	Command          string          `json:"command"`
	Cwd              string          `json:"cwd,omitempty"`
	Status           string          `json:"status"`
	CommandActions   json.RawMessage `json:"commandActions,omitempty"`
	AggregatedOutput string          `json:"aggregatedOutput,omitempty"`
	ExitCode         *int            `json:"exitCode,omitempty"`
	DurationMs       *int64          `json:"durationMs,omitempty"`
}

// FileChange is one proposed edit inside a FileChangeItem.
type FileChange struct {
	Path string `json:"path"`
	// Kind is the edit kind, such as "add", "update", or "delete".
	Kind string `json:"kind"`
	Diff string `json:"diff,omitempty"`
}

// FileChangeItem describes proposed edits.
type FileChangeItem struct {
	ID      string       `json:"id"`
	Changes []FileChange `json:"changes"`
	Status  string       `json:"status"`
}

// McpToolCallItem describes an MCP (or connector app) tool call.
type McpToolCallItem struct {
	ID         string          `json:"id"`
	Server     string          `json:"server"`
	Tool       string          `json:"tool"`
	Status     string          `json:"status"`
	Arguments  json.RawMessage `json:"arguments,omitempty"`
	AppContext json.RawMessage `json:"appContext,omitempty"`
	PluginID   string          `json:"pluginId,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
}

// WebSearchAction describes what a web search item is doing.
type WebSearchAction struct {
	// Type is "search", "openPage", or "findInPage".
	Type    string   `json:"type"`
	Query   string   `json:"query,omitempty"`
	Queries []string `json:"queries,omitempty"`
	URL     string   `json:"url,omitempty"`
	Pattern string   `json:"pattern,omitempty"`
}

// WebSearchItem is a web search issued by the agent.
type WebSearchItem struct {
	ID     string           `json:"id"`
	Query  string           `json:"query,omitempty"`
	Action *WebSearchAction `json:"action,omitempty"`
}

// PlanItem carries proposed plan text in plan mode.
type PlanItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// EnteredReviewModeItem is emitted when the reviewer starts.
type EnteredReviewModeItem struct {
	ID     string `json:"id"`
	Review string `json:"review"`
}

// ExitedReviewModeItem carries the final review text.
type ExitedReviewModeItem struct {
	ID     string `json:"id"`
	Review string `json:"review"`
}

// ContextCompactionItem is emitted when Codex compacts conversation history.
type ContextCompactionItem struct {
	ID string `json:"id"`
}

// UnknownItem preserves items whose type this client does not model, so that
// newer server versions do not break decoding.
type UnknownItem struct {
	ID   string          `json:"id"`
	Type string          `json:"type"`
	Raw  json.RawMessage `json:"-"`
}

// ItemID returns the item id.
func (i UserMessageItem) ItemID() string { return i.ID }

// ItemType returns the item type discriminator.
func (i UserMessageItem) ItemType() string { return ItemUserMessage }

// ItemID returns the item id.
func (i AgentMessageItem) ItemID() string { return i.ID }

// ItemType returns the item type discriminator.
func (i AgentMessageItem) ItemType() string { return ItemAgentMessage }

// ItemID returns the item id.
func (i ReasoningItem) ItemID() string { return i.ID }

// ItemType returns the item type discriminator.
func (i ReasoningItem) ItemType() string { return ItemReasoning }

// ItemID returns the item id.
func (i CommandExecutionItem) ItemID() string { return i.ID }

// ItemType returns the item type discriminator.
func (i CommandExecutionItem) ItemType() string { return ItemCommandExecution }

// ItemID returns the item id.
func (i FileChangeItem) ItemID() string { return i.ID }

// ItemType returns the item type discriminator.
func (i FileChangeItem) ItemType() string { return ItemFileChange }

// ItemID returns the item id.
func (i McpToolCallItem) ItemID() string { return i.ID }

// ItemType returns the item type discriminator.
func (i McpToolCallItem) ItemType() string { return ItemMcpToolCall }

// ItemID returns the item id.
func (i WebSearchItem) ItemID() string { return i.ID }

// ItemType returns the item type discriminator.
func (i WebSearchItem) ItemType() string { return ItemWebSearch }

// ItemID returns the item id.
func (i PlanItem) ItemID() string { return i.ID }

// ItemType returns the item type discriminator.
func (i PlanItem) ItemType() string { return ItemPlan }

// ItemID returns the item id.
func (i EnteredReviewModeItem) ItemID() string { return i.ID }

// ItemType returns the item type discriminator.
func (i EnteredReviewModeItem) ItemType() string { return ItemEnteredReviewMode }

// ItemID returns the item id.
func (i ExitedReviewModeItem) ItemID() string { return i.ID }

// ItemType returns the item type discriminator.
func (i ExitedReviewModeItem) ItemType() string { return ItemExitedReviewMode }

// ItemID returns the item id.
func (i ContextCompactionItem) ItemID() string { return i.ID }

// ItemType returns the item type discriminator.
func (i ContextCompactionItem) ItemType() string { return ItemContextCompaction }

// ItemID returns the item id.
func (i UnknownItem) ItemID() string { return i.ID }

// ItemType returns the item type discriminator as sent by the server.
func (i UnknownItem) ItemType() string { return i.Type }

// ThreadItem is the tagged union carried in turn responses and item/*
// notifications. Item holds the decoded value and Raw the original JSON.
type ThreadItem struct {
	// Item is the decoded item; unmodeled types decode to UnknownItem.
	Item Item
	// Raw is the original JSON object.
	Raw json.RawMessage
}

// Type returns the item's type discriminator, or "" for a zero value.
func (t ThreadItem) Type() string {
	if t.Item == nil {
		return ""
	}
	return t.Item.ItemType()
}

// ID returns the item id, or "" for a zero value.
func (t ThreadItem) ID() string {
	if t.Item == nil {
		return ""
	}
	return t.Item.ItemID()
}

// UnmarshalJSON decodes a tagged thread item, falling back to UnknownItem for
// types this client does not model.
func (t *ThreadItem) UnmarshalJSON(data []byte) error {
	t.Raw = append(json.RawMessage(nil), data...)

	var head struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return fmt.Errorf("codex: decode thread item: %w", err)
	}

	decode := func(v Item) error {
		if err := json.Unmarshal(data, v); err != nil {
			return fmt.Errorf("codex: decode %s item: %w", head.Type, err)
		}
		t.Item = v
		return nil
	}

	switch head.Type {
	case ItemUserMessage:
		return decode(&UserMessageItem{})
	case ItemAgentMessage:
		return decode(&AgentMessageItem{})
	case ItemReasoning:
		return decode(&ReasoningItem{})
	case ItemCommandExecution:
		return decode(&CommandExecutionItem{})
	case ItemFileChange:
		return decode(&FileChangeItem{})
	case ItemMcpToolCall:
		return decode(&McpToolCallItem{})
	case ItemWebSearch:
		return decode(&WebSearchItem{})
	case ItemPlan:
		return decode(&PlanItem{})
	case ItemEnteredReviewMode:
		return decode(&EnteredReviewModeItem{})
	case ItemExitedReviewMode:
		return decode(&ExitedReviewModeItem{})
	case ItemContextCompaction:
		return decode(&ContextCompactionItem{})
	default:
		t.Item = &UnknownItem{ID: head.ID, Type: head.Type, Raw: t.Raw}
		return nil
	}
}

// MarshalJSON re-emits the original JSON when available.
func (t ThreadItem) MarshalJSON() ([]byte, error) {
	if len(t.Raw) > 0 {
		return t.Raw, nil
	}
	if t.Item == nil {
		return []byte("null"), nil
	}
	return json.Marshal(t.Item)
}

// ---------------------------------------------------------------------------
// Request and response payloads
// ---------------------------------------------------------------------------

// StartThreadParams are the parameters of thread/start.
type StartThreadParams struct {
	Model          string         `json:"model,omitempty"`
	Cwd            string         `json:"cwd,omitempty"`
	ApprovalPolicy ApprovalPolicy `json:"approvalPolicy,omitempty"`
	// Sandbox is the shorthand sandbox mode name accepted by thread/start.
	Sandbox string `json:"sandbox,omitempty"`
	// SandboxPolicy is the full sandbox policy object.
	SandboxPolicy *SandboxPolicy `json:"sandboxPolicy,omitempty"`
	Personality   string         `json:"personality,omitempty"`
	// ServiceName tags thread-level metrics with the integration's name.
	ServiceName string `json:"serviceName,omitempty"`
}

// ResumeThreadParams are the parameters of thread/resume.
type ResumeThreadParams struct {
	ThreadID       string         `json:"threadId"`
	Model          string         `json:"model,omitempty"`
	Cwd            string         `json:"cwd,omitempty"`
	ApprovalPolicy ApprovalPolicy `json:"approvalPolicy,omitempty"`
	Sandbox        string         `json:"sandbox,omitempty"`
	SandboxPolicy  *SandboxPolicy `json:"sandboxPolicy,omitempty"`
	Personality    string         `json:"personality,omitempty"`
}

// ForkThreadParams are the parameters of thread/fork.
type ForkThreadParams struct {
	ThreadID string `json:"threadId"`
	// LastTurnID copies history through that turn, inclusive.
	LastTurnID string `json:"lastTurnId,omitempty"`
	// Ephemeral creates an in-memory fork.
	Ephemeral bool `json:"ephemeral,omitempty"`
}

// ThreadResult is the common `{ "thread": ... }` response body.
type ThreadResult struct {
	Thread Thread `json:"thread"`
}

// ReadThreadParams are the parameters of thread/read.
type ReadThreadParams struct {
	ThreadID     string `json:"threadId"`
	IncludeTurns bool   `json:"includeTurns,omitempty"`
}

// Thread list sort keys.
const (
	SortKeyCreatedAt = "created_at"
	SortKeyUpdatedAt = "updated_at"
	SortKeyRecencyAt = "recency_at"
)

// Thread list sort directions.
const (
	SortAscending  = "asc"
	SortDescending = "desc"
)

// ListThreadsParams are the parameters of thread/list.
type ListThreadsParams struct {
	Cursor         string   `json:"cursor,omitempty"`
	Limit          int      `json:"limit,omitempty"`
	SortKey        string   `json:"sortKey,omitempty"`
	SortDirection  string   `json:"sortDirection,omitempty"`
	ModelProviders []string `json:"modelProviders,omitempty"`
	SourceKinds    []string `json:"sourceKinds,omitempty"`
	Archived       bool     `json:"archived,omitempty"`
	IsPinned       *bool    `json:"isPinned,omitempty"`
	Cwd            []string `json:"cwd,omitempty"`
	UseStateDBOnly bool     `json:"useStateDbOnly,omitempty"`
	SearchTerm     string   `json:"searchTerm,omitempty"`
}

// ListThreadsResult is one page of thread/list results.
type ListThreadsResult struct {
	Data []Thread `json:"data"`
	// NextCursor is empty on the final page.
	NextCursor string `json:"nextCursor,omitempty"`
}

// UnsubscribeResult reports the outcome of thread/unsubscribe.
type UnsubscribeResult struct {
	// Status is "unsubscribed", "notSubscribed", or "notLoaded".
	Status string `json:"status"`
}

// TurnOptions are the per-turn overrides accepted by turn/start. When set they
// become the defaults for later turns on the same thread, except OutputSchema
// which applies only to the current turn.
type TurnOptions struct {
	Model          string          `json:"model,omitempty"`
	Cwd            string          `json:"cwd,omitempty"`
	ApprovalPolicy ApprovalPolicy  `json:"approvalPolicy,omitempty"`
	SandboxPolicy  *SandboxPolicy  `json:"sandboxPolicy,omitempty"`
	Effort         string          `json:"effort,omitempty"`
	Summary        string          `json:"summary,omitempty"`
	Personality    string          `json:"personality,omitempty"`
	OutputSchema   json.RawMessage `json:"outputSchema,omitempty"`
}

// StartTurnParams are the parameters of turn/start.
type StartTurnParams struct {
	ThreadID string      `json:"threadId"`
	Input    []InputItem `json:"input"`
	TurnOptions
}

// StartTurnResult is the turn/start response body.
type StartTurnResult struct {
	Turn Turn `json:"turn"`
}

// SteerTurnParams are the parameters of turn/steer.
type SteerTurnParams struct {
	ThreadID string      `json:"threadId"`
	Input    []InputItem `json:"input"`
	// ExpectedTurnID must match the active turn id.
	ExpectedTurnID string `json:"expectedTurnId"`
}

// SteerTurnResult is the turn/steer response body.
type SteerTurnResult struct {
	TurnID string `json:"turnId"`
}

// InterruptTurnParams are the parameters of turn/interrupt.
type InterruptTurnParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
}

// ---------------------------------------------------------------------------
// Notification payloads
// ---------------------------------------------------------------------------

// Notification method names this client handles.
const (
	MethodThreadStarted       = "thread/started"
	MethodThreadStatusChanged = "thread/status/changed"
	MethodThreadArchived      = "thread/archived"
	MethodThreadUnarchived    = "thread/unarchived"
	MethodThreadDeleted       = "thread/deleted"
	MethodThreadClosed        = "thread/closed"
	MethodThreadNameUpdated   = "thread/name/updated"
	MethodTokenUsageUpdated   = "thread/tokenUsage/updated"

	MethodTurnStarted   = "turn/started"
	MethodTurnCompleted = "turn/completed"
	MethodTurnDiff      = "turn/diff/updated"
	MethodTurnPlan      = "turn/plan/updated"

	MethodItemStarted   = "item/started"
	MethodItemCompleted = "item/completed"

	MethodAgentMessageDelta          = "item/agentMessage/delta"
	MethodPlanDelta                  = "item/plan/delta"
	MethodReasoningSummaryTextDelta  = "item/reasoning/summaryTextDelta"
	MethodReasoningSummaryPartAdded  = "item/reasoning/summaryPartAdded"
	MethodReasoningTextDelta         = "item/reasoning/textDelta"
	MethodCommandExecutionOutputDlta = "item/commandExecution/outputDelta"

	MethodServerRequestResolved = "serverRequest/resolved"
	MethodAccountUpdated        = "account/updated"
	MethodLoginCompleted        = "account/login/completed"
)

// ThreadStartedParams is the payload of thread/started.
type ThreadStartedParams struct {
	Thread Thread `json:"thread"`
}

// ThreadStatusChangedParams is the payload of thread/status/changed.
type ThreadStatusChangedParams struct {
	ThreadID string       `json:"threadId"`
	Status   ThreadStatus `json:"status"`
}

// ThreadIDParams is the payload of the thread/archived, thread/unarchived,
// thread/deleted, and thread/closed notifications.
type ThreadIDParams struct {
	ThreadID string `json:"threadId"`
}

// ThreadNameUpdatedParams is the payload of thread/name/updated.
type ThreadNameUpdatedParams struct {
	ThreadID string `json:"threadId"`
	Name     string `json:"name"`
}

// TurnParams is the payload of turn/started and turn/completed.
type TurnParams struct {
	// ThreadID is present when the server scopes the notification.
	ThreadID string `json:"threadId,omitempty"`
	Turn     Turn   `json:"turn"`
}

// TurnDiffParams is the payload of turn/diff/updated.
type TurnDiffParams struct {
	ThreadID string `json:"threadId,omitempty"`
	TurnID   string `json:"turnId"`
	Diff     string `json:"diff"`
}

// Plan step statuses.
const (
	PlanStepPending    = "pending"
	PlanStepInProgress = "inProgress"
	PlanStepCompleted  = "completed"
)

// PlanStep is one entry of an agent plan.
type PlanStep struct {
	Step   string `json:"step"`
	Status string `json:"status"`
}

// TurnPlanParams is the payload of turn/plan/updated.
type TurnPlanParams struct {
	ThreadID    string     `json:"threadId,omitempty"`
	TurnID      string     `json:"turnId"`
	Explanation string     `json:"explanation,omitempty"`
	Plan        []PlanStep `json:"plan"`
}

// ItemParams is the payload of item/started and item/completed.
type ItemParams struct {
	ThreadID string     `json:"threadId,omitempty"`
	TurnID   string     `json:"turnId,omitempty"`
	Item     ThreadItem `json:"item"`
}

// DeltaParams is the payload of the text delta notifications
// (item/agentMessage/delta, item/plan/delta, item/reasoning/textDelta, and
// item/reasoning/summaryTextDelta).
type DeltaParams struct {
	ThreadID string `json:"threadId,omitempty"`
	TurnID   string `json:"turnId,omitempty"`
	ItemID   string `json:"itemId"`
	Delta    string `json:"delta"`
	// SummaryIndex increments when a new reasoning summary section opens.
	SummaryIndex int `json:"summaryIndex,omitempty"`
	// ContentIndex identifies the raw reasoning content block.
	ContentIndex int `json:"contentIndex,omitempty"`
}

// CommandOutputDeltaParams is the payload of
// item/commandExecution/outputDelta.
type CommandOutputDeltaParams struct {
	ThreadID string `json:"threadId,omitempty"`
	TurnID   string `json:"turnId,omitempty"`
	ItemID   string `json:"itemId"`
	// Stream is "stdout" or "stderr" when the server reports it.
	Stream string `json:"stream,omitempty"`
	// Delta is plain-text output.
	Delta string `json:"delta,omitempty"`
	// Chunk is a base64-encoded output chunk used by some server versions.
	Chunk string `json:"chunk,omitempty"`
	// DeltaBase64 is a base64-encoded output chunk.
	DeltaBase64 string `json:"deltaBase64,omitempty"`
}

// Text returns the decoded output chunk, preferring the plain-text field and
// falling back to the base64-encoded variants.
func (p CommandOutputDeltaParams) Text() string {
	if p.Delta != "" {
		return p.Delta
	}
	for _, encoded := range []string{p.DeltaBase64, p.Chunk} {
		if encoded == "" {
			continue
		}
		if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil {
			return string(decoded)
		}
		return encoded
	}
	return ""
}

// TokenUsageParams is the payload of thread/tokenUsage/updated.
type TokenUsageParams struct {
	ThreadID string     `json:"threadId,omitempty"`
	TurnID   string     `json:"turnId,omitempty"`
	Usage    TokenUsage `json:"usage"`
}

// ServerRequestResolvedParams is the payload of serverRequest/resolved.
type ServerRequestResolvedParams struct {
	ThreadID  string          `json:"threadId,omitempty"`
	RequestID json.RawMessage `json:"requestId,omitempty"`
}
