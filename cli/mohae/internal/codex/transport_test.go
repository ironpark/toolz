package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"runtime"
	"sync"
	"testing"
	"time"
)

// fakePeer is the other end of a transport, driven explicitly by tests.
type fakePeer struct {
	t   *testing.T
	out *io.PipeWriter
	in  *bufio.Scanner
}

// send writes one JSON message to the client.
func (p *fakePeer) send(v any) {
	p.t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		p.t.Fatalf("marshal: %v", err)
	}
	if _, err := p.out.Write(append(b, '\n')); err != nil {
		p.t.Fatalf("write: %v", err)
	}
}

// sendRaw writes a raw line to the client.
func (p *fakePeer) sendRaw(line string) {
	p.t.Helper()
	if _, err := p.out.Write([]byte(line + "\n")); err != nil {
		p.t.Fatalf("write: %v", err)
	}
}

// drain consumes and discards everything the client writes. It never fails
// the test, so it is safe to leave running past the end of one.
func (p *fakePeer) drain() {
	go func() {
		for p.in.Scan() {
		}
	}()
}

// recv reads the next message the client sent.
func (p *fakePeer) recv() *wireMessage {
	p.t.Helper()
	if !p.in.Scan() {
		p.t.Fatalf("peer read: %v", p.in.Err())
	}
	var msg wireMessage
	if err := json.Unmarshal(p.in.Bytes(), &msg); err != nil {
		p.t.Fatalf("peer decode %q: %v", p.in.Text(), err)
	}
	return &msg
}

func newTestTransport(t *testing.T, cfg transportConfig) (*transport, *fakePeer) {
	t.Helper()
	serverRead, serverWrite := io.Pipe() // peer -> client
	clientRead, clientWrite := io.Pipe() // client -> peer

	cfg.in = serverRead
	cfg.out = clientWrite
	cfg.release = func() error {
		_ = serverRead.Close()
		_ = serverWrite.Close()
		_ = clientWrite.Close()
		_ = clientRead.Close()
		return nil
	}
	tr := newTransport(cfg)
	t.Cleanup(func() { _ = tr.Close() })

	scanner := bufio.NewScanner(clientRead)
	scanner.Buffer(make([]byte, 0, 4096), maxLineBytes)
	return tr, &fakePeer{t: t, out: serverWrite, in: scanner}
}

func TestTransportCallResponse(t *testing.T) {
	tr, peer := newTestTransport(t, transportConfig{})

	go func() {
		req := peer.recv()
		if req.Method != "thread/start" {
			t.Errorf("method = %q", req.Method)
		}
		var params struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			t.Errorf("params: %v", err)
		}
		if params.Model != "gpt-5.6-terra" {
			t.Errorf("model = %q", params.Model)
		}
		peer.send(map[string]any{
			"id":     json.RawMessage(req.ID),
			"result": map[string]any{"thread": map[string]any{"id": "thr_123"}},
		})
	}()

	var result struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	err := tr.Call(context.Background(), "thread/start",
		map[string]any{"model": "gpt-5.6-terra"}, &result)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Thread.ID != "thr_123" {
		t.Fatalf("thread id = %q", result.Thread.ID)
	}
}

func TestTransportCallError(t *testing.T) {
	tr, peer := newTestTransport(t, transportConfig{})

	go func() {
		req := peer.recv()
		peer.send(map[string]any{
			"id":    json.RawMessage(req.ID),
			"error": map[string]any{"code": CodeServerOverloaded, "message": "Server overloaded; retry later."},
		})
	}()

	err := tr.Call(context.Background(), "thread/start", nil, nil)
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("err = %v, want *RPCError", err)
	}
	if rpcErr.Code != CodeServerOverloaded {
		t.Fatalf("code = %d", rpcErr.Code)
	}
	if !IsOverloaded(err) {
		t.Fatal("IsOverloaded = false")
	}
}

func TestTransportOutOfOrderResponses(t *testing.T) {
	tr, peer := newTestTransport(t, transportConfig{})

	go func() {
		first := peer.recv()
		second := peer.recv()
		// Reply in reverse order.
		peer.send(map[string]any{"id": json.RawMessage(second.ID), "result": map[string]any{"v": second.Method}})
		peer.send(map[string]any{"id": json.RawMessage(first.ID), "result": map[string]any{"v": first.Method}})
	}()

	type out struct {
		V string `json:"v"`
	}
	var (
		wg           sync.WaitGroup
		aRes, bRes   out
		aErr, bErr   error
		started      = make(chan struct{})
		secondCallGo = make(chan struct{})
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		close(started)
		aErr = tr.Call(context.Background(), "alpha", nil, &aRes)
	}()
	<-started
	go func() {
		defer wg.Done()
		<-secondCallGo
		bErr = tr.Call(context.Background(), "beta", nil, &bRes)
	}()
	// Ensure "alpha" is written before "beta" so ordering is deterministic.
	time.Sleep(20 * time.Millisecond)
	close(secondCallGo)
	wg.Wait()

	if aErr != nil || bErr != nil {
		t.Fatalf("errors: %v %v", aErr, bErr)
	}
	if aRes.V != "alpha" || bRes.V != "beta" {
		t.Fatalf("results crossed: %q %q", aRes.V, bRes.V)
	}
}

func TestTransportNotification(t *testing.T) {
	got := make(chan string, 4)
	_, peer := newTestTransport(t, transportConfig{
		onNotification: func(method string, params json.RawMessage) {
			got <- method + ":" + string(params)
		},
	})

	peer.send(map[string]any{"method": "turn/started", "params": map[string]any{"turn": map[string]any{"id": "turn_456"}}})

	select {
	case v := <-got:
		if v != `turn/started:{"turn":{"id":"turn_456"}}` {
			t.Fatalf("notification = %q", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no notification")
	}
}

func TestTransportSkipsMalformedLines(t *testing.T) {
	got := make(chan string, 4)
	_, peer := newTestTransport(t, transportConfig{
		onNotification: func(method string, params json.RawMessage) { got <- method },
	})

	peer.sendRaw("this is not json")
	peer.sendRaw("")
	peer.send(map[string]any{"method": "turn/completed"})

	select {
	case v := <-got:
		if v != "turn/completed" {
			t.Fatalf("method = %q", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("transport died on malformed input")
	}
}

func TestTransportServerRequestReply(t *testing.T) {
	tr, peer := newTestTransport(t, transportConfig{
		onServerRequest: func(ctx context.Context, method string, params json.RawMessage) (any, error) {
			switch method {
			case "item/commandExecution/requestApproval":
				return map[string]any{"decision": "accept"}, nil
			default:
				return nil, &RPCError{Code: CodeMethodNotFound, Message: "no"}
			}
		},
	})
	_ = tr

	peer.send(map[string]any{"id": 7, "method": "item/commandExecution/requestApproval", "params": map[string]any{"itemId": "item_1"}})
	reply := peer.recv()
	if string(reply.ID) != "7" {
		t.Fatalf("id = %s", reply.ID)
	}
	var res struct {
		Decision string `json:"decision"`
	}
	if err := json.Unmarshal(reply.Result, &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.Decision != "accept" {
		t.Fatalf("decision = %q", res.Decision)
	}

	peer.send(map[string]any{"id": 8, "method": "mystery/request"})
	reply = peer.recv()
	if reply.Error == nil || reply.Error.Code != CodeMethodNotFound {
		t.Fatalf("error = %+v", reply.Error)
	}
}

func TestTransportServerRequestWithoutHandler(t *testing.T) {
	_, peer := newTestTransport(t, transportConfig{})

	peer.send(map[string]any{"id": 3, "method": "item/fileChange/requestApproval"})
	reply := peer.recv()
	if reply.Error == nil || reply.Error.Code != CodeMethodNotFound {
		t.Fatalf("error = %+v", reply.Error)
	}
}

func TestTransportCallContextCanceled(t *testing.T) {
	tr, peer := newTestTransport(t, transportConfig{})

	go func() { peer.recv() }() // drain, never reply

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	err := tr.Call(ctx, "thread/start", nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}

	tr.mu.Lock()
	n := len(tr.pending)
	tr.mu.Unlock()
	if n != 0 {
		t.Fatalf("pending = %d, want 0", n)
	}
}

func TestTransportCloseFailsPendingCalls(t *testing.T) {
	tr, peer := newTestTransport(t, transportConfig{})

	go func() { peer.recv() }()

	errCh := make(chan error, 1)
	go func() { errCh <- tr.Call(context.Background(), "thread/start", nil, nil) }()

	time.Sleep(20 * time.Millisecond)
	if err := tr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("err = %v, want ErrClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pending call not released by Close")
	}

	if err := tr.Call(context.Background(), "thread/start", nil, nil); !errors.Is(err, ErrClosed) {
		t.Fatalf("post-close Call = %v, want ErrClosed", err)
	}
	// Close is idempotent.
	if err := tr.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestTransportStreamEndIsFatal(t *testing.T) {
	tr, peer := newTestTransport(t, transportConfig{})

	_ = peer.out.Close() // EOF from the server side

	select {
	case <-tr.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("transport did not notice EOF")
	}
	if err := tr.Err(); err == nil || !errors.Is(err, io.EOF) {
		t.Fatalf("Err = %v, want wrapped io.EOF", err)
	}
}

func TestTransportCloseStopsGoroutines(t *testing.T) {
	before := runtime.NumGoroutine()

	for range 20 {
		serverRead, serverWrite := io.Pipe()
		clientRead, clientWrite := io.Pipe()
		tr := newTransport(transportConfig{
			in:  serverRead,
			out: clientWrite,
			release: func() error {
				_ = serverRead.Close()
				_ = serverWrite.Close()
				_ = clientWrite.Close()
				_ = clientRead.Close()
				return nil
			},
		})
		if err := tr.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before+2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := runtime.NumGoroutine(); got > before+2 {
		t.Fatalf("goroutines leaked: before=%d after=%d", before, got)
	}
}
