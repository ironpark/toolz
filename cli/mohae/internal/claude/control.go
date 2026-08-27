package claude

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultControlTimeout bounds a control request that does not carry its own
// deadline.
const DefaultControlTimeout = 60 * time.Second

// messageBufferSize is how many messages the engine buffers ahead of the
// consumer.
const messageBufferSize = 100

// deferringTaskTypes are the task types whose completion runs a follow-up turn,
// so the input stream must stay open past the turn's result frame. Background
// shells and other open-ended tasks are deliberately excluded: they may never
// reach a terminal status, and holding input open for one would withhold it
// forever.
var deferringTaskTypes = map[string]bool{"local_agent": true, "local_workflow": true}

// ControlError reports a failure returned by the CLI for a control request.
type ControlError struct {
	baseError
}

// ServerInfo is the CLI's response to the initialize handshake: supported
// commands, output styles and other capability metadata, passed through as-is.
type ServerInfo map[string]any

// engine speaks the control protocol on top of a Transport: it demultiplexes
// the CLI's output into typed messages, answers the control requests the CLI
// sends (permissions, hooks, in-process MCP traffic) and correlates the control
// requests the SDK sends with their responses.
type engine struct {
	transport Transport
	opts      *Options

	// mcpRouter answers mcp_message control requests. It is nil until SDK MCP
	// servers are registered, in which case such requests are refused with a
	// JSON-RPC method-not-found error.
	mcpRouter func(ctx context.Context, serverName string, message json.RawMessage) (json.RawMessage, error)

	hookCallbacks map[string]HookCallback

	mu           sync.Mutex
	counter      int
	pending      map[string]chan controlResult
	inflight     map[string]context.CancelFunc
	inflightTask map[string]bool
	lastErrorRes map[string]any
	serverInfo   ServerInfo

	handlers sync.WaitGroup

	messages chan messageOrError

	startOnce  sync.Once
	closeOnce  sync.Once
	closed     chan struct{}
	readerDone chan struct{}

	firstResultOnce sync.Once
	firstResult     chan struct{}
}

type messageOrError struct {
	msg Message
	err error
}

type controlResult struct {
	response map[string]any
	err      error
}

// newEngine builds an engine over transport. opts may be nil.
func newEngine(transport Transport, opts *Options) *engine {
	if opts == nil {
		opts = &Options{}
	}
	return &engine{
		transport:     transport,
		opts:          opts,
		hookCallbacks: map[string]HookCallback{},
		pending:       map[string]chan controlResult{},
		inflight:      map[string]context.CancelFunc{},
		inflightTask:  map[string]bool{},
		messages:      make(chan messageOrError, messageBufferSize),
		closed:        make(chan struct{}),
		readerDone:    make(chan struct{}),
		firstResult:   make(chan struct{}),
	}
}

// Start begins reading from the transport. It is safe to call more than once.
func (e *engine) Start(ctx context.Context) {
	e.startOnce.Do(func() {
		go e.readLoop(context.WithoutCancel(ctx))
	})
}

// ServerInfo reports the initialize response, or nil before Initialize.
func (e *engine) ServerInfo() ServerInfo {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.serverInfo
}

// Messages yields every non-control message the CLI produced, in order. The
// sequence ends when the CLI's output ends; a fatal error is the last item.
func (e *engine) Messages() iter.Seq2[Message, error] {
	return func(yield func(Message, error) bool) {
		for item := range e.messages {
			if !yield(item.msg, item.err) {
				return
			}
			if item.err != nil {
				return
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Reader
// ---------------------------------------------------------------------------

// readLoop demultiplexes the transport's frames until the CLI's output ends.
func (e *engine) readLoop(ctx context.Context) {
	defer close(e.readerDone)
	defer close(e.messages)
	defer e.signalFirstResult()

	for raw, err := range e.transport.ReadMessages() {
		if err != nil {
			e.failAll(err)
			return
		}
		if e.isClosed() {
			return
		}
		var frame map[string]any
		if uerr := json.Unmarshal(raw, &frame); uerr != nil {
			e.failAll(NewJSONDecodeError(string(raw), uerr))
			return
		}
		if done := e.route(ctx, frame, raw); done {
			return
		}
	}
}

// route handles one decoded frame, reporting whether the read loop should stop.
func (e *engine) route(ctx context.Context, frame map[string]any, raw json.RawMessage) bool {
	switch str(frame["type"]) {
	case "control_response":
		response, _ := frame["response"].(map[string]any)
		e.deliverControlResponse(response)
		return false
	case "control_request":
		if !e.isClosed() {
			e.spawnControlHandler(ctx, frame)
		}
		return false
	case "control_cancel_request":
		if id := str(frame["request_id"]); id != "" {
			e.mu.Lock()
			cancel := e.inflight[id]
			delete(e.inflight, id)
			e.mu.Unlock()
			if cancel != nil {
				cancel()
			}
		}
		return false
	case "transcript_mirror":
		// The session-store subsystem is not ported; these frames carry no
		// information for consumers.
		return false
	case "system":
		e.trackTaskLifecycle(frame)
	case "result":
		e.noteResult(frame)
	}

	if str(frame["type"]) != "result" && !isSessionStateChanged(frame) {
		// The conversation moved on, so a later non-zero exit is a fresh
		// failure rather than the expected exit after an error result.
		e.mu.Lock()
		e.lastErrorRes = nil
		e.mu.Unlock()
	}

	msg, err := parseMessageMap(frame, raw)
	if err != nil {
		e.failAll(err)
		return true
	}
	if msg == nil {
		return false
	}
	return !e.emit(messageOrError{msg: msg})
}

func isSessionStateChanged(frame map[string]any) bool {
	return str(frame["type"]) == "system" && str(frame["subtype"]) == "session_state_changed"
}

// emit hands one item to the consumer, reporting whether it was accepted.
func (e *engine) emit(item messageOrError) bool {
	select {
	case e.messages <- item:
		return true
	case <-e.closed:
		return false
	}
}

// noteResult records a run-ending result and releases the input-stream hold.
func (e *engine) noteResult(frame map[string]any) {
	e.mu.Lock()
	inflight := len(e.inflightTask)
	if isErr, _ := frame["is_error"].(bool); isErr {
		e.lastErrorRes = frame
	} else {
		e.lastErrorRes = nil
	}
	e.mu.Unlock()
	if inflight == 0 {
		// A result with tasks still in flight ends one turn, not the run:
		// those tasks still need the input stream for control responses.
		e.signalFirstResult()
	}
}

func (e *engine) signalFirstResult() {
	e.firstResultOnce.Do(func() { close(e.firstResult) })
}

// trackTaskLifecycle keeps the set of delegated tasks that are still running.
func (e *engine) trackTaskLifecycle(frame map[string]any) {
	taskID := str(frame["task_id"])
	if taskID == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	switch str(frame["subtype"]) {
	case "task_started":
		if deferringTaskTypes[str(frame["task_type"])] {
			e.inflightTask[taskID] = true
		}
	case "task_notification":
		delete(e.inflightTask, taskID)
	case "task_updated":
		patch, _ := frame["patch"].(map[string]any)
		if TerminalTaskStatuses[str(patch["status"])] {
			delete(e.inflightTask, taskID)
		}
	}
}

// failAll turns a fatal read error into the stream's final item and fails every
// control request still waiting for a response.
func (e *engine) failAll(err error) {
	var perr *ProcessError
	if errors.As(err, &perr) {
		e.mu.Lock()
		last := e.lastErrorRes
		e.mu.Unlock()
		if last != nil {
			// The CLI exits non-zero on purpose after reporting an error
			// result; the generic exit-code error carries nothing the result
			// does not already say.
			err = NewResultError(
				"Claude Code returned an error result: "+errorResultText(last),
				last, perr.ExitCode)
		}
	}

	e.mu.Lock()
	pending := e.pending
	e.pending = map[string]chan controlResult{}
	e.mu.Unlock()
	for _, ch := range pending {
		select {
		case ch <- controlResult{err: err}:
		default:
		}
	}
	e.emit(messageOrError{err: err})
}

// errorResultText picks the most informative text out of a failed result frame.
func errorResultText(frame map[string]any) string {
	if errs := normalizeResultErrors(frame["errors"]); len(errs) > 0 {
		return strings.Join(errs, "; ")
	}
	if result := strings.TrimSpace(str(frame["result"])); result != "" {
		return result
	}
	if subtype := str(frame["subtype"]); subtype != "" && subtype != "success" {
		return subtype
	}
	if status, ok := toInt(frame["api_error_status"]); ok {
		return fmt.Sprintf("API error (HTTP %d)", status)
	}
	return "unknown error"
}

// ---------------------------------------------------------------------------
// Outgoing control requests
// ---------------------------------------------------------------------------

// deliverControlResponse matches a response to its pending request.
func (e *engine) deliverControlResponse(response map[string]any) {
	if response == nil {
		return
	}
	id := str(response["request_id"])
	e.mu.Lock()
	ch, ok := e.pending[id]
	delete(e.pending, id)
	e.mu.Unlock()
	if !ok {
		return
	}
	if str(response["subtype"]) == "error" {
		msg := str(response["error"])
		if msg == "" {
			msg = "Unknown error"
		}
		ch <- controlResult{err: &ControlError{baseError{Msg: msg}}}
		return
	}
	payload, _ := response["response"].(map[string]any)
	ch <- controlResult{response: payload}
}

// sendControlRequest writes one control request and waits for its response.
func (e *engine) sendControlRequest(ctx context.Context, request map[string]any, timeout time.Duration) (map[string]any, error) {
	if timeout <= 0 {
		timeout = DefaultControlTimeout
	}
	e.mu.Lock()
	e.counter++
	id := "req_" + strconv.Itoa(e.counter) + "_" + randomHex(4)
	ch := make(chan controlResult, 1)
	e.pending[id] = ch
	e.mu.Unlock()

	cleanup := func() {
		e.mu.Lock()
		delete(e.pending, id)
		e.mu.Unlock()
	}

	frame := map[string]any{"type": "control_request", "request_id": id, "request": request}
	payload, err := json.Marshal(frame)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("claude: encoding control request: %w", err)
	}
	if err := e.transport.Write(ctx, payload); err != nil {
		cleanup()
		return nil, err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case res := <-ch:
		if res.err != nil {
			return nil, res.err
		}
		return res.response, nil
	case <-ctx.Done():
		cleanup()
		return nil, ctx.Err()
	case <-timer.C:
		cleanup()
		return nil, &ControlError{baseError{Msg: "Control request timeout: " + str(request["subtype"])}}
	case <-e.closed:
		cleanup()
		return nil, NewConnectionError("connection closed while awaiting a control response")
	}
}

func randomHex(n int) string {
	buf := make([]byte, n)
	// crypto/rand.Read never fails on supported platforms.
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

// Initialize performs the initialize handshake, registering hooks and
// capabilities, and stores the response for ServerInfo.
func (e *engine) Initialize(ctx context.Context) (ServerInfo, error) {
	request := map[string]any{"subtype": "initialize"}

	hooksConfig := map[string]any{}
	e.hookCallbacks = map[string]HookCallback{}
	next := 0
	for _, event := range slices.Sorted(maps.Keys(e.opts.Hooks)) {
		matchers := e.opts.Hooks[event]
		if len(matchers) == 0 {
			continue
		}
		configs := make([]map[string]any, 0, len(matchers))
		for _, matcher := range matchers {
			ids := make([]string, 0, len(matcher.Hooks))
			for _, cb := range matcher.Hooks {
				id := "hook_" + strconv.Itoa(next)
				next++
				e.hookCallbacks[id] = cb
				ids = append(ids, id)
			}
			config := map[string]any{"matcher": nil, "hookCallbackIds": ids}
			if matcher.Matcher != "" {
				config["matcher"] = matcher.Matcher
			}
			if matcher.Timeout > 0 {
				config["timeout"] = matcher.Timeout
			}
			configs = append(configs, config)
		}
		hooksConfig[event] = configs
	}
	if len(hooksConfig) > 0 {
		request["hooks"] = hooksConfig
	} else {
		request["hooks"] = nil
	}
	if len(e.opts.Agents) > 0 {
		request["agents"] = e.opts.Agents
	}
	if preset, ok := e.opts.SystemPrompt.(*SystemPromptPreset); ok && preset.ExcludeDynamicSections {
		request["excludeDynamicSections"] = true
	}
	// "all" and "unset" are the same on the wire (no filter), so only an
	// explicit list is sent.
	if list, ok := e.opts.Skills.(SkillList); ok {
		request["skills"] = []string(list)
	}
	if e.opts.ForwardSubagentText {
		request["forwardSubagentText"] = true
	}

	response, err := e.sendControlRequest(ctx, request, DefaultControlTimeout)
	if err != nil {
		return nil, err
	}
	info := ServerInfo(response)
	e.mu.Lock()
	e.serverInfo = info
	e.mu.Unlock()
	return info, nil
}

// Interrupt aborts the current turn.
func (e *engine) Interrupt(ctx context.Context) error {
	_, err := e.sendControlRequest(ctx, map[string]any{"subtype": "interrupt"}, 0)
	return err
}

// SetPermissionMode changes the session's permission mode.
func (e *engine) SetPermissionMode(ctx context.Context, mode PermissionMode) error {
	_, err := e.sendControlRequest(ctx,
		map[string]any{"subtype": "set_permission_mode", "mode": mode}, 0)
	return err
}

// SetModel changes the model. An empty model restores the CLI default.
func (e *engine) SetModel(ctx context.Context, model string) error {
	var value any
	if model != "" {
		value = model
	}
	_, err := e.sendControlRequest(ctx,
		map[string]any{"subtype": "set_model", "model": value}, 0)
	return err
}

// RewindFiles restores tracked files to their state at a user message. It
// requires Options.EnableFileCheckpointing.
func (e *engine) RewindFiles(ctx context.Context, userMessageID string) error {
	_, err := e.sendControlRequest(ctx,
		map[string]any{"subtype": "rewind_files", "user_message_id": userMessageID}, 0)
	return err
}

// MCPStatus reports the connection status of every configured MCP server.
func (e *engine) MCPStatus(ctx context.Context) (map[string]any, error) {
	return e.sendControlRequest(ctx, map[string]any{"subtype": "mcp_status"}, 0)
}

// ContextUsage reports the context window usage breakdown.
func (e *engine) ContextUsage(ctx context.Context) (map[string]any, error) {
	return e.sendControlRequest(ctx, map[string]any{"subtype": "get_context_usage"}, 0)
}

// ReconnectMCPServer reconnects a disconnected or failed MCP server.
func (e *engine) ReconnectMCPServer(ctx context.Context, serverName string) error {
	_, err := e.sendControlRequest(ctx,
		map[string]any{"subtype": "mcp_reconnect", "serverName": serverName}, 0)
	return err
}

// ToggleMCPServer enables or disables an MCP server.
func (e *engine) ToggleMCPServer(ctx context.Context, serverName string, enabled bool) error {
	_, err := e.sendControlRequest(ctx,
		map[string]any{"subtype": "mcp_toggle", "serverName": serverName, "enabled": enabled}, 0)
	return err
}

// StopTask stops a running background task.
func (e *engine) StopTask(ctx context.Context, taskID string) error {
	_, err := e.sendControlRequest(ctx,
		map[string]any{"subtype": "stop_task", "task_id": taskID}, 0)
	return err
}

// ---------------------------------------------------------------------------
// Incoming control requests
// ---------------------------------------------------------------------------

// spawnControlHandler answers one control request from the CLI in its own
// goroutine, so a slow callback cannot stall the read loop.
func (e *engine) spawnControlHandler(ctx context.Context, frame map[string]any) {
	requestID := str(frame["request_id"])
	handlerCtx, cancel := context.WithCancel(ctx)
	e.mu.Lock()
	e.inflight[requestID] = cancel
	e.mu.Unlock()

	e.handlers.Add(1)
	go func() {
		defer e.handlers.Done()
		defer cancel()
		defer func() {
			e.mu.Lock()
			delete(e.inflight, requestID)
			e.mu.Unlock()
		}()
		e.handleControlRequest(handlerCtx, requestID, frame)
	}()
}

func (e *engine) handleControlRequest(ctx context.Context, requestID string, frame map[string]any) {
	request, _ := frame["request"].(map[string]any)
	data, err := e.dispatchControlRequest(ctx, request)
	if ctx.Err() != nil {
		// The CLI cancelled the request and is no longer listening for a
		// reply.
		return
	}
	var reply map[string]any
	if err != nil {
		reply = map[string]any{
			"type": "control_response",
			"response": map[string]any{
				"subtype":    "error",
				"request_id": requestID,
				"error":      err.Error(),
			},
		}
	} else {
		if data == nil {
			data = map[string]any{}
		}
		reply = map[string]any{
			"type": "control_response",
			"response": map[string]any{
				"subtype":    "success",
				"request_id": requestID,
				"response":   data,
			},
		}
	}
	payload, merr := json.Marshal(reply)
	if merr != nil {
		return
	}
	_ = e.transport.Write(ctx, payload)
}

// dispatchControlRequest runs the handler for one control request subtype.
func (e *engine) dispatchControlRequest(ctx context.Context, request map[string]any) (map[string]any, error) {
	switch subtype := str(request["subtype"]); subtype {
	case "can_use_tool":
		return e.handleCanUseTool(ctx, request)
	case "hook_callback":
		return e.handleHookCallback(ctx, request)
	case "mcp_message":
		return e.handleMCPMessage(ctx, request)
	default:
		return nil, fmt.Errorf("Unsupported control request subtype: %s", subtype)
	}
}

func (e *engine) handleCanUseTool(ctx context.Context, request map[string]any) (data map[string]any, err error) {
	if e.opts.CanUseTool == nil {
		return nil, errors.New("canUseTool callback is not provided")
	}
	input, _ := request["input"].(map[string]any)
	permCtx := ToolPermissionContext{
		ToolUseID:      str(request["tool_use_id"]),
		AgentID:        str(request["agent_id"]),
		BlockedPath:    str(request["blocked_path"]),
		DecisionReason: str(request["decision_reason"]),
		Title:          str(request["title"]),
		DisplayName:    str(request["display_name"]),
		Description:    str(request["description"]),
	}
	if suggestions, ok := request["permission_suggestions"].([]any); ok {
		for _, s := range suggestions {
			raw, merr := json.Marshal(s)
			if merr != nil {
				continue
			}
			var update PermissionUpdate
			if json.Unmarshal(raw, &update) == nil {
				permCtx.Suggestions = append(permCtx.Suggestions, update)
			}
		}
	}

	// A panicking callback becomes an error response rather than taking the
	// process down.
	defer func() {
		if r := recover(); r != nil {
			data, err = nil, fmt.Errorf("can_use_tool callback panicked: %v", r)
		}
	}()

	result, err := e.opts.CanUseTool(ctx, str(request["tool_name"]), input, permCtx)
	if err != nil {
		return nil, err
	}
	switch r := result.(type) {
	case *PermissionResultAllow:
		updated := r.UpdatedInput
		if updated == nil {
			updated = input
		}
		out := map[string]any{"behavior": "allow", "updatedInput": updated}
		if r.UpdatedPermissions != nil {
			out["updatedPermissions"] = r.UpdatedPermissions
		}
		return out, nil
	case *PermissionResultDeny:
		out := map[string]any{"behavior": "deny", "message": r.Message}
		if r.Interrupt {
			out["interrupt"] = true
		}
		return out, nil
	default:
		return nil, fmt.Errorf("permission callback returned %T, want *PermissionResultAllow or *PermissionResultDeny", result)
	}
}

func (e *engine) handleHookCallback(ctx context.Context, request map[string]any) (data map[string]any, err error) {
	id := str(request["callback_id"])
	callback, ok := e.hookCallbacks[id]
	if !ok {
		return nil, fmt.Errorf("No hook callback found for ID: %s", id)
	}
	input, _ := request["input"].(map[string]any)

	defer func() {
		if r := recover(); r != nil {
			data, err = nil, fmt.Errorf("hook callback panicked: %v", r)
		}
	}()

	out, err := callback(ctx, input, str(request["tool_use_id"]), HookContext{})
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("claude: encoding hook output: %w", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(payload, &wire); err != nil {
		return nil, fmt.Errorf("claude: encoding hook output: %w", err)
	}
	return wire, nil
}

func (e *engine) handleMCPMessage(ctx context.Context, request map[string]any) (map[string]any, error) {
	serverName := str(request["server_name"])
	message, ok := request["message"]
	if serverName == "" || !ok || message == nil {
		return nil, errors.New("Missing server_name or message for MCP request")
	}
	raw, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("claude: encoding mcp message: %w", err)
	}
	var id any
	if m, ok := message.(map[string]any); ok {
		id = m["id"]
	}
	if e.mcpRouter == nil {
		return map[string]any{"mcp_response": jsonRPCError(id, -32601,
			fmt.Sprintf("Server '%s' not found", serverName))}, nil
	}
	response, err := e.mcpRouter(ctx, serverName, raw)
	if err != nil {
		return map[string]any{"mcp_response": jsonRPCError(id, -32603, err.Error())}, nil
	}
	if response == nil {
		// A JSON-RPC notification gets no reply, but the control request
		// that carried it still expects an acknowledgement.
		return map[string]any{"mcp_response": map[string]any{"jsonrpc": "2.0", "result": map[string]any{}}}, nil
	}
	var decoded any
	if err := json.Unmarshal(response, &decoded); err != nil {
		return nil, fmt.Errorf("claude: decoding mcp response: %w", err)
	}
	return map[string]any{"mcp_response": decoded}, nil
}

func jsonRPCError(id any, code int, message string) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]any{"code": code, "message": message},
	}
}

// ---------------------------------------------------------------------------
// Input
// ---------------------------------------------------------------------------

// hasBidirectionalNeeds reports whether the CLI may still send control requests
// that need a reply, in which case the input stream must stay open.
func (e *engine) hasBidirectionalNeeds() bool {
	return e.opts.CanUseTool != nil || len(e.opts.Hooks) > 0 || e.mcpRouter != nil
}

// StreamInput writes each user-message frame and then ends the input stream.
func (e *engine) StreamInput(ctx context.Context, inputs iter.Seq[map[string]any]) error {
	written := 0
	var writeErr error
	for input := range inputs {
		if e.isClosed() {
			break
		}
		payload, err := json.Marshal(input)
		if err != nil {
			writeErr = fmt.Errorf("claude: encoding user message: %w", err)
			break
		}
		if err := e.transport.Write(ctx, payload); err != nil {
			writeErr = err
			break
		}
		written++
	}
	if written == 0 {
		// Nothing was sent, so no result will arrive to release the hold.
		if err := e.transport.EndInput(); err != nil && writeErr == nil {
			writeErr = err
		}
		return writeErr
	}
	if err := e.waitForResultAndEndInput(ctx); err != nil && writeErr == nil {
		writeErr = err
	}
	return writeErr
}

// waitForResultAndEndInput closes the input stream, first waiting for a
// run-ending result when the session still needs to answer control requests.
func (e *engine) waitForResultAndEndInput(ctx context.Context) error {
	if e.hasBidirectionalNeeds() {
		select {
		case <-e.firstResult:
		case <-e.closed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return e.transport.EndInput()
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

func (e *engine) isClosed() bool {
	select {
	case <-e.closed:
		return true
	default:
		return false
	}
}

// Close stops the engine: it releases anything blocked on the message stream,
// waits for in-flight control handlers, and closes the transport.
func (e *engine) Close() error {
	var err error
	e.closeOnce.Do(func() {
		close(e.closed)
		e.signalFirstResult()
		err = e.transport.Close()
		select {
		case <-e.readerDone:
		case <-time.After(5 * time.Second):
			// A transport whose Close leaves the reader blocked must not
			// wedge the caller; the goroutine ends when its stream does.
		}
		e.handlers.Wait()
	})
	return err
}
