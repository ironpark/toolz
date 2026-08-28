package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"
)

// fakeTimeout bounds every blocking wait in the tests.
const fakeTimeout = 5 * time.Second

// fakeServer is an in-process app-server used by every test in this package.
// It is script-driven: tests expect a request, respond to it, emit
// notifications, and send server-initiated requests.
type fakeServer struct {
	t *testing.T

	toClient   *io.PipeWriter
	toClientR  *io.PipeReader
	fromClient *io.PipeReader
	clientW    *io.PipeWriter

	inbox chan *wireMessage

	mu      sync.Mutex
	replies map[string]chan *wireMessage

	writeMu   sync.Mutex
	closeOnce sync.Once
}

// newFakeServer creates the pipes and starts reading client traffic.
func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	toClientR, toClient := io.Pipe()
	fromClient, clientW := io.Pipe()

	s := &fakeServer{
		t:          t,
		toClient:   toClient,
		toClientR:  toClientR,
		fromClient: fromClient,
		clientW:    clientW,
		inbox:      make(chan *wireMessage, 256),
		replies:    make(map[string]chan *wireMessage),
	}
	go s.readLoop()
	t.Cleanup(s.close)
	return s
}

func (s *fakeServer) readLoop() {
	scanner := bufio.NewScanner(s.fromClient)
	scanner.Buffer(make([]byte, 0, 4096), maxLineBytes)
	for scanner.Scan() {
		var msg wireMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		if msg.Method == "" && len(msg.ID) > 0 {
			// Reply to one of our server-initiated requests.
			s.mu.Lock()
			ch := s.replies[string(msg.ID)]
			s.mu.Unlock()
			if ch != nil {
				ch <- &msg
				continue
			}
		}
		select {
		case s.inbox <- &msg:
		default:
		}
	}
	close(s.inbox)
}

func (s *fakeServer) close() {
	s.closeOnce.Do(func() {
		_ = s.toClient.Close()
		_ = s.toClientR.Close()
		_ = s.clientW.Close()
		_ = s.fromClient.Close()
	})
}

// send writes one JSON message to the client.
func (s *fakeServer) send(v any) {
	s.t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		s.t.Errorf("fake server marshal: %v", err)
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := s.toClient.Write(append(b, '\n')); err != nil {
		s.t.Errorf("fake server write: %v", err)
	}
}

// sendLine writes a raw line to the client, bypassing JSON encoding.
func (s *fakeServer) sendLine(line string) {
	s.t.Helper()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := s.toClient.Write([]byte(line + "\n")); err != nil {
		s.t.Errorf("fake server write: %v", err)
	}
}

// tryNext returns the next client message, reporting false on timeout or a
// closed stream. It is safe to call from a helper goroutine.
func (s *fakeServer) tryNext() (*wireMessage, bool) {
	select {
	case msg, ok := <-s.inbox:
		return msg, ok
	case <-time.After(fakeTimeout):
		return nil, false
	}
}

// next returns the next message the client sent.
func (s *fakeServer) next() *wireMessage {
	s.t.Helper()
	msg, ok := s.tryNext()
	if !ok {
		s.t.Fatal("fake server: timed out waiting for a client message")
	}
	return msg
}

// expect returns the next client message and asserts its method.
func (s *fakeServer) expect(method string) *wireMessage {
	s.t.Helper()
	msg := s.next()
	if msg.Method != method {
		s.t.Fatalf("got method %q, want %q", msg.Method, method)
	}
	return msg
}

// respond answers a client request with a result.
func (s *fakeServer) respond(req *wireMessage, result any) {
	s.t.Helper()
	if result == nil {
		result = struct{}{}
	}
	s.send(map[string]any{"id": json.RawMessage(req.ID), "result": result})
}

// respondError answers a client request with a JSON-RPC error.
func (s *fakeServer) respondError(req *wireMessage, code int, message string) {
	s.t.Helper()
	s.send(map[string]any{
		"id":    json.RawMessage(req.ID),
		"error": map[string]any{"code": code, "message": message},
	})
}

// notify emits a server notification.
func (s *fakeServer) notify(method string, params any) {
	s.t.Helper()
	msg := map[string]any{"method": method}
	if params != nil {
		msg["params"] = params
	}
	s.send(msg)
}

// request sends a server-initiated request and returns a channel carrying the
// client's reply.
func (s *fakeServer) request(id string, method string, params any) <-chan *wireMessage {
	s.t.Helper()
	ch := make(chan *wireMessage, 1)
	s.mu.Lock()
	s.replies[`"`+id+`"`] = ch
	s.mu.Unlock()
	s.send(map[string]any{"id": id, "method": method, "params": params})
	return ch
}

// awaitReply waits for a reply on ch.
func (s *fakeServer) awaitReply(ch <-chan *wireMessage) *wireMessage {
	s.t.Helper()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(fakeTimeout):
		s.t.Fatal("fake server: timed out waiting for a reply")
		return nil
	}
}

// serveHandshake answers initialize and consumes the initialized
// notification. It runs on its own goroutine because dial blocks on it, and
// returns a channel closed once the handshake traffic has been drained.
func (s *fakeServer) serveHandshake(result InitializeResult) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		req, ok := s.tryNext()
		if !ok || req.Method != "initialize" {
			s.t.Errorf("fake server: expected initialize, got %v", req)
			return
		}
		s.respond(req, result)
		if ack, ok := s.tryNext(); !ok || ack.Method != "initialized" {
			s.t.Errorf("fake server: expected initialized, got %v", ack)
		}
	}()
	return done
}

// defaultInitializeResult is the handshake result used by most tests.
var defaultInitializeResult = InitializeResult{
	UserAgent:      "codex_cli_rs/1.0",
	PlatformFamily: "unix",
	PlatformOs:     "macos",
}

// connect returns a handshaken client wired to a fake server.
func connect(t *testing.T, opts Options) (*Client, *fakeServer) {
	t.Helper()
	server := newFakeServer(t)
	handshake := server.serveHandshake(defaultInitializeResult)

	client, err := dial(context.Background(), opts, server.toClientR, server.clientW, func() error {
		server.close()
		return nil
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	// dial returns as soon as it writes the initialized notification, so wait
	// for the handshake goroutine to drain it. Otherwise it races the test for
	// the next message on the inbox.
	select {
	case <-handshake:
	case <-time.After(fakeTimeout):
		t.Fatal("fake server: timed out completing the handshake")
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, server
}
