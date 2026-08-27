package claude

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
)

// scriptedCLI answers the initialize handshake and then replays frames.
func scriptedCLI(t *testing.T, frames ...any) *fakeTransport {
	t.Helper()
	ft := newFakeTransport()
	ft.mu.Lock()
	ft.onWrite = func(frame map[string]any) {
		if frame["type"] != "control_request" {
			return
		}
		req, _ := frame["request"].(map[string]any)
		if req["subtype"] != "initialize" {
			return
		}
		ft.push(map[string]any{
			"type": "control_response",
			"response": map[string]any{
				"subtype":    "success",
				"request_id": frame["request_id"],
				"response":   map[string]any{"commands": []any{}},
			},
		})
		for _, f := range frames {
			ft.push(f)
		}
		ft.finish(nil)
	}
	ft.mu.Unlock()
	return ft
}

func assistantFrame(text string) map[string]any {
	return map[string]any{"type": "assistant", "message": map[string]any{
		"model":   "claude-opus-4-5",
		"content": []any{map[string]any{"type": "text", "text": text}},
	}}
}

func resultFrame() map[string]any {
	return map[string]any{"type": "result", "subtype": "success", "duration_ms": 5,
		"duration_api_ms": 4, "is_error": false, "num_turns": 1, "session_id": "s1"}
}

func TestQueryYieldsTypedMessages(t *testing.T) {
	t.Parallel()
	ft := scriptedCLI(t, assistantFrame("4"), resultFrame())
	var texts []string
	var result *ResultMessage
	for msg, err := range Query(t.Context(), "What is 2+2?", &Options{Transport: ft}) {
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		switch m := msg.(type) {
		case *AssistantMessage:
			texts = append(texts, m.Content[0].(*TextBlock).Text)
		case *ResultMessage:
			result = m
		}
	}
	if len(texts) != 1 || texts[0] != "4" {
		t.Fatalf("texts = %q", texts)
	}
	if result == nil || result.SessionID != "s1" {
		t.Fatalf("result = %+v", result)
	}

	// The prompt was written as a stream-json user message after initialize.
	frames := ft.frames(t)
	if len(frames) != 2 {
		t.Fatalf("frames = %#v", frames)
	}
	user := frames[1]
	if user["type"] != "user" {
		t.Fatalf("prompt frame = %#v", user)
	}
	if user["message"].(map[string]any)["content"] != "What is 2+2?" {
		t.Fatalf("prompt frame = %#v", user)
	}
	if !ft.endedInput() {
		t.Fatal("input should have been ended")
	}
}

func TestQueryStreamWritesEveryInput(t *testing.T) {
	t.Parallel()
	ft := scriptedCLI(t, resultFrame())
	inputs := func(yield func(UserInput) bool) {
		yield(UserInput{Content: "one", SessionID: "s1"})
		yield(UserInput{Content: "two", ParentToolUseID: "tu1", Origin: &MessageOrigin{Kind: OriginHuman}})
		yield(UserInput{Raw: map[string]any{"type": "user", "custom": true}})
	}
	for _, err := range QueryStream(t.Context(), inputs, &Options{Transport: ft}) {
		if err != nil {
			t.Fatalf("query: %v", err)
		}
	}
	frames := ft.frames(t)
	if len(frames) != 4 {
		t.Fatalf("frames = %#v", frames)
	}
	if frames[1]["session_id"] != "s1" {
		t.Fatalf("frame = %#v", frames[1])
	}
	if frames[2]["parent_tool_use_id"] != "tu1" {
		t.Fatalf("frame = %#v", frames[2])
	}
	if frames[2]["origin"].(map[string]any)["kind"] != OriginHuman {
		t.Fatalf("frame = %#v", frames[2])
	}
	if frames[3]["custom"] != true {
		t.Fatalf("raw frame = %#v", frames[3])
	}
}

func TestQueryEarlyBreakClosesTransport(t *testing.T) {
	before := runtime.NumGoroutine()
	ft := newFakeTransport()
	ft.mu.Lock()
	ft.onWrite = func(frame map[string]any) {
		if frame["type"] != "control_request" {
			return
		}
		ft.push(map[string]any{"type": "control_response", "response": map[string]any{
			"subtype": "success", "request_id": frame["request_id"], "response": map[string]any{}}})
		// A long stream the caller will abandon halfway through.
		for range 10 {
			ft.push(assistantFrame("chunk"))
		}
	}
	ft.mu.Unlock()

	count := 0
	for msg, err := range Query(t.Context(), "hi", &Options{Transport: ft}) {
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if _, ok := msg.(*AssistantMessage); ok {
			count++
			if count == 2 {
				break
			}
		}
	}
	if count != 2 {
		t.Fatalf("count = %d", count)
	}
	ft.mu.Lock()
	closed := ft.closed
	ft.mu.Unlock()
	if !closed {
		t.Fatal("breaking out of the loop should close the transport")
	}
	// Give the reader and writer goroutines a moment to unwind.
	for range 100 {
		if runtime.NumGoroutine() <= before+2 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("goroutines leaked: before=%d after=%d", before, runtime.NumGoroutine())
}

func TestQueryContextCancellation(t *testing.T) {
	t.Parallel()
	ft := newFakeTransport()
	ft.mu.Lock()
	ft.onWrite = func(frame map[string]any) {
		if frame["type"] != "control_request" {
			return
		}
		ft.push(map[string]any{"type": "control_response", "response": map[string]any{
			"subtype": "success", "request_id": frame["request_id"], "response": map[string]any{}}})
	}
	ft.mu.Unlock()

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range Query(ctx, "hi", &Options{Transport: ft}) {
		}
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	// Cancelling must unblock the consumer: the fake transport is closed by
	// the deferred cleanup, which ends the message stream.
	ft.finish(context.Canceled)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cancelling the context did not end the query")
	}
}

func TestQueryConnectError(t *testing.T) {
	t.Parallel()
	opts := &Options{CLIPath: "/nonexistent/claude/binary"}
	var got error
	for _, err := range Query(t.Context(), "hi", opts) {
		got = err
	}
	if got == nil {
		t.Fatal("expected a connect error")
	}
}

func TestQueryRejectsConflictingPermissionOptions(t *testing.T) {
	t.Parallel()
	opts := &Options{
		PermissionPromptToolName: "mcp__x__prompt",
		CanUseTool: func(context.Context, string, map[string]any, ToolPermissionContext) (PermissionResult, error) {
			return &PermissionResultAllow{}, nil
		},
	}
	var got error
	for _, err := range Query(t.Context(), "hi", opts) {
		got = err
	}
	if got == nil || !strings.Contains(got.Error(), "CanUseTool cannot be used with") {
		t.Fatalf("error = %v", got)
	}
}

func TestPrepareOptions(t *testing.T) {
	t.Parallel()
	// The permission callback routes prompts over the control protocol.
	opts, err := prepareOptions(&Options{CanUseTool: func(context.Context, string, map[string]any, ToolPermissionContext) (PermissionResult, error) {
		return &PermissionResultAllow{}, nil
	}}, entrypoint)
	if err != nil {
		t.Fatalf("prepareOptions: %v", err)
	}
	if opts.PermissionPromptToolName != "stdio" {
		t.Fatalf("permission prompt tool = %q", opts.PermissionPromptToolName)
	}
	if opts.Env["CLAUDE_CODE_ENTRYPOINT"] != entrypoint {
		t.Fatalf("entrypoint = %q", opts.Env["CLAUDE_CODE_ENTRYPOINT"])
	}

	// The caller's own entrypoint wins, and the original options are not
	// mutated.
	original := &Options{Env: map[string]string{"CLAUDE_CODE_ENTRYPOINT": "custom"}}
	opts, err = prepareOptions(original, entrypointClient)
	if err != nil {
		t.Fatalf("prepareOptions: %v", err)
	}
	if opts.Env["CLAUDE_CODE_ENTRYPOINT"] != "custom" {
		t.Fatalf("entrypoint = %q", opts.Env["CLAUDE_CODE_ENTRYPOINT"])
	}
	if len(original.Env) != 1 {
		t.Fatal("prepareOptions must not mutate the caller's options")
	}

	// nil options are usable.
	if opts, err = prepareOptions(nil, entrypoint); err != nil || opts == nil {
		t.Fatalf("prepareOptions(nil) = %v, %v", opts, err)
	}
}

func TestQueryPropagatesStreamError(t *testing.T) {
	t.Parallel()
	ft := newFakeTransport()
	ft.mu.Lock()
	ft.onWrite = func(frame map[string]any) {
		if frame["type"] != "control_request" {
			return
		}
		ft.push(map[string]any{"type": "control_response", "response": map[string]any{
			"subtype": "success", "request_id": frame["request_id"], "response": map[string]any{}}})
		code := 1
		ft.finish(NewProcessError("Command failed with exit code 1", &code, "bad"))
	}
	ft.mu.Unlock()

	var got error
	for _, err := range Query(t.Context(), "hi", &Options{Transport: ft}) {
		if err != nil {
			got = err
		}
	}
	var perr *ProcessError
	if !errors.As(got, &perr) {
		t.Fatalf("error = %T (%v)", got, got)
	}
}

func TestQueryInitializeFailure(t *testing.T) {
	t.Parallel()
	ft := newFakeTransport()
	ft.mu.Lock()
	ft.onWrite = func(frame map[string]any) {
		ft.push(map[string]any{"type": "control_response", "response": map[string]any{
			"subtype": "error", "request_id": frame["request_id"], "error": "unsupported CLI"}})
	}
	ft.mu.Unlock()

	var got error
	for _, err := range Query(t.Context(), "hi", &Options{Transport: ft}) {
		got = err
	}
	var ctrlErr *ControlError
	if !errors.As(got, &ctrlErr) {
		t.Fatalf("error = %T (%v)", got, got)
	}
}
