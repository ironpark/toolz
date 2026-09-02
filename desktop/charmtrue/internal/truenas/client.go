package truenas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// Notification is a server-initiated JSON-RPC message.
type Notification struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      uint64 `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *uint64         `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type callResult struct {
	response response
	err      error
}

type loginResponse struct {
	ResponseType string `json:"response_type"`
}

// Client is a persistent TrueNAS 25.10 JSON-RPC connection. It matches
// concurrent calls by request ID and restores authentication and subscriptions
// after an unexpected disconnect. Client methods are safe for concurrent use.
type Client struct {
	config Config

	connMu        sync.RWMutex
	conn          *websocket.Conn
	connectionID  uint64
	reconnectMu   sync.Mutex
	autoReconnect atomic.Bool

	nextID  atomic.Uint64
	writeMu sync.Mutex

	pendingMu sync.Mutex
	pending   map[uint64]chan callResult

	subscriptionsMu sync.Mutex
	subscriptions   map[*Subscription]struct{}

	events    chan Notification
	done      chan struct{}
	closeOnce sync.Once
	lifecycle sync.Mutex
	readers   sync.WaitGroup
	workers   sync.WaitGroup
	callSlots chan struct{}
}

// Dial opens a connection and authenticates it with auth.login_ex when
// credentials are configured. Config.OTP completes an OTP_REQUIRED response.
func Dial(ctx context.Context, config Config) (*Client, error) {
	config, err := config.validate()
	if err != nil {
		return nil, err
	}
	c := &Client{
		config:        config,
		pending:       make(map[uint64]chan callResult),
		subscriptions: make(map[*Subscription]struct{}),
		events:        make(chan Notification, config.EventBuffer),
		done:          make(chan struct{}),
		callSlots:     make(chan struct{}, config.MaxConcurrentCalls),
	}
	if err := c.connect(ctx, false); err != nil {
		return nil, err
	}
	c.autoReconnect.Store(!config.DisableReconnect)
	return c, nil
}

// Notifications returns server-initiated messages. A slow consumer does not
// block RPC responses; notifications are dropped when the buffer is full.
func (c *Client) Notifications() <-chan Notification { return c.events }

// Done closes only when Close permanently shuts down the client.
func (c *Client) Done() <-chan struct{} { return c.done }

// Call invokes one JSON-RPC method. Only the retryable -32000 overload response
// is replayed; transport and method errors are returned without replaying calls.
func (c *Client) Call(ctx context.Context, method string, params []any, result any) error {
	method = strings.TrimSpace(method)
	if method == "" {
		return &ValidationError{Field: "method", Message: "is required"}
	}
	if params == nil {
		params = []any{}
	}
	for attempt := 0; ; attempt++ {
		if err := c.acquireCallSlot(ctx); err != nil {
			return err
		}
		err := c.callOnce(ctx, method, params, result)
		c.releaseCallSlot()
		if !IsOverloaded(err) || attempt >= c.config.BusyRetryLimit {
			return err
		}
		delay := c.config.BusyRetryDelay * time.Duration(1<<min(attempt, 6))
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		case <-c.done:
			return ErrClosed
		}
	}
}

// Close permanently stops reconnects, closes the socket, and fails pending calls.
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	var closeErr error
	c.closeOnce.Do(func() {
		c.lifecycle.Lock()
		c.autoReconnect.Store(false)
		close(c.done)
		c.connMu.Lock()
		conn := c.conn
		c.conn = nil
		c.connMu.Unlock()
		c.lifecycle.Unlock()
		if conn != nil {
			closeErr = conn.Close(websocket.StatusNormalClosure, "")
		}
		c.failPending(ErrClosed)
		c.workers.Wait()
		c.readers.Wait()
		close(c.events)
	})
	return closeErr
}

func (c *Client) connect(ctx context.Context, restore bool) error {
	c.reconnectMu.Lock()
	defer c.reconnectMu.Unlock()
	if c.currentConn() != nil {
		return nil
	}
	select {
	case <-c.done:
		return ErrClosed
	default:
	}
	conn, _, err := websocket.Dial(ctx, c.config.Endpoint, &websocket.DialOptions{HTTPClient: c.config.HTTPClient})
	if err != nil {
		return &TransportError{Op: "connect", Err: err}
	}
	conn.SetReadLimit(c.config.ReadLimit)
	c.lifecycle.Lock()
	select {
	case <-c.done:
		c.lifecycle.Unlock()
		conn.CloseNow()
		return ErrClosed
	default:
	}
	c.connMu.Lock()
	c.connectionID++
	connectionID := c.connectionID
	c.conn = conn
	c.connMu.Unlock()
	c.readers.Add(1)
	c.lifecycle.Unlock()
	go c.readLoop(conn, connectionID)

	if err := c.authenticate(ctx); err != nil {
		c.discardConnection(conn, connectionID)
		return err
	}
	if restore {
		if err := c.restoreSubscriptions(ctx); err != nil {
			c.discardConnection(conn, connectionID)
			return fmt.Errorf("truenas: restore subscriptions: %w", err)
		}
	}
	return nil
}

func (c *Client) authenticate(ctx context.Context) error {
	if c.config.APIKey == "" && c.config.Password == "" {
		return nil
	}
	loginData := map[string]any{
		"username":      c.config.Username,
		"login_options": map[string]any{"user_info": false},
	}
	if c.config.APIKey != "" {
		loginData["mechanism"] = "API_KEY_PLAIN"
		loginData["api_key"] = c.config.APIKey
	} else {
		loginData["mechanism"] = "PASSWORD_PLAIN"
		loginData["password"] = c.config.Password
	}
	var login loginResponse
	if err := c.callOnceConnected(ctx, "auth.login_ex", []any{loginData}, &login); err != nil {
		return fmt.Errorf("truenas: authenticate: %w", err)
	}
	if login.ResponseType == "OTP_REQUIRED" {
		if c.config.OTP == "" {
			return ErrOTPRequired
		}
		continueData := map[string]any{
			"mechanism":     "OTP_TOKEN",
			"otp_token":     c.config.OTP,
			"login_options": map[string]any{"user_info": false},
		}
		if err := c.callOnceConnected(ctx, "auth.login_ex_continue", []any{continueData}, &login); err != nil {
			return fmt.Errorf("truenas: continue authentication: %w", err)
		}
	}
	if login.ResponseType == "OTP_REQUIRED" {
		return ErrOTPRequired
	}
	if login.ResponseType != "SUCCESS" {
		return fmt.Errorf("%w (%s)", ErrAuthenticationFailed, login.ResponseType)
	}
	return nil
}

func (c *Client) callOnce(ctx context.Context, method string, params []any, result any) error {
	if c.currentConn() == nil {
		if err := c.connect(ctx, true); err != nil {
			return err
		}
	}
	return c.callOnceConnected(ctx, method, params, result)
}

func (c *Client) callOnceConnected(ctx context.Context, method string, params []any, result any) error {
	conn := c.currentConn()
	if conn == nil {
		return &TransportError{Op: "call", Err: ErrClosed}
	}
	id := c.nextID.Add(1)
	reply := make(chan callResult, 1)
	if err := c.addPending(id, reply); err != nil {
		return err
	}
	payload, err := json.Marshal(request{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	if err != nil {
		c.removePending(id)
		return fmt.Errorf("truenas: encode request: %w", err)
	}
	c.writeMu.Lock()
	err = conn.Write(ctx, websocket.MessageText, payload)
	c.writeMu.Unlock()
	if err != nil {
		c.removePending(id)
		transportErr := &TransportError{Op: "write request", Err: err}
		if c.discardConnection(conn, 0) {
			c.failPending(transportErr)
			c.scheduleReconnect()
		}
		return transportErr
	}
	select {
	case call := <-reply:
		if call.err != nil {
			return call.err
		}
		if call.response.Error != nil {
			return call.response.Error
		}
		if result == nil || len(call.response.Result) == 0 || string(call.response.Result) == "null" {
			return nil
		}
		if err := json.Unmarshal(call.response.Result, result); err != nil {
			return fmt.Errorf("truenas: decode %s result: %w", method, err)
		}
		return nil
	case <-ctx.Done():
		c.removePending(id)
		return ctx.Err()
	case <-c.done:
		c.removePending(id)
		return ErrClosed
	}
}

func (c *Client) readLoop(conn *websocket.Conn, connectionID uint64) {
	defer c.readers.Done()
	for {
		messageType, payload, err := conn.Read(context.Background())
		if err != nil {
			if c.discardConnection(conn, connectionID) {
				c.failPending(&TransportError{Op: "read response", Err: err})
				c.scheduleReconnect()
			}
			return
		}
		if messageType != websocket.MessageText {
			continue
		}
		var message response
		if err := json.Unmarshal(payload, &message); err != nil {
			continue
		}
		if message.ID != nil {
			c.deliver(*message.ID, callResult{response: message})
			continue
		}
		if message.Method != "" {
			select {
			case c.events <- Notification{Method: message.Method, Params: message.Params}:
			default:
			}
		}
	}
}

func (c *Client) scheduleReconnect() {
	c.lifecycle.Lock()
	defer c.lifecycle.Unlock()
	if !c.autoReconnect.Load() {
		return
	}
	c.workers.Add(1)
	go func() {
		defer c.workers.Done()
		for {
			timer := time.NewTimer(c.config.ReconnectDelay)
			select {
			case <-timer.C:
			case <-c.done:
				timer.Stop()
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			err := c.connect(ctx, true)
			cancel()
			if err == nil || errors.Is(err, ErrClosed) {
				return
			}
		}
	}()
}

func (c *Client) currentConn() *websocket.Conn {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.conn
}

func (c *Client) discardConnection(conn *websocket.Conn, connectionID uint64) bool {
	c.connMu.Lock()
	current := c.conn == conn && (connectionID == 0 || c.connectionID == connectionID)
	if current {
		c.conn = nil
	}
	c.connMu.Unlock()
	if current {
		conn.CloseNow()
	}
	return current
}

func (c *Client) acquireCallSlot(ctx context.Context) error {
	select {
	case c.callSlots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return ErrClosed
	}
}

func (c *Client) releaseCallSlot() { <-c.callSlots }

func (c *Client) addPending(id uint64, reply chan callResult) error {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	select {
	case <-c.done:
		return ErrClosed
	default:
		c.pending[id] = reply
		return nil
	}
}

func (c *Client) removePending(id uint64) {
	c.pendingMu.Lock()
	delete(c.pending, id)
	c.pendingMu.Unlock()
}

func (c *Client) deliver(id uint64, result callResult) {
	c.pendingMu.Lock()
	reply := c.pending[id]
	delete(c.pending, id)
	c.pendingMu.Unlock()
	if reply != nil {
		reply <- result
	}
}

func (c *Client) failPending(err error) {
	c.pendingMu.Lock()
	pending := c.pending
	c.pending = make(map[uint64]chan callResult)
	c.pendingMu.Unlock()
	for _, reply := range pending {
		reply <- callResult{err: err}
	}
}
