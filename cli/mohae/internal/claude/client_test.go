package claude

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// initResponder answers the initialize handshake with the given server info.
func initResponder(ft *fakeTransport, info map[string]any) {
	ft.mu.Lock()
	ft.onWrite = func(frame map[string]any) {
		if frame["type"] != "control_request" {
			return
		}
		req, _ := frame["request"].(map[string]any)
		payload := map[string]any{}
		if req["subtype"] == "initialize" {
			payload = info
		}
		ft.push(map[string]any{"type": "control_response", "response": map[string]any{
			"subtype": "success", "request_id": frame["request_id"], "response": payload}})
	}
	ft.mu.Unlock()
}

func connectedClient(t *testing.T, opts *Options) (*Client, *fakeTransport) {
	t.Helper()
	ft := newFakeTransport()
	initResponder(ft, map[string]any{"commands": []any{"/help"}, "output_style": "default"})
	if opts == nil {
		opts = &Options{}
	}
	opts.Transport = ft
	client := NewClient(opts)
	if err := client.Connect(t.Context()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect() })
	return client, ft
}

func TestClientConnectAndServerInfo(t *testing.T) {
	t.Parallel()
	client, ft := connectedClient(t, nil)
	if info := client.ServerInfo(); info == nil || info["output_style"] != "default" {
		t.Fatalf("server info = %#v", client.ServerInfo())
	}
	frames := ft.frames(t)
	if len(frames) != 1 || frames[0]["request"].(map[string]any)["subtype"] != "initialize" {
		t.Fatalf("frames = %#v", frames)
	}
	// The client entrypoint is reported to the CLI.
	if client.opts.Env["CLAUDE_CODE_ENTRYPOINT"] != "" {
		t.Fatal("the caller's options must not be mutated")
	}

	// Connecting twice is refused.
	if err := client.Connect(t.Context()); err == nil || !strings.Contains(err.Error(), "already connected") {
		t.Fatalf("error = %v", err)
	}
}

func TestClientConnectWithInitialPrompt(t *testing.T) {
	t.Parallel()
	ft := newFakeTransport()
	initResponder(ft, nil)
	client := NewClient(&Options{Transport: ft})
	if err := client.Connect(t.Context(), UserInput{Content: "hello"}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Disconnect()

	frames := ft.frames(t)
	if len(frames) != 2 || frames[1]["type"] != "user" {
		t.Fatalf("frames = %#v", frames)
	}
	if frames[1]["session_id"] != DefaultSessionID {
		t.Fatalf("session = %#v", frames[1])
	}
}

func TestClientMultiTurnReceiveResponse(t *testing.T) {
	t.Parallel()
	client, ft := connectedClient(t, nil)

	if err := client.Query(t.Context(), "first", ""); err != nil {
		t.Fatalf("query: %v", err)
	}
	ft.push(assistantFrame("one"))
	ft.push(resultFrame())
	// The second turn's messages are queued behind the first result and must
	// not be consumed by the first ReceiveResponse.
	ft.push(assistantFrame("two"))
	ft.push(resultFrame())

	var texts []string
	for msg, err := range client.ReceiveResponse(t.Context()) {
		if err != nil {
			t.Fatalf("receive: %v", err)
		}
		if am, ok := msg.(*AssistantMessage); ok {
			texts = append(texts, am.Content[0].(*TextBlock).Text)
		}
	}
	if len(texts) != 1 || texts[0] != "one" {
		t.Fatalf("first turn = %q", texts)
	}

	if err := client.Query(t.Context(), "second", "sess-2"); err != nil {
		t.Fatalf("query: %v", err)
	}
	texts = nil
	for msg, err := range client.ReceiveResponse(t.Context()) {
		if err != nil {
			t.Fatalf("receive: %v", err)
		}
		if am, ok := msg.(*AssistantMessage); ok {
			texts = append(texts, am.Content[0].(*TextBlock).Text)
		}
	}
	if len(texts) != 1 || texts[0] != "two" {
		t.Fatalf("second turn = %q", texts)
	}

	frames := ft.frames(t)
	last := frames[len(frames)-1]
	if last["session_id"] != "sess-2" {
		t.Fatalf("session = %#v", last)
	}
}

func TestClientQueryStream(t *testing.T) {
	t.Parallel()
	client, ft := connectedClient(t, nil)
	err := client.QueryStream(t.Context(), func(yield func(UserInput) bool) {
		yield(UserInput{Content: "a"})
		yield(UserInput{Content: "b", SessionID: "own"})
	}, "shared")
	if err != nil {
		t.Fatalf("query stream: %v", err)
	}
	frames := ft.frames(t)
	if frames[1]["session_id"] != "shared" || frames[2]["session_id"] != "own" {
		t.Fatalf("frames = %#v", frames[1:])
	}
}

func TestClientControlMethods(t *testing.T) {
	t.Parallel()
	client, ft := connectedClient(t, nil)
	ctx := t.Context()
	if err := client.Interrupt(ctx); err != nil {
		t.Fatalf("interrupt: %v", err)
	}
	if err := client.SetPermissionMode(ctx, PermissionModeAcceptEdits); err != nil {
		t.Fatalf("set permission mode: %v", err)
	}
	if err := client.SetModel(ctx, "sonnet"); err != nil {
		t.Fatalf("set model: %v", err)
	}
	if err := client.RewindFiles(ctx, "u1"); err != nil {
		t.Fatalf("rewind: %v", err)
	}
	if err := client.ReconnectMCPServer(ctx, "fs"); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if err := client.ToggleMCPServer(ctx, "fs", true); err != nil {
		t.Fatalf("toggle: %v", err)
	}
	if err := client.StopTask(ctx, "t1"); err != nil {
		t.Fatalf("stop task: %v", err)
	}
	if _, err := client.MCPServerStatus(ctx); err != nil {
		t.Fatalf("mcp status: %v", err)
	}
	if _, err := client.ContextUsage(ctx); err != nil {
		t.Fatalf("context usage: %v", err)
	}

	var subtypes []string
	for _, frame := range ft.frames(t) {
		if frame["type"] == "control_request" {
			subtypes = append(subtypes, frame["request"].(map[string]any)["subtype"].(string))
		}
	}
	want := []string{"initialize", "interrupt", "set_permission_mode", "set_model",
		"rewind_files", "mcp_reconnect", "mcp_toggle", "stop_task", "mcp_status", "get_context_usage"}
	if len(subtypes) != len(want) {
		t.Fatalf("subtypes = %q, want %q", subtypes, want)
	}
	for i := range want {
		if subtypes[i] != want[i] {
			t.Fatalf("subtypes = %q, want %q", subtypes, want)
		}
	}
}

func TestClientUseBeforeConnect(t *testing.T) {
	t.Parallel()
	client := NewClient(nil)
	ctx := t.Context()
	checks := map[string]error{
		"query":       client.Query(ctx, "hi", ""),
		"interrupt":   client.Interrupt(ctx),
		"setMode":     client.SetPermissionMode(ctx, PermissionModePlan),
		"setModel":    client.SetModel(ctx, "opus"),
		"rewind":      client.RewindFiles(ctx, "u1"),
		"reconnect":   client.ReconnectMCPServer(ctx, "fs"),
		"toggle":      client.ToggleMCPServer(ctx, "fs", true),
		"stopTask":    client.StopTask(ctx, "t1"),
		"queryStream": client.QueryStream(ctx, func(yield func(UserInput) bool) { yield(UserInput{}) }, ""),
	}
	for name, err := range checks {
		var connErr *ConnectionError
		if !errors.As(err, &connErr) {
			t.Errorf("%s: error = %T (%v), want *ConnectionError", name, err, err)
		}
	}
	if _, err := client.MCPServerStatus(ctx); err == nil {
		t.Error("mcp status should fail before connect")
	}
	if _, err := client.ContextUsage(ctx); err == nil {
		t.Error("context usage should fail before connect")
	}
	if client.ServerInfo() != nil {
		t.Error("server info should be nil before connect")
	}
	var got error
	for _, err := range client.ReceiveMessages(ctx) {
		got = err
	}
	var connErr *ConnectionError
	if !errors.As(got, &connErr) {
		t.Errorf("receive: error = %T (%v)", got, got)
	}
}

func TestClientConcurrentControlDuringReceive(t *testing.T) {
	t.Parallel()
	client, ft := connectedClient(t, nil)

	received := make(chan struct{})
	go func() {
		for msg := range client.ReceiveMessages(t.Context()) {
			if _, ok := msg.(*ResultMessage); ok {
				close(received)
				return
			}
		}
	}()

	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = client.Interrupt(t.Context())
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("interrupt %d: %v", i, err)
		}
	}
	ft.push(resultFrame())
	select {
	case <-received:
	case <-time.After(5 * time.Second):
		t.Fatal("receiver never saw the result")
	}
}

func TestClientDisconnectIsIdempotent(t *testing.T) {
	t.Parallel()
	client, ft := connectedClient(t, nil)
	if err := client.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	ft.mu.Lock()
	closed := ft.closed
	ft.mu.Unlock()
	if !closed {
		t.Fatal("disconnect should close the transport")
	}
	if err := client.Disconnect(); err != nil {
		t.Fatalf("second disconnect: %v", err)
	}
	if err := client.Query(t.Context(), "hi", ""); err == nil {
		t.Fatal("query after disconnect should fail")
	}
}

func TestClientReceiveHonorsContext(t *testing.T) {
	t.Parallel()
	client, _ := connectedClient(t, nil)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		var last error
		for _, err := range client.ReceiveMessages(ctx) {
			last = err
		}
		done <- last
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelling the context did not end the stream")
	}
}

func TestClientConnectFailurePropagates(t *testing.T) {
	t.Parallel()
	ft := newFakeTransport()
	ft.mu.Lock()
	ft.onWrite = func(frame map[string]any) {
		ft.push(map[string]any{"type": "control_response", "response": map[string]any{
			"subtype": "error", "request_id": frame["request_id"], "error": "handshake refused"}})
	}
	ft.mu.Unlock()
	client := NewClient(&Options{Transport: ft})
	err := client.Connect(t.Context())
	var ctrlErr *ControlError
	if !errors.As(err, &ctrlErr) {
		t.Fatalf("error = %T (%v)", err, err)
	}
	// A failed connect leaves the client usable for a retry, and closes the
	// transport it opened.
	ft.mu.Lock()
	closed := ft.closed
	ft.mu.Unlock()
	if !closed {
		t.Fatal("a failed connect should close the transport")
	}
	if client.ServerInfo() != nil {
		t.Fatal("server info should be nil after a failed connect")
	}
}

func TestClientHooksAndPermissionsRoundTrip(t *testing.T) {
	t.Parallel()
	hookRan := make(chan struct{}, 1)
	opts := &Options{
		CanUseTool: func(_ context.Context, tool string, _ map[string]any, _ ToolPermissionContext) (PermissionResult, error) {
			if tool == "Bash" {
				return &PermissionResultDeny{Message: "no"}, nil
			}
			return &PermissionResultAllow{}, nil
		},
		Hooks: map[HookEvent][]HookMatcher{
			HookPreToolUse: {{Hooks: []HookCallback{
				func(context.Context, map[string]any, string, HookContext) (HookOutput, error) {
					hookRan <- struct{}{}
					return HookOutput{}, nil
				},
			}}},
		},
	}
	client, ft := connectedClient(t, opts)
	// The permission callback implies the stdio permission prompt tool.
	c := client
	if c.opts.PermissionPromptToolName != "" {
		t.Fatal("the caller's options must not be mutated")
	}

	ft.mu.Lock()
	ft.onWrite = nil
	ft.mu.Unlock()
	ft.push(map[string]any{"type": "control_request", "request_id": "p1", "request": map[string]any{
		"subtype": "can_use_tool", "tool_name": "Bash", "input": map[string]any{}, "tool_use_id": "tu1"}})
	out := ft.nextResponse(t)["response"].(map[string]any)
	if out["behavior"] != "deny" {
		t.Fatalf("permission response = %#v", out)
	}

	ft.push(map[string]any{"type": "control_request", "request_id": "h1", "request": map[string]any{
		"subtype": "hook_callback", "callback_id": "hook_0", "input": map[string]any{}}})
	if resp := ft.nextResponse(t); resp["subtype"] != "success" {
		t.Fatalf("hook response = %#v", resp)
	}
	select {
	case <-hookRan:
	case <-time.After(5 * time.Second):
		t.Fatal("hook was not invoked")
	}
}
