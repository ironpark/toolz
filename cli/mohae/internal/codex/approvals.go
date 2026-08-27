package codex

import (
	"context"
	"encoding/json"
	"sync"
)

// Server-initiated request methods handled by this client.
const (
	MethodCommandApproval      = "item/commandExecution/requestApproval"
	MethodFileChangeApproval   = "item/fileChange/requestApproval"
	MethodPermissionsApproval  = "item/permissions/requestApproval"
	MethodRequestUserInput     = "tool/requestUserInput"
	MethodItemRequestUserInput = "item/tool/requestUserInput"
	MethodChatGPTTokenRefresh  = "account/chatgptAuthTokens/refresh"
)

// Decision is the client's answer to an approval request.
type Decision string

// Approval decisions accepted for command execution and file changes.
const (
	// DecisionAccept runs the proposed action once.
	DecisionAccept Decision = "accept"
	// DecisionAcceptForSession runs it and stops asking for this session.
	DecisionAcceptForSession Decision = "acceptForSession"
	// DecisionDecline refuses the action; the turn continues.
	DecisionDecline Decision = "decline"
	// DecisionCancel refuses the action and cancels the turn.
	DecisionCancel Decision = "cancel"
)

// NetworkApprovalContext is present when a command approval prompt is really a
// managed network-access prompt.
type NetworkApprovalContext struct {
	Host     string `json:"host,omitempty"`
	Protocol string `json:"protocol,omitempty"`
	Port     int    `json:"port,omitempty"`
}

// CommandApprovalRequest asks whether the agent may run a command.
type CommandApprovalRequest struct {
	ItemID   string `json:"itemId"`
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	Reason   string `json:"reason,omitempty"`
	Command  string `json:"command,omitempty"`
	Cwd      string `json:"cwd,omitempty"`
	// CommandActions describes the parsed command actions, when available.
	CommandActions json.RawMessage `json:"commandActions,omitempty"`
	// ProposedExecpolicyAmendment is an amendment the client may accept with
	// the acceptWithExecpolicyAmendment decision.
	ProposedExecpolicyAmendment json.RawMessage `json:"proposedExecpolicyAmendment,omitempty"`
	// NetworkApprovalContext marks a managed network-access prompt.
	NetworkApprovalContext *NetworkApprovalContext `json:"networkApprovalContext,omitempty"`
	// AvailableDecisions restricts the decisions the server accepts.
	AvailableDecisions []Decision `json:"availableDecisions,omitempty"`
	// AdditionalPermissions describes requested per-command sandbox access
	// (experimental).
	AdditionalPermissions json.RawMessage `json:"additionalPermissions,omitempty"`
	// Params is the raw request payload.
	Params json.RawMessage `json:"-"`
}

// FileChangeApprovalRequest asks whether the agent may apply file edits.
type FileChangeApprovalRequest struct {
	ItemID   string `json:"itemId"`
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	Reason   string `json:"reason,omitempty"`
	// GrantRoot is the root the agent asks to be granted write access to.
	GrantRoot string `json:"grantRoot,omitempty"`
	// Changes lists the proposed edits, when the server includes them.
	Changes []FileChange `json:"changes,omitempty"`
	// Params is the raw request payload.
	Params json.RawMessage `json:"-"`
}

// PermissionsRequest is sent by the built-in request_permissions tool.
type PermissionsRequest struct {
	ItemID        string `json:"itemId"`
	ThreadID      string `json:"threadId"`
	TurnID        string `json:"turnId"`
	EnvironmentID string `json:"environmentId,omitempty"`
	Cwd           string `json:"cwd,omitempty"`
	Reason        string `json:"reason,omitempty"`
	// Permissions is the requested network or filesystem permission set.
	Permissions json.RawMessage `json:"permissions,omitempty"`
	// Params is the raw request payload.
	Params json.RawMessage `json:"-"`
}

// Permission grant scopes.
const (
	// ScopeTurn grants permissions for the current turn only.
	ScopeTurn = "turn"
	// ScopeSession persists the grant for later turns in the session.
	ScopeSession = "session"
)

// PermissionsResponse carries the granted subset of a permissions request.
// Permissions that were not requested are ignored by the server.
type PermissionsResponse struct {
	Permissions json.RawMessage `json:"permissions"`
	Scope       string          `json:"scope,omitempty"`
}

// TokenRefreshRequest asks the host application for fresh externally managed
// ChatGPT tokens.
type TokenRefreshRequest struct {
	Reason            string          `json:"reason,omitempty"`
	PreviousAccountID string          `json:"previousAccountId,omitempty"`
	Params            json.RawMessage `json:"-"`
}

// ChatGPTAuthTokens are externally managed ChatGPT credentials.
type ChatGPTAuthTokens struct {
	AccessToken      string `json:"accessToken"`
	ChatGPTAccountID string `json:"chatgptAccountId"`
	ChatGPTPlanType  string `json:"chatgptPlanType,omitempty"`
}

// ApprovalHandler answers the approval requests the app-server sends during a
// turn. The context is canceled when the turn ends, so a handler that is
// waiting on a user must honor it.
type ApprovalHandler interface {
	// ApproveCommand decides whether a command may run.
	ApproveCommand(ctx context.Context, req *CommandApprovalRequest) (Decision, error)
	// ApproveFileChange decides whether proposed edits may be applied.
	ApproveFileChange(ctx context.Context, req *FileChangeApprovalRequest) (Decision, error)
}

// PermissionApprover is an optional ApprovalHandler extension for the
// request_permissions tool.
type PermissionApprover interface {
	ApprovePermissions(ctx context.Context, req *PermissionsRequest) (*PermissionsResponse, error)
}

// UserInputResponder is an optional ApprovalHandler extension for
// tool/requestUserInput. The returned value is marshaled as the JSON-RPC
// result.
type UserInputResponder interface {
	RequestUserInput(ctx context.Context, params json.RawMessage) (any, error)
}

// TokenRefresher is an optional ApprovalHandler extension for hosts that own
// the ChatGPT auth lifecycle.
type TokenRefresher interface {
	RefreshChatGPTTokens(ctx context.Context, req *TokenRefreshRequest) (*ChatGPTAuthTokens, error)
}

// ApprovalFuncs adapts plain functions to ApprovalHandler. A nil field falls
// back to the default behavior for that request.
type ApprovalFuncs struct {
	Command      func(ctx context.Context, req *CommandApprovalRequest) (Decision, error)
	FileChange   func(ctx context.Context, req *FileChangeApprovalRequest) (Decision, error)
	Permissions  func(ctx context.Context, req *PermissionsRequest) (*PermissionsResponse, error)
	UserInput    func(ctx context.Context, params json.RawMessage) (any, error)
	TokenRefresh func(ctx context.Context, req *TokenRefreshRequest) (*ChatGPTAuthTokens, error)
}

// ApproveCommand implements ApprovalHandler.
func (f ApprovalFuncs) ApproveCommand(ctx context.Context, req *CommandApprovalRequest) (Decision, error) {
	if f.Command == nil {
		return DecisionDecline, nil
	}
	return f.Command(ctx, req)
}

// ApproveFileChange implements ApprovalHandler.
func (f ApprovalFuncs) ApproveFileChange(ctx context.Context, req *FileChangeApprovalRequest) (Decision, error) {
	if f.FileChange == nil {
		return DecisionDecline, nil
	}
	return f.FileChange(ctx, req)
}

// ApprovePermissions implements PermissionApprover.
func (f ApprovalFuncs) ApprovePermissions(ctx context.Context, req *PermissionsRequest) (*PermissionsResponse, error) {
	if f.Permissions == nil {
		return &PermissionsResponse{Permissions: json.RawMessage("[]")}, nil
	}
	return f.Permissions(ctx, req)
}

// RequestUserInput implements UserInputResponder.
func (f ApprovalFuncs) RequestUserInput(ctx context.Context, params json.RawMessage) (any, error) {
	if f.UserInput == nil {
		return nil, &RPCError{Code: CodeMethodNotFound, Message: "codex: no user input handler registered"}
	}
	return f.UserInput(ctx, params)
}

// RefreshChatGPTTokens implements TokenRefresher.
func (f ApprovalFuncs) RefreshChatGPTTokens(ctx context.Context, req *TokenRefreshRequest) (*ChatGPTAuthTokens, error) {
	if f.TokenRefresh == nil {
		return nil, &RPCError{Code: CodeMethodNotFound, Message: "codex: no token refresh handler registered"}
	}
	return f.TokenRefresh(ctx, req)
}

// decisionResult is the JSON-RPC result of an approval request.
type decisionResult struct {
	Decision Decision `json:"decision"`
}

// pendingRequests tracks in-flight server requests so their handler contexts
// can be canceled when the owning turn ends.
type pendingRequests struct {
	mu     sync.Mutex
	next   int
	byTurn map[string]map[int]context.CancelFunc
}

func newPendingRequests() *pendingRequests {
	return &pendingRequests{byTurn: make(map[string]map[int]context.CancelFunc)}
}

// turnKey identifies a turn for pending-request bookkeeping.
func turnKey(threadID, turnID string) string { return threadID + "\x00" + turnID }

// add registers a cancel function and returns its release func.
func (p *pendingRequests) add(key string, cancel context.CancelFunc) func() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.next++
	id := p.next
	entries := p.byTurn[key]
	if entries == nil {
		entries = make(map[int]context.CancelFunc)
		p.byTurn[key] = entries
	}
	entries[id] = cancel
	return func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		if entries, ok := p.byTurn[key]; ok {
			delete(entries, id)
			if len(entries) == 0 {
				delete(p.byTurn, key)
			}
		}
	}
}

// cancelTurn cancels every handler context bound to a turn.
func (p *pendingRequests) cancelTurn(key string) {
	p.mu.Lock()
	entries := p.byTurn[key]
	delete(p.byTurn, key)
	p.mu.Unlock()
	for _, cancel := range entries {
		cancel()
	}
}

// cancelAll cancels every pending handler context.
func (p *pendingRequests) cancelAll() {
	p.mu.Lock()
	all := p.byTurn
	p.byTurn = make(map[string]map[int]context.CancelFunc)
	p.mu.Unlock()
	for _, entries := range all {
		for _, cancel := range entries {
			cancel()
		}
	}
}

// handleServerRequest answers a server-initiated request. Unknown methods are
// answered with a method-not-found error so a turn fails closed instead of
// hanging, and approvals default to declining when no handler is registered.
func (c *Client) handleServerRequest(ctx context.Context, method string, params json.RawMessage) (any, error) {
	threadID, turnID := routeIDs(params)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	release := c.pending.add(turnKey(threadID, turnID), cancel)
	defer release()

	switch method {
	case MethodCommandApproval:
		req := &CommandApprovalRequest{}
		if err := json.Unmarshal(params, req); err != nil {
			return nil, &RPCError{Code: CodeInvalidParams, Message: err.Error()}
		}
		req.Params = params
		decision := DecisionDecline
		if c.opts.Approvals != nil {
			var err error
			if decision, err = c.opts.Approvals.ApproveCommand(ctx, req); err != nil {
				return nil, err
			}
		}
		return decisionResult{Decision: decision}, nil

	case MethodFileChangeApproval:
		req := &FileChangeApprovalRequest{}
		if err := json.Unmarshal(params, req); err != nil {
			return nil, &RPCError{Code: CodeInvalidParams, Message: err.Error()}
		}
		req.Params = params
		decision := DecisionDecline
		if c.opts.Approvals != nil {
			var err error
			if decision, err = c.opts.Approvals.ApproveFileChange(ctx, req); err != nil {
				return nil, err
			}
		}
		return decisionResult{Decision: decision}, nil

	case MethodPermissionsApproval:
		req := &PermissionsRequest{}
		if err := json.Unmarshal(params, req); err != nil {
			return nil, &RPCError{Code: CodeInvalidParams, Message: err.Error()}
		}
		req.Params = params
		if approver, ok := c.opts.Approvals.(PermissionApprover); ok {
			return approver.ApprovePermissions(ctx, req)
		}
		// Fail closed: grant nothing.
		return &PermissionsResponse{Permissions: json.RawMessage("[]")}, nil

	case MethodRequestUserInput, MethodItemRequestUserInput:
		if responder, ok := c.opts.Approvals.(UserInputResponder); ok {
			return responder.RequestUserInput(ctx, params)
		}
		return nil, &RPCError{Code: CodeMethodNotFound, Message: "codex: no user input handler registered"}

	case MethodChatGPTTokenRefresh:
		if refresher, ok := c.opts.Approvals.(TokenRefresher); ok {
			req := &TokenRefreshRequest{}
			if err := json.Unmarshal(params, req); err != nil {
				return nil, &RPCError{Code: CodeInvalidParams, Message: err.Error()}
			}
			req.Params = params
			return refresher.RefreshChatGPTTokens(ctx, req)
		}
		return nil, &RPCError{Code: CodeMethodNotFound, Message: "codex: no token refresh handler registered"}

	default:
		return nil, &RPCError{Code: CodeMethodNotFound, Message: "codex: unhandled server request " + method}
	}
}
