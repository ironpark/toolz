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

func TestClientHandshakeOrder(t *testing.T) {
	server := newFakeServer(t)

	type dialResult struct {
		client *Client
		err    error
	}
	results := make(chan dialResult, 1)
	go func() {
		client, err := dial(context.Background(), Options{
			ClientInfo: ClientInfo{Name: "mohae", Title: "Mohae", Version: "0.1.0"},
			Capabilities: &ClientCapabilities{
				OptOutNotificationMethods: []string{"item/agentMessage/delta"},
			},
		}, server.toClientR, server.clientW, func() error { server.close(); return nil })
		results <- dialResult{client, err}
	}()

	req := server.expect("initialize")
	if len(req.ID) == 0 {
		t.Fatal("initialize must be a request with an id")
	}
	var params InitializeParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		t.Fatalf("params: %v", err)
	}
	if params.ClientInfo.Name != "mohae" || params.ClientInfo.Version != "0.1.0" {
		t.Fatalf("clientInfo = %+v", params.ClientInfo)
	}
	if params.Capabilities == nil || len(params.Capabilities.OptOutNotificationMethods) != 1 {
		t.Fatalf("capabilities = %+v", params.Capabilities)
	}

	// The initialized notification must not arrive before the response.
	select {
	case msg := <-server.inbox:
		t.Fatalf("client sent %q before the initialize response", msg.Method)
	case <-time.After(50 * time.Millisecond):
	}

	server.respond(req, defaultInitializeResult)
	ack := server.expect("initialized")
	if len(ack.ID) != 0 {
		t.Fatal("initialized must be a notification")
	}

	res := <-results
	if res.err != nil {
		t.Fatalf("dial: %v", res.err)
	}
	defer func() { _ = res.client.Close() }()

	if info := res.client.Info(); info != defaultInitializeResult {
		t.Fatalf("Info() = %+v", info)
	}
}

func TestClientInitializeFails(t *testing.T) {
	server := newFakeServer(t)

	errCh := make(chan error, 1)
	go func() {
		_, err := dial(context.Background(), Options{}, server.toClientR, server.clientW,
			func() error { server.close(); return nil })
		errCh <- err
	}()

	req := server.expect("initialize")
	server.respondError(req, CodeInvalidRequest, "Already initialized")

	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), "Already initialized") {
			t.Fatalf("err = %v", err)
		}
		var rpcErr *RPCError
		if !errors.As(err, &rpcErr) {
			t.Fatalf("err = %v, want wrapped *RPCError", err)
		}
	case <-time.After(fakeTimeout):
		t.Fatal("dial did not return")
	}
}

func TestClientInitializeOnProcessDeath(t *testing.T) {
	server := newFakeServer(t)

	errCh := make(chan error, 1)
	go func() {
		_, err := dial(context.Background(), Options{}, server.toClientR, server.clientW,
			func() error { server.close(); return nil })
		errCh <- err
	}()

	server.expect("initialize")
	server.close() // the app-server dies before responding

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("dial succeeded despite a dead server")
		}
	case <-time.After(fakeTimeout):
		t.Fatal("dial did not return")
	}
}

func TestClientDefaultsClientName(t *testing.T) {
	server := newFakeServer(t)
	go func() {
		_, _ = dial(context.Background(), Options{}, server.toClientR, server.clientW,
			func() error { server.close(); return nil })
	}()

	req := server.expect("initialize")
	var params InitializeParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		t.Fatalf("params: %v", err)
	}
	if params.ClientInfo.Name != "mohae" {
		t.Fatalf("default client name = %q", params.ClientInfo.Name)
	}
	server.respond(req, defaultInitializeResult)
}

func TestClientCallBeforeHandshake(t *testing.T) {
	server := newFakeServer(t)
	c := &Client{}
	c.tr = newTransport(transportConfig{in: server.toClientR, out: server.clientW})
	t.Cleanup(func() { _ = c.tr.Close() })

	if err := c.call(context.Background(), "thread/start", nil, nil); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("err = %v, want ErrNotInitialized", err)
	}
}

func TestClientCallAfterClose(t *testing.T) {
	client, _ := connect(t, Options{})

	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Close is idempotent.
	if err := client.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	err := client.call(context.Background(), "thread/start", nil, nil)
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("err = %v, want ErrClosed", err)
	}

	select {
	case <-client.Done():
	case <-time.After(fakeTimeout):
		t.Fatal("Done not closed")
	}
}

func TestClientProcessExitSurfaces(t *testing.T) {
	client, server := connect(t, Options{})

	server.close()

	select {
	case <-client.Done():
	case <-time.After(fakeTimeout):
		t.Fatal("Done not closed after server exit")
	}
	if client.Err() == nil {
		t.Fatal("Err = nil after server exit")
	}
	if err := client.call(context.Background(), "thread/start", nil, nil); err == nil {
		t.Fatal("call succeeded after server exit")
	}
}

func TestClientOnNotificationHook(t *testing.T) {
	seen := make(chan string, 8)
	_, server := connect(t, Options{
		OnNotification: func(method string, params json.RawMessage) {
			seen <- method + " " + string(params)
		},
	})

	server.notify(MethodThreadStarted, map[string]any{"thread": map[string]any{"id": "thr_1"}})

	select {
	case got := <-seen:
		if got != `thread/started {"thread":{"id":"thr_1"}}` {
			t.Fatalf("notification = %q", got)
		}
	case <-time.After(fakeTimeout):
		t.Fatal("no notification delivered")
	}
}

func TestRouteIDs(t *testing.T) {
	tests := []struct {
		params     string
		wantThread string
		wantTurn   string
	}{
		{`{"threadId":"thr_1","turnId":"turn_1"}`, "thr_1", "turn_1"},
		{`{"turn":{"id":"turn_2","threadId":"thr_2"}}`, "thr_2", "turn_2"},
		{`{"thread":{"id":"thr_3"}}`, "thr_3", ""},
		{`{}`, "", ""},
		{`not json`, "", ""},
		{``, "", ""},
	}
	for _, tc := range tests {
		gotThread, gotTurn := routeIDs(json.RawMessage(tc.params))
		if gotThread != tc.wantThread || gotTurn != tc.wantTurn {
			t.Errorf("routeIDs(%s) = %q,%q want %q,%q", tc.params, gotThread, gotTurn, tc.wantThread, tc.wantTurn)
		}
	}
}

func TestClientCloseStopsGoroutines(t *testing.T) {
	before := runtime.NumGoroutine()

	for range 10 {
		client, _ := connect(t, Options{})
		if err := client.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before+4 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := runtime.NumGoroutine(); got > before+4 {
		t.Fatalf("goroutines leaked: before=%d after=%d", before, got)
	}
}
