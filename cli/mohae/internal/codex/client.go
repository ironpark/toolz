package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
)

// DefaultBinary is the executable looked up on PATH when Options.Binary is
// empty.
const DefaultBinary = "codex"

// defaultEventBuffer bounds each turn's event channel.
const defaultEventBuffer = 64

// Options configures a Client.
type Options struct {
	// Binary is the codex executable; it defaults to DefaultBinary on PATH.
	Binary string
	// Command builds the process that runs the app-server. Nil starts it as a
	// child of this process; a builder is handed the same command line and is
	// free to run it elsewhere, which is how mohae runs the agent inside a
	// container.
	Command CommandBuilder
	// Args replaces the default subcommand arguments ("app-server").
	Args []string
	// Env replaces the child process environment; nil inherits the parent's.
	Env []string
	// Dir is the working directory of the subprocess, in whatever namespace
	// Command runs it in.
	Dir string
	// Stderr receives the subprocess stderr; it defaults to os.Stderr.
	Stderr io.Writer

	// ClientInfo identifies the integration during initialize.
	ClientInfo ClientInfo
	// Capabilities are the optional client capabilities sent at initialize.
	Capabilities *ClientCapabilities

	// Approvals answers server-initiated approval requests. When nil, command
	// and file-change approvals are declined, permission requests grant
	// nothing, and any other server request is answered with a
	// method-not-found error, so a turn fails closed instead of hanging.
	Approvals ApprovalHandler

	// OnNotification, when set, receives every notification the server sends,
	// including those routed to thread and turn subscribers.
	OnNotification func(method string, params json.RawMessage)

	// EventBuffer is the per-turn event channel capacity. It defaults to 64.
	EventBuffer int

	// Logger receives debug messages about dropped or unroutable events.
	Logger *slog.Logger
}

// Client controls a Codex app-server subprocess over the JSON-RPC app-server
// protocol. A Client is safe for concurrent use.
type Client struct {
	opts   Options
	logger *slog.Logger
	tr     *transport
	info   InitializeResult

	pending *pendingRequests

	accounts chan AccountUpdate

	mu          sync.Mutex
	initialized bool
	threads     map[string]*threadSubscription
	logins      map[string][]chan *LoginCompletedParams
}

// New spawns `codex app-server`, performs the initialize/initialized
// handshake, and returns a ready client. The context bounds the handshake
// only; the client outlives it.
func New(ctx context.Context, opts Options) (*Client, error) {
	binary := opts.Binary
	if binary == "" {
		binary = DefaultBinary
	}
	args := opts.Args
	if args == nil {
		args = []string{"app-server"}
	}
	proc, err := startProcess(ctx, opts.Command, binary, args, opts.Env, opts.Dir, opts.Stderr)
	if err != nil {
		return nil, err
	}
	return dial(ctx, opts, proc.stdout, proc.stdin, proc.close)
}

// dial wires a client onto an existing byte stream. Tests use it to drive an
// in-process fake app-server.
func dial(ctx context.Context, opts Options, in io.Reader, out io.Writer, release func() error) (*Client, error) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	c := &Client{
		opts:    opts,
		logger:  logger,
		pending: newPendingRequests(),
		threads: make(map[string]*threadSubscription),
		logins:  make(map[string][]chan *LoginCompletedParams),
	}
	c.accounts = make(chan AccountUpdate, c.eventBuffer())
	c.tr = newTransport(transportConfig{
		in:              in,
		out:             out,
		release:         release,
		onNotification:  c.handleNotification,
		onServerRequest: c.handleServerRequest,
	})

	if err := c.handshake(ctx); err != nil {
		_ = c.tr.Close()
		return nil, err
	}
	go func() {
		<-c.tr.Done()
		c.pending.cancelAll()
		c.shutdown()
	}()
	return c, nil
}

// handshake sends initialize, waits for the result, then acknowledges with the
// initialized notification.
func (c *Client) handshake(ctx context.Context) error {
	params := InitializeParams{ClientInfo: c.opts.ClientInfo, Capabilities: c.opts.Capabilities}
	if params.ClientInfo.Name == "" {
		params.ClientInfo.Name = "mohae"
	}
	var result InitializeResult
	if err := c.tr.Call(ctx, "initialize", params, &result); err != nil {
		return fmt.Errorf("codex: initialize: %w", err)
	}
	if err := c.tr.Notify("initialized", struct{}{}); err != nil {
		return fmt.Errorf("codex: initialized: %w", err)
	}

	c.mu.Lock()
	c.info = result
	c.initialized = true
	c.mu.Unlock()
	return nil
}

// Info returns the app-server details reported by initialize.
func (c *Client) Info() InitializeResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.info
}

// Done returns a channel closed when the client stops, either because Close
// was called or because the subprocess exited.
func (c *Client) Done() <-chan struct{} { return c.tr.Done() }

// Err returns the error that stopped the client, or nil while it runs.
func (c *Client) Err() error { return c.tr.Err() }

// Close terminates the subprocess and releases every waiting caller. It is
// safe to call more than once.
func (c *Client) Close() error { return c.tr.Close() }

// call performs a JSON-RPC request, rejecting use before the handshake
// completes or after the client is closed.
func (c *Client) call(ctx context.Context, method string, params, result any) error {
	c.mu.Lock()
	initialized := c.initialized
	c.mu.Unlock()
	if !initialized {
		return ErrNotInitialized
	}
	return c.tr.Call(ctx, method, params, result)
}

// notificationEnvelope carries the routing fields shared by app-server
// notifications. Every field is optional.
type notificationEnvelope struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	Turn     *struct {
		ID       string `json:"id"`
		ThreadID string `json:"threadId"`
	} `json:"turn"`
	Thread *struct {
		ID string `json:"id"`
	} `json:"thread"`
}

// routeIDs extracts the thread and turn ids a notification applies to.
func routeIDs(params json.RawMessage) (threadID, turnID string) {
	if len(params) == 0 {
		return "", ""
	}
	var env notificationEnvelope
	if err := json.Unmarshal(params, &env); err != nil {
		return "", ""
	}
	threadID, turnID = env.ThreadID, env.TurnID
	if env.Turn != nil {
		if turnID == "" {
			turnID = env.Turn.ID
		}
		if threadID == "" {
			threadID = env.Turn.ThreadID
		}
	}
	if threadID == "" && env.Thread != nil {
		threadID = env.Thread.ID
	}
	return threadID, turnID
}

// handleNotification runs on the transport reader goroutine. It must not
// block, so every fan-out path uses buffered channels or dedicated goroutines.
func (c *Client) handleNotification(method string, params json.RawMessage) {
	if c.opts.OnNotification != nil {
		c.opts.OnNotification(method, params)
	}
	switch method {
	case MethodLoginCompleted, MethodAccountUpdated:
		c.routeAccountNotification(method, params)
		return
	}
	// Thread and turn subscribers are registered by the thread and turn APIs
	// and dispatched from here.
	c.dispatchNotification(method, params)
}

// dispatchNotification routes a notification to its subscribers.
func (c *Client) dispatchNotification(method string, params json.RawMessage) {
	threadID, turnID := routeIDs(params)
	if c.routeTurnNotification(method, params, threadID, turnID) {
		return
	}
	c.routeThreadNotification(method, params, threadID)
}
