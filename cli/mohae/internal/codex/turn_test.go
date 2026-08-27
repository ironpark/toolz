package codex

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
)

// startThread starts a thread against the fake server and returns its id.
func startThread(t *testing.T, client *Client, server *fakeServer, id string) string {
	t.Helper()
	done := serve(t, func() {
		req := server.expect("thread/start")
		server.respond(req, map[string]any{"thread": map[string]any{"id": id}})
	})
	thread, err := client.StartThread(context.Background(), StartThreadParams{})
	<-done
	if err != nil {
		t.Fatalf("StartThread: %v", err)
	}
	return thread.ID
}

// startTurn starts a turn and answers turn/start with the given turn id.
func startTurn(t *testing.T, client *Client, server *fakeServer, threadID, turnID string, input []InputItem) *TurnStream {
	t.Helper()
	done := serve(t, func() {
		req := server.expect("turn/start")
		server.respond(req, map[string]any{"turn": map[string]any{
			"id": turnID, "status": "inProgress", "items": []any{}, "error": nil,
		}})
	})
	stream, err := client.StartTurn(context.Background(), threadID, input, nil)
	<-done
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	return stream
}

func recvEvent(t *testing.T, stream *TurnStream) (Event, bool) {
	t.Helper()
	select {
	case event, ok := <-stream.Events():
		return event, ok
	case <-time.After(fakeTimeout):
		t.Fatal("timed out waiting for a turn event")
		return Event{}, false
	}
}

func TestTurnStreamEndToEnd(t *testing.T) {
	client, server := connect(t, Options{})
	threadID := startThread(t, client, server, "thr_1")

	done := serve(t, func() {
		req := server.expect("turn/start")
		var params struct {
			ThreadID string            `json:"threadId"`
			Input    []json.RawMessage `json:"input"`
			Model    string            `json:"model"`
			Effort   string            `json:"effort"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			t.Errorf("params: %v", err)
		}
		if params.ThreadID != "thr_1" || params.Model != "gpt-5.6-terra" {
			t.Errorf("params = %+v", params)
		}
		if !strings.Contains(string(req.Params), `"text":"Run tests"`) {
			t.Errorf("input missing: %s", req.Params)
		}
		server.respond(req, map[string]any{"turn": map[string]any{
			"id": "turn_1", "status": "inProgress", "items": []any{}, "error": nil,
		}})
	})

	stream, err := client.StartTurn(context.Background(), threadID, Text("Run tests"),
		&TurnOptions{Model: "gpt-5.6-terra", Effort: "medium"})
	<-done
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	if stream.TurnID() != "turn_1" || stream.ThreadID() != "thr_1" {
		t.Fatalf("stream ids = %q %q", stream.ThreadID(), stream.TurnID())
	}

	server.notify(MethodTurnStarted, map[string]any{"threadId": "thr_1",
		"turn": map[string]any{"id": "turn_1", "status": "inProgress"}})
	server.notify(MethodItemStarted, map[string]any{"threadId": "thr_1", "turnId": "turn_1",
		"item": map[string]any{"type": "agentMessage", "id": "item_1", "text": ""}})
	server.notify(MethodAgentMessageDelta, map[string]any{"threadId": "thr_1", "turnId": "turn_1",
		"itemId": "item_1", "delta": "Hel"})
	server.notify(MethodAgentMessageDelta, map[string]any{"threadId": "thr_1", "turnId": "turn_1",
		"itemId": "item_1", "delta": "lo"})
	server.notify(MethodReasoningSummaryTextDelta, map[string]any{"threadId": "thr_1", "turnId": "turn_1",
		"itemId": "item_2", "delta": "thinking", "summaryIndex": 1})
	server.notify(MethodCommandExecutionOutputDlta, map[string]any{"threadId": "thr_1", "turnId": "turn_1",
		"itemId": "item_3", "stream": "stdout", "delta": "ok\n"})
	server.notify(MethodTurnPlan, map[string]any{"threadId": "thr_1", "turnId": "turn_1",
		"explanation": "why", "plan": []any{map[string]any{"step": "a", "status": "completed"}}})
	server.notify(MethodTurnDiff, map[string]any{"threadId": "thr_1", "turnId": "turn_1", "diff": "@@ -1 +1 @@"})
	server.notify(MethodTokenUsageUpdated, map[string]any{"threadId": "thr_1", "turnId": "turn_1",
		"usage": map[string]any{"inputTokens": 10, "outputTokens": 5, "totalTokens": 15}})
	server.notify(MethodItemCompleted, map[string]any{"threadId": "thr_1", "turnId": "turn_1",
		"item": map[string]any{"type": "agentMessage", "id": "item_1", "text": "Hello"}})
	server.notify(MethodTurnCompleted, map[string]any{"threadId": "thr_1",
		"turn": map[string]any{"id": "turn_1", "status": "completed", "items": []any{}}})

	var (
		kinds     []EventKind
		accum     strings.Builder
		finalText string
	)
	for {
		event, ok := recvEvent(t, stream)
		if !ok {
			break
		}
		kinds = append(kinds, event.Kind)
		switch event.Kind {
		case EventAgentMessageDelta:
			accum.WriteString(event.Delta)
		case EventItemCompleted:
			if msg, ok := event.Item.Item.(*AgentMessageItem); ok {
				finalText = msg.Text
			}
		case EventReasoningDelta:
			if !event.ReasoningSummary || event.SummaryIndex != 1 {
				t.Errorf("reasoning delta = %+v", event)
			}
		case EventCommandOutputDelta:
			if event.Stream != "stdout" || event.Delta != "ok\n" {
				t.Errorf("command delta = %+v", event)
			}
		case EventPlanUpdated:
			if len(event.Plan) != 1 || event.Explanation != "why" {
				t.Errorf("plan = %+v", event)
			}
		case EventDiffUpdated:
			if event.Diff == "" {
				t.Error("empty diff")
			}
		case EventTokenUsageUpdated:
			if event.Usage == nil || event.Usage.TotalTokens != 15 {
				t.Errorf("usage = %+v", event.Usage)
			}
		}
		if event.ThreadID != "thr_1" || event.TurnID != "turn_1" {
			t.Errorf("event ids = %q %q (%s)", event.ThreadID, event.TurnID, event.Kind)
		}
	}

	want := []EventKind{
		EventTurnStarted, EventItemStarted, EventAgentMessageDelta, EventAgentMessageDelta,
		EventReasoningDelta, EventCommandOutputDelta, EventPlanUpdated, EventDiffUpdated,
		EventTokenUsageUpdated, EventItemCompleted, EventTurnCompleted,
	}
	if len(kinds) != len(want) {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("kinds = %v, want %v", kinds, want)
		}
	}
	if accum.String() != "Hello" || finalText != "Hello" {
		t.Fatalf("deltas = %q, final = %q", accum.String(), finalText)
	}

	final, err := stream.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if final == nil || final.Status != TurnCompleted {
		t.Fatalf("final = %+v", final)
	}
}

func TestTurnEventsBeforeStartResponse(t *testing.T) {
	client, server := connect(t, Options{})
	threadID := startThread(t, client, server, "thr_1")

	done := serve(t, func() {
		req := server.expect("turn/start")
		// Events can race ahead of the response.
		server.notify(MethodTurnStarted, map[string]any{"threadId": "thr_1",
			"turn": map[string]any{"id": "turn_1", "status": "inProgress"}})
		time.Sleep(20 * time.Millisecond)
		server.respond(req, map[string]any{"turn": map[string]any{"id": "turn_1", "status": "inProgress"}})
	})

	stream, err := client.StartTurn(context.Background(), threadID, Text("hi"), nil)
	<-done
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}

	event, ok := recvEvent(t, stream)
	if !ok || event.Kind != EventTurnStarted {
		t.Fatalf("event = %+v (ok=%v)", event, ok)
	}
	if stream.TurnID() != "turn_1" {
		t.Fatalf("turn id = %q", stream.TurnID())
	}
}

func TestTurnStartError(t *testing.T) {
	client, server := connect(t, Options{})
	threadID := startThread(t, client, server, "thr_1")

	done := serve(t, func() {
		req := server.expect("turn/start")
		server.respondError(req, CodeServerOverloaded, "Server overloaded; retry later.")
	})
	stream, err := client.StartTurn(context.Background(), threadID, Text("hi"), nil)
	<-done
	if err == nil {
		t.Fatal("StartTurn succeeded")
	}
	if !IsOverloaded(err) {
		t.Fatalf("err = %v, want overload", err)
	}
	if stream != nil {
		t.Fatal("stream returned with an error")
	}
}

func TestTurnInterrupt(t *testing.T) {
	client, server := connect(t, Options{})
	threadID := startThread(t, client, server, "thr_1")
	stream := startTurn(t, client, server, threadID, "turn_1", Text("hi"))

	done := serve(t, func() {
		req := server.expect("turn/interrupt")
		var params InterruptTurnParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			t.Errorf("params: %v", err)
		}
		if params.ThreadID != "thr_1" || params.TurnID != "turn_1" {
			t.Errorf("params = %+v", params)
		}
		server.respond(req, map[string]any{})
		server.notify(MethodTurnCompleted, map[string]any{"threadId": "thr_1",
			"turn": map[string]any{"id": "turn_1", "status": "interrupted"}})
	})

	if err := client.InterruptTurn(context.Background(), threadID, "turn_1"); err != nil {
		t.Fatalf("InterruptTurn: %v", err)
	}
	<-done

	event, ok := recvEvent(t, stream)
	if !ok || event.Kind != EventTurnCompleted || event.Turn.Status != TurnInterrupted {
		t.Fatalf("event = %+v", event)
	}
	if _, ok := recvEvent(t, stream); ok {
		t.Fatal("channel not closed after terminal event")
	}
	final, err := stream.Wait(context.Background())
	if err != nil || final.Status != TurnInterrupted {
		t.Fatalf("Wait = %+v, %v", final, err)
	}
}

func TestTurnFailed(t *testing.T) {
	client, server := connect(t, Options{})
	threadID := startThread(t, client, server, "thr_1")
	stream := startTurn(t, client, server, threadID, "turn_1", Text("hi"))

	server.notify(MethodTurnCompleted, map[string]any{"threadId": "thr_1", "turn": map[string]any{
		"id": "turn_1", "status": "failed",
		"error": map[string]any{
			"message":        "context window exceeded",
			"codexErrorInfo": map[string]any{"type": "ContextWindowExceeded"},
		},
	}})

	event, ok := recvEvent(t, stream)
	if !ok || event.Kind != EventTurnCompleted {
		t.Fatalf("event = %+v", event)
	}
	final, err := stream.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if final.Status != TurnFailed || final.Error == nil {
		t.Fatalf("final = %+v", final)
	}
	if final.Error.Kind() != ErrorInfoContextWindowExceeded {
		t.Fatalf("kind = %q", final.Error.Kind())
	}
}

func TestSteerTurn(t *testing.T) {
	client, server := connect(t, Options{})
	threadID := startThread(t, client, server, "thr_1")

	done := serve(t, func() {
		req := server.expect("turn/steer")
		var params struct {
			ThreadID       string            `json:"threadId"`
			Input          []json.RawMessage `json:"input"`
			ExpectedTurnID string            `json:"expectedTurnId"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			t.Errorf("params: %v", err)
		}
		if params.ThreadID != "thr_1" || params.ExpectedTurnID != "turn_1" {
			t.Errorf("params = %+v", params)
		}
		if !strings.Contains(string(req.Params), "failing tests") {
			t.Errorf("input = %s", req.Params)
		}
		server.respond(req, map[string]any{"turnId": "turn_1"})
	})

	turnID, err := client.SteerTurn(context.Background(), threadID, "turn_1",
		Text("Actually focus on failing tests first."))
	<-done
	if err != nil {
		t.Fatalf("SteerTurn: %v", err)
	}
	if turnID != "turn_1" {
		t.Fatalf("turnId = %q", turnID)
	}
}

func TestCompactAndShellCommand(t *testing.T) {
	client, server := connect(t, Options{})

	done := serve(t, func() {
		req := server.expect("thread/compact/start")
		if string(req.Params) != `{"threadId":"thr_b"}` {
			t.Errorf("params = %s", req.Params)
		}
		server.respond(req, map[string]any{})

		req = server.expect("thread/shellCommand")
		if string(req.Params) != `{"threadId":"thr_b","command":"git status --short"}` {
			t.Errorf("params = %s", req.Params)
		}
		server.respond(req, map[string]any{})
	})

	if err := client.CompactThread(context.Background(), "thr_b"); err != nil {
		t.Fatalf("CompactThread: %v", err)
	}
	if err := client.RunShellCommand(context.Background(), "thr_b", "git status --short"); err != nil {
		t.Fatalf("RunShellCommand: %v", err)
	}
	<-done
}

func TestAbandonedStreamDoesNotBlockOtherThreads(t *testing.T) {
	client, server := connect(t, Options{EventBuffer: 1})

	first := startThread(t, client, server, "thr_1")
	second := startThread(t, client, server, "thr_2")

	abandoned := startTurn(t, client, server, first, "turn_1", Text("slow"))
	live := startTurn(t, client, server, second, "turn_2", Text("fast"))

	// Flood the abandoned stream; its pump blocks, nothing else may.
	for range 10 {
		server.notify(MethodAgentMessageDelta, map[string]any{"threadId": "thr_1", "turnId": "turn_1",
			"itemId": "item_1", "delta": "x"})
	}

	server.notify(MethodTurnCompleted, map[string]any{"threadId": "thr_2",
		"turn": map[string]any{"id": "turn_2", "status": "completed"}})

	final, err := live.Wait(context.Background())
	if err != nil || final.Status != TurnCompleted {
		t.Fatalf("second thread stalled: %+v %v", final, err)
	}

	// Abandoning releases the blocked pump and closes the channel.
	abandoned.Close()
	if _, err := abandoned.Wait(context.Background()); !errors.Is(err, ErrTurnAbandoned) {
		t.Fatalf("Wait = %v, want ErrTurnAbandoned", err)
	}
	deadline := time.After(fakeTimeout)
	for {
		select {
		case _, ok := <-abandoned.Events():
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("abandoned stream channel never closed")
		}
	}
}

func TestEventsForUnknownTurnAreDropped(t *testing.T) {
	client, server := connect(t, Options{})
	threadID := startThread(t, client, server, "thr_1")
	stream := startTurn(t, client, server, threadID, "turn_1", Text("hi"))

	// An event for a thread nobody subscribed to must not wedge the stream.
	server.notify(MethodItemStarted, map[string]any{"threadId": "thr_missing", "turnId": "turn_x",
		"item": map[string]any{"type": "contextCompaction", "id": "item_1"}})
	server.notify(MethodTurnCompleted, map[string]any{"threadId": "thr_1",
		"turn": map[string]any{"id": "turn_1", "status": "completed"}})

	event, ok := recvEvent(t, stream)
	if !ok || event.Kind != EventTurnCompleted {
		t.Fatalf("event = %+v", event)
	}
}

func TestTurnStreamClosedOnClientShutdown(t *testing.T) {
	client, server := connect(t, Options{})
	threadID := startThread(t, client, server, "thr_1")
	stream := startTurn(t, client, server, threadID, "turn_1", Text("hi"))

	server.close()

	if _, err := stream.Wait(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("Wait = %v, want ErrClosed", err)
	}
	deadline := time.After(fakeTimeout)
	for {
		select {
		case _, ok := <-stream.Events():
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("stream channel never closed")
		}
	}
}

func TestTurnWaitContextCancel(t *testing.T) {
	client, server := connect(t, Options{})
	threadID := startThread(t, client, server, "thr_1")
	stream := startTurn(t, client, server, threadID, "turn_1", Text("hi"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := stream.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait = %v, want context.Canceled", err)
	}
}

func TestAbandonedStreamsDoNotLeakGoroutines(t *testing.T) {
	before := runtime.NumGoroutine()

	for range 5 {
		client, server := connect(t, Options{EventBuffer: 1})
		threadID := startThread(t, client, server, "thr_1")
		stream := startTurn(t, client, server, threadID, "turn_1", Text("hi"))
		for range 5 {
			server.notify(MethodAgentMessageDelta, map[string]any{"threadId": "thr_1", "turnId": "turn_1",
				"itemId": "item_1", "delta": "x"})
		}
		stream.Close()
		if err := client.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}

	deadline := time.Now().Add(3 * time.Second)
	for runtime.NumGoroutine() > before+4 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := runtime.NumGoroutine(); got > before+4 {
		t.Fatalf("goroutines leaked: before=%d after=%d", before, got)
	}
}
