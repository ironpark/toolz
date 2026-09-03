package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
)

// maxLineBytes bounds a single JSONL message read from the app-server.
// Aggregated command output can be large, so the limit is generous.
const maxLineBytes = 32 << 20

// wireMessage is the raw JSON-RPC envelope exchanged with the app-server.
// The `jsonrpc` header is omitted on the wire, so the message kind is derived
// from which fields are present:
//
//	method + id -> server-initiated request
//	method      -> notification
//	id          -> response (result or error)
type wireMessage struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *RPCError       `json:"error,omitempty"`
}

// notifyFunc receives server notifications. It runs on the transport reader
// goroutine and must not block.
type notifyFunc func(method string, params json.RawMessage)

// serverRequestFunc handles a server-initiated request. It runs on its own
// goroutine; the returned value is marshaled as the JSON-RPC result, and a
// non-nil error is reported as a JSON-RPC error object.
type serverRequestFunc func(ctx context.Context, method string, params json.RawMessage) (any, error)

// transportConfig configures a transport.
type transportConfig struct {
	// in is the stream of newline-delimited JSON coming from the peer.
	in io.Reader
	// out receives newline-delimited JSON sent to the peer.
	out io.Writer
	// release frees the underlying resources (process, pipes) on Close.
	release func() error
	// onNotification handles server notifications. Optional.
	onNotification notifyFunc
	// onServerRequest handles server-initiated requests. When nil, every
	// server request is answered with a method-not-found error.
	onServerRequest serverRequestFunc
	// maxLine bounds one JSONL message; it defaults to maxLineBytes.
	maxLine int
}

// transport implements JSON-RPC 2.0 framing over a newline-delimited JSON
// byte stream, correlating responses to in-flight calls by id.
type transport struct {
	cfg transportConfig

	writeMu sync.Mutex
	nextID  atomic.Int64

	mu      sync.Mutex
	pending map[int64]chan *wireMessage
	closed  bool
	err     error

	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	handler sync.WaitGroup
	reader  sync.WaitGroup

	closeOnce sync.Once
	fatalOnce sync.Once
}

// newTransport starts the reader goroutine and returns a ready transport.
func newTransport(cfg transportConfig) *transport {
	ctx, cancel := context.WithCancel(context.Background())
	t := &transport{
		cfg:     cfg,
		pending: make(map[int64]chan *wireMessage),
		ctx:     ctx,
		cancel:  cancel,
		done:    make(chan struct{}),
	}
	t.reader.Add(1)
	go t.readLoop()
	return t
}

// Done returns a channel closed when the transport stops for any reason.
func (t *transport) Done() <-chan struct{} { return t.done }

// Err returns the error that terminated the transport, or nil while running.
func (t *transport) Err() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.err
}

// Call sends a request and waits for the matching response. result, when
// non-nil, receives the decoded JSON result.
func (t *transport) Call(ctx context.Context, method string, params any, result any) error {
	id := t.nextID.Add(1)
	ch := make(chan *wireMessage, 1)

	t.mu.Lock()
	if t.closed {
		err := t.err
		t.mu.Unlock()
		if err == nil {
			err = ErrClosed
		}
		return err
	}
	t.pending[id] = ch
	t.mu.Unlock()

	msg := map[string]any{"method": method, "id": id}
	if params != nil {
		msg["params"] = params
	}
	if err := t.write(msg); err != nil {
		t.mu.Lock()
		delete(t.pending, id)
		t.mu.Unlock()
		return err
	}

	select {
	case <-ctx.Done():
		t.mu.Lock()
		delete(t.pending, id)
		t.mu.Unlock()
		return ctx.Err()
	case <-t.done:
		t.mu.Lock()
		delete(t.pending, id)
		err := t.err
		t.mu.Unlock()
		if err == nil {
			err = ErrClosed
		}
		return err
	case resp := <-ch:
		if resp == nil {
			if err := t.Err(); err != nil {
				return err
			}
			return ErrClosed
		}
		if resp.Error != nil {
			return resp.Error
		}
		if result == nil || len(resp.Result) == 0 {
			return nil
		}
		if err := json.Unmarshal(resp.Result, result); err != nil {
			return fmt.Errorf("codex: decode result of %s: %w", method, err)
		}
		return nil
	}
}

// Notify sends a notification, which has no id and no response.
func (t *transport) Notify(method string, params any) error {
	msg := map[string]any{"method": method}
	if params != nil {
		msg["params"] = params
	}
	return t.write(msg)
}

// Close stops the transport, releases the underlying resources, and fails
// every in-flight call. It is safe to call more than once.
func (t *transport) Close() error {
	var releaseErr error
	t.closeOnce.Do(func() {
		t.fatal(ErrClosed)
		if t.cfg.release != nil {
			releaseErr = t.cfg.release()
		}
		// Unblock the reader even when no release hook was supplied.
		if closer, ok := t.cfg.in.(io.Closer); ok {
			_ = closer.Close()
		}
	})
	t.reader.Wait()
	t.handler.Wait()
	return releaseErr
}

// write serializes v as one compact JSON line.
func (t *transport) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("codex: encode message: %w", err)
	}
	b = append(b, '\n')

	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	t.mu.Lock()
	closed, cerr := t.closed, t.err
	t.mu.Unlock()
	if closed {
		if cerr != nil {
			return cerr
		}
		return ErrClosed
	}
	if _, err := t.cfg.out.Write(b); err != nil {
		return fmt.Errorf("codex: write message: %w", err)
	}
	return nil
}

// readLoop consumes newline-delimited JSON until EOF or a fatal error.
func (t *transport) readLoop() {
	defer t.reader.Done()

	limit := t.cfg.maxLine
	if limit <= 0 {
		limit = maxLineBytes
	}
	start := min(64*1024, limit)
	scanner := bufio.NewScanner(t.cfg.in)
	scanner.Buffer(make([]byte, 0, start), limit)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg wireMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			// Malformed input is skipped rather than fatal: a stray line on
			// the stream must not tear down a working connection.
			continue
		}
		t.dispatch(&msg)
	}
	err := scanner.Err()
	if err == nil {
		err = io.EOF
	}
	if errors.Is(err, bufio.ErrTooLong) {
		err = fmt.Errorf("codex: app-server message exceeds %d bytes: %w", limit, bufio.ErrTooLong)
	}
	t.fatal(fmt.Errorf("codex: app-server stream ended: %w", err))
}

// dispatch routes one decoded message to the right handler.
func (t *transport) dispatch(msg *wireMessage) {
	switch {
	case msg.Method != "" && len(msg.ID) > 0:
		t.handleServerRequest(msg)
	case msg.Method != "":
		if t.cfg.onNotification != nil {
			t.cfg.onNotification(msg.Method, msg.Params)
		}
	case len(msg.ID) > 0:
		id, err := strconv.ParseInt(string(msg.ID), 10, 64)
		if err != nil {
			return
		}
		t.mu.Lock()
		ch, ok := t.pending[id]
		delete(t.pending, id)
		t.mu.Unlock()
		if ok {
			ch <- msg
		}
	}
}

// handleServerRequest answers a server-initiated request on its own goroutine
// so that a slow handler (an approval prompt, for instance) never blocks the
// reader.
func (t *transport) handleServerRequest(msg *wireMessage) {
	id := append(json.RawMessage(nil), msg.ID...)
	method := msg.Method
	params := append(json.RawMessage(nil), msg.Params...)

	t.handler.Add(1)
	go func() {
		defer t.handler.Done()
		var (
			result any
			err    error
		)
		if t.cfg.onServerRequest == nil {
			err = &RPCError{Code: CodeMethodNotFound, Message: "method not found: " + method}
		} else {
			result, err = t.cfg.onServerRequest(t.ctx, method, params)
		}
		t.respond(id, result, err)
	}()
}

// respond writes the reply to a server-initiated request.
func (t *transport) respond(id json.RawMessage, result any, err error) {
	reply := map[string]any{"id": id}
	if err != nil {
		var rpcErr *RPCError
		if !errors.As(err, &rpcErr) {
			rpcErr = &RPCError{Code: CodeInternalError, Message: err.Error()}
		}
		reply["error"] = rpcErr
	} else {
		if result == nil {
			result = struct{}{}
		}
		reply["result"] = result
	}
	_ = t.write(reply)
}

// fatal marks the transport dead and releases every waiting caller.
func (t *transport) fatal(err error) {
	t.fatalOnce.Do(func() {
		t.mu.Lock()
		t.closed = true
		if t.err == nil {
			t.err = err
		}
		pending := t.pending
		t.pending = make(map[int64]chan *wireMessage)
		t.mu.Unlock()

		for _, ch := range pending {
			close(ch)
		}
		// Closing the input unblocks a peer that is still writing to us.
		if closer, ok := t.cfg.in.(io.Closer); ok {
			_ = closer.Close()
		}
		t.cancel()
		close(t.done)
	})
}

// processHandles holds the streams and lifecycle of a spawned app-server.
type processHandle struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

// CommandBuilder starts the app-server somewhere other than as a plain child
// of this process. It is handed the executable, its arguments, the working
// directory and the environment, and must return a command that has not been
// started; the caller pipes it and starts it as it would its own.
type CommandBuilder func(ctx context.Context, path string, args []string, dir string, env []string) *exec.Cmd

// startProcess spawns `codex app-server` (or the configured equivalent) with
// piped stdin/stdout.
func startProcess(ctx context.Context, build CommandBuilder, bin string, args []string, env []string, dir string, stderr io.Writer) (*processHandle, error) {
	var cmd *exec.Cmd
	if build != nil {
		// Stripped of cancellation: the caller's context bounds the
		// initialize handshake, while the subprocess is the client's for as
		// long as the client lives and is stopped by closing it. A builder
		// handed the handshake's context would kill the app-server the moment
		// New returned.
		cmd = build(context.WithoutCancel(ctx), bin, args, dir, env)
	} else {
		cmd = exec.Command(bin, args...)
		cmd.Dir = dir
		if env != nil {
			cmd.Env = env
		}
	}
	if stderr != nil {
		cmd.Stderr = stderr
	} else {
		cmd.Stderr = os.Stderr
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("codex: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("codex: stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("codex: start %s: %w", bin, err)
	}
	return &processHandle{cmd: cmd, stdin: stdin, stdout: stdout}, nil
}

// close terminates the subprocess and waits for it to exit.
func (p *processHandle) close() error {
	_ = p.stdin.Close()
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	_ = p.cmd.Wait()
	_ = p.stdout.Close()
	return nil
}
