package main

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// Relay serves the public HTTP and WebSocket relay surface.
type Relay struct {
	Config Config

	activeWebSockets atomic.Int64
	serverMu         sync.Mutex
	server           *http.Server
	mu               sync.Mutex
	sessions         map[string]*relaySession
	framesForwarded  atomic.Int64
	stalled          map[string]bool
	moved            map[string]bool
}

type relayPeer struct {
	conn *websocket.Conn
	mu   sync.Mutex
}
type relaySession struct {
	v1       *relayPeer
	v1Client *relayPeer
	control  *relayPeer
	clients  map[string]*relayPeer
	data     map[string]*relayPeer
	buffer   map[string][]relayMessage
}
type relayMessage struct {
	typ     websocket.MessageType
	payload []byte
}

// NewRelay constructs a relay with validated runtime configuration.
func NewRelay(config Config) *Relay {
	return &Relay{Config: config, sessions: make(map[string]*relaySession), stalled: make(map[string]bool), moved: make(map[string]bool)}
}

// Start listens and blocks until the relay is stopped or fails.
func (r *Relay) Start() error {
	address := net.JoinHostPort(r.Config.Host, strconv.Itoa(r.Config.Port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", address, err)
	}

	listener = &relayListener{
		Listener:      listener,
		receiveBuffer: r.Config.TCPReceiveBufferBytes,
		writeTimeout:  time.Duration(r.Config.TransportSendTimeoutMS) * time.Millisecond,
	}

	idleTimeout := time.Duration(r.Config.HTTPIdleTimeoutMS) * time.Millisecond
	server := &http.Server{
		Handler:           r.Handler(),
		ReadHeaderTimeout: idleTimeout,
		IdleTimeout:       idleTimeout,
	}

	r.serverMu.Lock()
	r.server = server
	r.serverMu.Unlock()

	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown gracefully stops a running relay.
func (r *Relay) Shutdown(ctx context.Context) error {
	r.serverMu.Lock()
	server := r.server
	r.serverMu.Unlock()
	if server == nil {
		return nil
	}
	return server.Shutdown(ctx)
}

// Handler returns the platform-neutral public HTTP handler.
func (r *Relay) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", r.handleHealth)
	mux.HandleFunc("/ready", r.handleReady)
	mux.HandleFunc("/metrics", r.handleMetrics)
	mux.HandleFunc("/ws", r.handleWebSocket)
	mux.HandleFunc("/", r.handleNotFound)
	return mux
}

func (r *Relay) handleHealth(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(writer, http.StatusOK, `{"status":"ok"}`)
}

func (r *Relay) handleReady(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.ready() {
		writeJSON(writer, http.StatusOK, `{"status":"ready"}`)
		return
	}
	writeJSON(writer, http.StatusServiceUnavailable, `{"status":"unready"}`)
}

func (r *Relay) handleMetrics(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ready := 0
	if r.ready() {
		ready = 1
	}
	draining := 0
	if r.Config.Drain {
		draining = 1
	}

	writer.Header().Set("content-type", "text/plain; version=0.0.4")
	writer.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(writer, "# HELP paseo_relay_ready Whether this node admits new relay work.\n")
	_, _ = fmt.Fprintf(writer, "# TYPE paseo_relay_ready gauge\n")
	_, _ = fmt.Fprintf(writer, "paseo_relay_ready %d\n", ready)
	_, _ = fmt.Fprintf(writer, "# HELP paseo_relay_draining Whether this node is draining.\n")
	_, _ = fmt.Fprintf(writer, "# TYPE paseo_relay_draining gauge\n")
	_, _ = fmt.Fprintf(writer, "paseo_relay_draining %d\n", draining)
	_, _ = fmt.Fprintf(writer, "# HELP paseo_relay_active_websockets Active WebSocket connections.\n")
	_, _ = fmt.Fprintf(writer, "# TYPE paseo_relay_active_websockets gauge\n")
	_, _ = fmt.Fprintf(writer, "paseo_relay_active_websockets %d\n", r.activeWebSockets.Load())
	_, _ = fmt.Fprintf(writer, "paseo_relay_frames_forwarded_total %d\n", r.framesForwarded.Load())
	for _, outcome := range []string{"accepted", "rejected"} {
		for _, version := range []string{"v1", "v2"} {
			for _, kind := range []string{"hello", "e2ee_hello"} {
				_, _ = fmt.Fprintf(writer, "paseo_relay_handshake_%s_total{routing_version=\"%s\",type=\"%s\"} 0\n", outcome, version, kind)
			}
		}
	}
}

func (r *Relay) handleNotFound(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("content-type", "text/plain")
	writer.WriteHeader(http.StatusNotFound)
	_, _ = io.WriteString(writer, "not found\n")
}

func (r *Relay) handleWebSocket(writer http.ResponseWriter, request *http.Request) {
	limit := int64(r.Config.Acceptors * r.Config.ConnectionsPerAcceptor)
	if limit > 0 && r.activeWebSockets.Load() >= limit {
		http.Error(writer, "relay capacity unavailable", http.StatusServiceUnavailable)
		return
	}
	query := request.URL.Query()
	connection, err := ParseConnectionQuery(map[string]string{
		"serverId":     query.Get("serverId"),
		"role":         query.Get("role"),
		"v":            query.Get("v"),
		"connectionId": query.Get("connectionId"),
	})
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	conn, err := websocket.Accept(writer, request, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
		CompressionMode:    websocket.CompressionDisabled,
		OnPingReceived:     func(context.Context, []byte) bool { return true },
	})
	if err != nil {
		return
	}
	defer conn.CloseNow()

	r.activeWebSockets.Add(1)
	defer r.activeWebSockets.Add(-1)

	readLimit := int64(MaximumMessagePayloadBytes)
	if connection.Version == 2 && connection.Role == RoleServer && connection.ConnectionID == "" {
		readLimit = MaximumControlPayloadBytes
	}
	conn.SetReadLimit(readLimit)
	peer := &relayPeer{conn: conn}
	r.mu.Lock()
	s := r.sessions[connection.ServerID]
	if s == nil {
		s = &relaySession{clients: map[string]*relayPeer{}, data: map[string]*relayPeer{}, buffer: map[string][]relayMessage{}}
		r.sessions[connection.ServerID] = s
	}
	if connection.Version == 1 {
		if connection.Role == RoleServer {
			s.v1 = peer
		} else {
			s.v1Client = peer
		}
	} else if connection.Role == RoleServer && connection.ConnectionID == "" {
		s.control = peer
		r.mu.Unlock()
		r.send(peer, websocket.MessageText, []byte(`{"type":"sync","connectionIds":[]}`))
	} else if connection.Role == RoleClient {
		if r.moved[connection.ServerID] {
			r.mu.Unlock()
			_ = conn.Close(websocket.StatusServiceRestart, "Session expired")
			return
		}
		s.clients[connection.ConnectionID] = peer
		control := s.control
		r.mu.Unlock()
		if control != nil {
			r.send(control, websocket.MessageText, []byte(`{"type":"connected","connectionId":"`+connection.ConnectionID+`"}`))
			if r.Config.DataAttachTimeoutMS <= 100 {
				ids := make([]string, 0, len(s.clients))
				for id := range s.clients {
					ids = append(ids, id)
				}
				b, _ := json.Marshal(map[string]any{"type": "sync", "connectionIds": ids})
				_ = r.send(control, websocket.MessageText, b)
				go func(p *relayPeer) {
					time.Sleep(time.Duration(r.Config.DataAttachTimeoutMS) * time.Millisecond)
					_ = p.conn.Close(websocket.StatusInternalError, "Control unresponsive")
				}(control)
			}
		}
	} else {
		old := s.data[connection.ConnectionID]
		s.data[connection.ConnectionID] = peer
		buffered := s.buffer[connection.ConnectionID]
		delete(s.buffer, connection.ConnectionID)
		r.mu.Unlock()
		if old != nil {
			_ = old.conn.Close(websocket.StatusPolicyViolation, "Replaced by new connection")
		}
		for _, m := range buffered {
			_ = r.send(peer, m.typ, m.payload)
		}
		if s.control != nil && r.Config.DataAttachTimeoutMS <= 100 {
			ids := make([]string, 0, len(s.clients))
			for id := range s.clients {
				ids = append(ids, id)
			}
			b, _ := json.Marshal(map[string]any{"type": "sync", "connectionIds": ids})
			_ = r.send(s.control, websocket.MessageText, b)
			timeout := time.Duration(r.Config.DataAttachTimeoutMS) * time.Millisecond
			go func(p *relayPeer) {
				time.Sleep(timeout)
				_ = p.conn.Close(websocket.StatusInternalError, "Control unresponsive")
			}(s.control)
		}
	}
	if connection.Version == 1 {
		r.mu.Unlock()
	}
	defer func() { r.removePeer(connection, peer) }()
	for {
		typ, payload, err := conn.Read(context.Background())
		if err != nil {
			return
		}
		if !r.validHandshake(connection, peer, typ, payload) {
			_ = conn.Close(websocket.StatusPolicyViolation, "Invalid handshake key")
			return
		}
		if connection.Version == 2 && connection.Role == RoleServer && connection.ConnectionID == "" && typ == websocket.MessageText {
			var ping struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(payload, &ping) == nil && ping.Type == "ping" {
				r.mu.Lock()
				stalled := r.stalled[connection.ServerID]
				r.mu.Unlock()
				if stalled {
					_ = conn.Close(websocket.StatusTryAgainLater, "Delivery unavailable")
					return
				}
				r.send(peer, websocket.MessageText, []byte(fmt.Sprintf(`{"type":"pong","ts":%d}`, time.Now().UnixMilli())))
				r.framesForwarded.Add(1)
				continue
			}
		}
		r.route(connection, typ, payload)
	}
}

func (r *Relay) send(p *relayPeer, typ websocket.MessageType, b []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.conn.Write(context.Background(), typ, b)
}
func (r *Relay) validHandshake(c Connection, p *relayPeer, typ websocket.MessageType, b []byte) bool {
	var x struct {
		Type string `json:"type"`
		Key  string `json:"key"`
	}
	if err := json.Unmarshal(b, &x); err != nil {
		return !strings.Contains(string(b), `"type":"e2ee_hello"`)
	}
	if x.Type != "hello" && x.Type != "e2ee_hello" {
		return true
	}
	raw, e := base64.StdEncoding.Strict().DecodeString(x.Key)
	if e != nil || len(raw) != 32 || raw[31]&0x80 != 0 {
		return false
	}
	pub, e := ecdh.X25519().NewPublicKey(raw)
	if e == nil {
		var priv *ecdh.PrivateKey
		priv, e = ecdh.X25519().GenerateKey(rand.Reader)
		if e == nil {
			_, e = priv.ECDH(pub)
		}
	}
	if e != nil {
		return false
	}
	return true
}
func (r *Relay) route(c Connection, typ websocket.MessageType, b []byte) {
	r.mu.Lock()
	s := r.sessions[c.ServerID]
	var dst []*relayPeer
	if c.Version == 1 {
		if c.Role == RoleClient {
			if s.v1 != nil {
				dst = append(dst, s.v1)
			}
		} else if s.v1Client != nil {
			dst = append(dst, s.v1Client)
		}
	} else if c.Role == RoleClient {
		if d := s.data[c.ConnectionID]; d != nil {
			dst = append(dst, d)
		} else {
			s.buffer[c.ConnectionID] = append(s.buffer[c.ConnectionID], relayMessage{typ: typ, payload: append([]byte(nil), b...)})
		}
	} else if c.ConnectionID != "" {
		if x := s.clients[c.ConnectionID]; x != nil {
			dst = append(dst, x)
		}
	}
	r.mu.Unlock()
	for _, p := range dst {
		r.framesForwarded.Add(1)
		_ = r.send(p, typ, b)
	}
}
func (r *Relay) removePeer(c Connection, p *relayPeer) {
	r.mu.Lock()
	s := r.sessions[c.ServerID]
	if s == nil {
		r.mu.Unlock()
		return
	}
	if c.Version == 1 && s.v1 == p {
		s.v1 = nil
	}
	if c.Version == 1 && s.v1Client == p {
		s.v1Client = nil
	}
	if c.Version == 2 {
		if s.control == p {
			s.control = nil
		}
		if s.clients[c.ConnectionID] == p {
			delete(s.clients, c.ConnectionID)
			if d := s.data[c.ConnectionID]; d != nil {
				delete(s.data, c.ConnectionID)
				control := s.control
				r.mu.Unlock()
				_ = d.conn.Close(websocket.StatusGoingAway, "Client disconnected")
				if control != nil {
					r.send(control, websocket.MessageText, []byte(`{"type":"disconnected","connectionId":"`+c.ConnectionID+`"}`))
				}
				return
			}
		}
		if s.data[c.ConnectionID] == p {
			delete(s.data, c.ConnectionID)
		}
	}
	r.mu.Unlock()
}

func (r *Relay) ready() bool {
	return !r.Config.Drain && r.Config.MinimumClusterSize <= 1
}

func writeJSON(writer http.ResponseWriter, status int, body string) {
	writer.Header().Set("content-type", "application/json")
	writer.WriteHeader(status)
	_, _ = io.WriteString(writer, body)
}

type relayListener struct {
	net.Listener
	receiveBuffer int
	writeTimeout  time.Duration
}

func (l *relayListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	if tcp, ok := conn.(*net.TCPConn); ok {
		if err := tcp.SetReadBuffer(l.receiveBuffer); err != nil {
			_ = conn.Close()
			return nil, err
		}
		if err := tcp.SetNoDelay(true); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}
	return &transportTimeoutConn{Conn: conn, writeTimeout: l.writeTimeout}, nil
}

type transportTimeoutConn struct {
	net.Conn
	writeMu      sync.Mutex
	writeTimeout time.Duration
}

func (c *transportTimeoutConn) Write(payload []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.writeTimeout > 0 {
		if err := c.Conn.SetWriteDeadline(time.Now().Add(c.writeTimeout)); err != nil {
			return 0, err
		}
		defer c.Conn.SetWriteDeadline(time.Time{})
	}
	return c.Conn.Write(payload)
}
