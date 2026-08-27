package claude

import (
	"context"
	"iter"
	"sync"
)

// DefaultSessionID is the session a client's turns are attributed to when the
// caller does not name one.
const DefaultSessionID = "default"

// Client runs a bidirectional, stateful conversation with the Claude Code CLI:
// turns can be sent at any time, responses consumed as they arrive, and the run
// steered with interrupts and setter calls. Use Query for one-shot prompts.
//
// The lifecycle is explicit rather than scoped:
//
//	client := claude.NewClient(opts)
//	if err := client.Connect(ctx); err != nil {
//		return err
//	}
//	defer client.Disconnect()
//
// A connected client is safe for concurrent use: control calls may run while
// another goroutine ranges over ReceiveMessages. The message stream itself has
// a single consumer — ReceiveMessages and ReceiveResponse read from the same
// underlying sequence, so ranging over both at once splits the messages
// between them.
type Client struct {
	opts *Options

	mu        sync.Mutex
	eng       *engine
	transport Transport
}

// NewClient builds an unconnected client. opts may be nil.
func NewClient(opts *Options) *Client {
	return &Client{opts: opts}
}

// Connect starts the CLI session and performs the initialize handshake. Any
// initial turns are sent right after it. Calling Connect on an already
// connected client is an error.
func (c *Client) Connect(ctx context.Context, initial ...UserInput) error {
	c.mu.Lock()
	if c.eng != nil {
		c.mu.Unlock()
		return NewConnectionError("already connected")
	}
	c.mu.Unlock()

	opts, err := prepareOptions(c.opts, entrypointClient)
	if err != nil {
		return err
	}
	transport := opts.Transport
	if transport == nil {
		transport = newSubprocessTransport(opts)
	}
	if err := transport.Connect(ctx); err != nil {
		return err
	}

	eng := newEngine(transport, opts)
	attachSDKMCPServers(eng, opts)
	eng.Start(ctx)
	if _, err := eng.Initialize(ctx); err != nil {
		_ = eng.Close()
		return err
	}

	c.mu.Lock()
	c.eng, c.transport = eng, transport
	c.mu.Unlock()

	for _, input := range initial {
		if err := c.send(ctx, input, DefaultSessionID); err != nil {
			_ = c.Disconnect()
			return err
		}
	}
	return nil
}

// engineOrErr returns the live engine, or an error when not connected.
func (c *Client) engineOrErr() (*engine, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.eng == nil {
		return nil, NewConnectionError("Not connected. Call Connect first.")
	}
	return c.eng, nil
}

// send writes one user turn.
func (c *Client) send(ctx context.Context, input UserInput, sessionID string) error {
	eng, err := c.engineOrErr()
	if err != nil {
		return err
	}
	if input.SessionID == "" {
		input.SessionID = sessionID
	}
	payload, err := marshalFrame(input.frame())
	if err != nil {
		return err
	}
	return eng.transport.Write(ctx, payload)
}

// Query sends one prompt as a new turn. An empty sessionID uses
// DefaultSessionID.
func (c *Client) Query(ctx context.Context, prompt, sessionID string) error {
	if sessionID == "" {
		sessionID = DefaultSessionID
	}
	return c.send(ctx, UserInput{Content: prompt}, sessionID)
}

// QueryStream sends several turns in order. An empty sessionID uses
// DefaultSessionID; inputs that name their own session keep it.
func (c *Client) QueryStream(ctx context.Context, inputs iter.Seq[UserInput], sessionID string) error {
	if sessionID == "" {
		sessionID = DefaultSessionID
	}
	for input := range inputs {
		if err := c.send(ctx, input, sessionID); err != nil {
			return err
		}
	}
	return nil
}

// ReceiveMessages yields every message the session produces until it ends. A
// fatal error is the final item.
func (c *Client) ReceiveMessages(ctx context.Context) iter.Seq2[Message, error] {
	return func(yield func(Message, error) bool) {
		eng, err := c.engineOrErr()
		if err != nil {
			yield(nil, err)
			return
		}
		for msg, err := range eng.messagesWithContext(ctx) {
			if !yield(msg, err) {
				return
			}
			if err != nil {
				return
			}
		}
	}
}

// ReceiveResponse yields messages up to and including the next ResultMessage,
// then stops. Messages of later turns stay queued for the next call.
func (c *Client) ReceiveResponse(ctx context.Context) iter.Seq2[Message, error] {
	return func(yield func(Message, error) bool) {
		for msg, err := range c.ReceiveMessages(ctx) {
			if !yield(msg, err) {
				return
			}
			if err != nil {
				return
			}
			if _, ok := msg.(*ResultMessage); ok {
				return
			}
		}
	}
}

// Interrupt aborts the current turn.
func (c *Client) Interrupt(ctx context.Context) error {
	eng, err := c.engineOrErr()
	if err != nil {
		return err
	}
	return eng.Interrupt(ctx)
}

// SetPermissionMode changes the permission mode mid-conversation.
func (c *Client) SetPermissionMode(ctx context.Context, mode PermissionMode) error {
	eng, err := c.engineOrErr()
	if err != nil {
		return err
	}
	return eng.SetPermissionMode(ctx, mode)
}

// SetModel changes the model mid-conversation. An empty model restores the CLI
// default.
func (c *Client) SetModel(ctx context.Context, model string) error {
	eng, err := c.engineOrErr()
	if err != nil {
		return err
	}
	return eng.SetModel(ctx, model)
}

// RewindFiles restores tracked files to their state at the given user message.
// It requires Options.EnableFileCheckpointing, and the message UUIDs it takes
// arrive on UserMessage values (enable them with the CLI's
// replay-user-messages flag via Options.ExtraArgs).
func (c *Client) RewindFiles(ctx context.Context, userMessageID string) error {
	eng, err := c.engineOrErr()
	if err != nil {
		return err
	}
	return eng.RewindFiles(ctx, userMessageID)
}

// MCPServerStatus reports the live connection status of every configured MCP
// server, under the "mcpServers" key.
func (c *Client) MCPServerStatus(ctx context.Context) (map[string]any, error) {
	eng, err := c.engineOrErr()
	if err != nil {
		return nil, err
	}
	return eng.MCPStatus(ctx)
}

// ContextUsage reports the context window usage breakdown, the same data the
// CLI's /context command shows.
func (c *Client) ContextUsage(ctx context.Context) (map[string]any, error) {
	eng, err := c.engineOrErr()
	if err != nil {
		return nil, err
	}
	return eng.ContextUsage(ctx)
}

// ReconnectMCPServer retries a disconnected or failed MCP server.
func (c *Client) ReconnectMCPServer(ctx context.Context, serverName string) error {
	eng, err := c.engineOrErr()
	if err != nil {
		return err
	}
	return eng.ReconnectMCPServer(ctx, serverName)
}

// ToggleMCPServer enables or disables an MCP server, connecting or
// disconnecting it and adding or removing its tools.
func (c *Client) ToggleMCPServer(ctx context.Context, serverName string, enabled bool) error {
	eng, err := c.engineOrErr()
	if err != nil {
		return err
	}
	return eng.ToggleMCPServer(ctx, serverName, enabled)
}

// StopTask stops a running background task. A task_notification with status
// "stopped" follows in the message stream.
func (c *Client) StopTask(ctx context.Context, taskID string) error {
	eng, err := c.engineOrErr()
	if err != nil {
		return err
	}
	return eng.StopTask(ctx, taskID)
}

// ServerInfo reports the initialize response: available commands, output
// styles and other capabilities. It is nil before Connect.
func (c *Client) ServerInfo() ServerInfo {
	c.mu.Lock()
	eng := c.eng
	c.mu.Unlock()
	if eng == nil {
		return nil
	}
	return eng.ServerInfo()
}

// Disconnect ends the session and releases its resources. It is idempotent, so
// `defer client.Disconnect()` is safe even on paths that already disconnected.
func (c *Client) Disconnect() error {
	c.mu.Lock()
	eng := c.eng
	c.eng, c.transport = nil, nil
	c.mu.Unlock()
	if eng == nil {
		return nil
	}
	return eng.Close()
}
