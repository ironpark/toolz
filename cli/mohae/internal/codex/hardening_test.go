package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTransportOversizedLineIsFatal(t *testing.T) {
	tr, peer := newTestTransport(t, transportConfig{maxLine: 1024})

	errCh := make(chan error, 1)
	go func() { errCh <- tr.Call(context.Background(), "thread/start", nil, nil) }()
	peer.drain()

	// The write is asynchronous: once the reader gives up on the oversized
	// line the transport closes the pipe, so the peer's write fails.
	oversized := `{"method":"warning","params":{"message":"` + strings.Repeat("x", 4096) + `"}}` + "\n"
	go func() { _, _ = peer.out.Write([]byte(oversized)) }()

	select {
	case <-tr.Done():
	case <-time.After(fakeTimeout):
		t.Fatal("transport survived an oversized message")
	}
	err := tr.Err()
	if !errors.Is(err, bufio.ErrTooLong) {
		t.Fatalf("Err = %v, want bufio.ErrTooLong", err)
	}
	if !strings.Contains(err.Error(), "1024") {
		t.Fatalf("Err = %v, want the limit in the message", err)
	}

	select {
	case callErr := <-errCh:
		if callErr == nil {
			t.Fatal("pending call succeeded")
		}
	case <-time.After(fakeTimeout):
		t.Fatal("pending call not released")
	}
}

func TestMalformedServerOutputIsSkipped(t *testing.T) {
	client, server := connect(t, Options{})
	threadID := startThread(t, client, server, "thr_1")
	stream := startTurn(t, client, server, threadID, "turn_1", Text("hi"))

	server.sendLine(`{"method":`) // truncated JSON
	server.notify(MethodTurnCompleted, map[string]any{"threadId": "thr_1",
		"turn": map[string]any{"id": "turn_1", "status": "completed"}})

	final, err := stream.Wait(context.Background())
	if err != nil || final.Status != TurnCompleted {
		t.Fatalf("malformed line broke the stream: %+v %v", final, err)
	}
}

func TestUndecodableNotificationIsDropped(t *testing.T) {
	client, server := connect(t, Options{})
	threadID := startThread(t, client, server, "thr_1")
	stream := startTurn(t, client, server, threadID, "turn_1", Text("hi"))

	// A well-formed notification whose payload has the wrong shape.
	server.notify(MethodItemStarted, map[string]any{"threadId": "thr_1", "turnId": "turn_1", "item": 42})
	server.notify(MethodTurnCompleted, map[string]any{"threadId": "thr_1",
		"turn": map[string]any{"id": "turn_1", "status": "completed"}})

	event, ok := recvEvent(t, stream)
	if !ok || event.Kind != EventTurnCompleted {
		t.Fatalf("event = %+v", event)
	}
}

func TestServerErrorMapsToRPCError(t *testing.T) {
	client, server := connect(t, Options{})

	done := serve(t, func() {
		req := server.expect("thread/read")
		server.respondError(req, CodeInvalidParams, "unknown thread")
	})
	_, err := client.ReadThread(context.Background(), "thr_missing", false)
	<-done

	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("err = %v, want *RPCError", err)
	}
	if rpcErr.Code != CodeInvalidParams || rpcErr.Message != "unknown thread" {
		t.Fatalf("rpcErr = %+v", rpcErr)
	}
	if IsOverloaded(err) {
		t.Fatal("invalid params reported as overload")
	}
	if !strings.Contains(rpcErr.Error(), "unknown thread") {
		t.Fatalf("Error() = %q", rpcErr.Error())
	}
}

func TestOverloadErrorIsDistinct(t *testing.T) {
	client, server := connect(t, Options{})

	done := serve(t, func() {
		req := server.expect("thread/list")
		server.send(map[string]any{"id": json.RawMessage(req.ID), "error": map[string]any{
			"code": CodeServerOverloaded, "message": "Server overloaded; retry later.",
			"data": map[string]any{"retryAfterMs": 250},
		}})
	})
	_, err := client.ListThreads(context.Background(), ListThreadsParams{})
	<-done

	if !IsOverloaded(err) {
		t.Fatalf("err = %v, want overload", err)
	}
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) || !strings.Contains(rpcErr.Error(), "retryAfterMs") {
		t.Fatalf("err = %v, want data in the message", err)
	}
}

func TestProcessExitFailsActiveTurns(t *testing.T) {
	client, server := connect(t, Options{})
	threadID := startThread(t, client, server, "thr_1")
	first := startTurn(t, client, server, threadID, "turn_1", Text("hi"))

	second := startThread(t, client, server, "thr_2")
	other := startTurn(t, client, server, second, "turn_2", Text("hi"))

	// The subprocess dies mid-turn.
	server.close()

	for _, stream := range []*TurnStream{first, other} {
		if _, err := stream.Wait(context.Background()); !errors.Is(err, ErrClosed) {
			t.Fatalf("Wait = %v, want ErrClosed", err)
		}
	}
	if client.Err() == nil {
		t.Fatal("client Err = nil after process exit")
	}
	if _, err := client.StartTurn(context.Background(), threadID, Text("again"), nil); err == nil {
		t.Fatal("StartTurn succeeded after process exit")
	}
}

func TestUnknownItemTypeSurvivesStreaming(t *testing.T) {
	client, server := connect(t, Options{})
	threadID := startThread(t, client, server, "thr_1")
	stream := startTurn(t, client, server, threadID, "turn_1", Text("hi"))

	server.notify(MethodItemStarted, map[string]any{"threadId": "thr_1", "turnId": "turn_1",
		"item": map[string]any{"type": "someFutureItem", "id": "item_9", "extra": true}})
	server.notify("some/unknown/notification", map[string]any{"threadId": "thr_1"})
	server.notify(MethodTurnCompleted, map[string]any{"threadId": "thr_1",
		"turn": map[string]any{"id": "turn_1", "status": "completed"}})

	event, ok := recvEvent(t, stream)
	if !ok || event.Kind != EventItemStarted {
		t.Fatalf("event = %+v", event)
	}
	unknown, ok := event.Item.Item.(*UnknownItem)
	if !ok || unknown.Type != "someFutureItem" {
		t.Fatalf("item = %#v", event.Item.Item)
	}
	if final, err := stream.Wait(context.Background()); err != nil || final.Status != TurnCompleted {
		t.Fatalf("Wait = %+v, %v", final, err)
	}
}
