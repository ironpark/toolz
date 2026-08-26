package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
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
}

// NewRelay constructs a relay with validated runtime configuration.
func NewRelay(config Config) *Relay {
	return &Relay{Config: config}
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
}

func (r *Relay) handleNotFound(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("content-type", "text/plain")
	writer.WriteHeader(http.StatusNotFound)
	_, _ = io.WriteString(writer, "not found\n")
}

func (r *Relay) handleWebSocket(writer http.ResponseWriter, request *http.Request) {
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

	for {
		_, reader, err := conn.Reader(context.Background())
		if err != nil {
			return
		}
		if _, err := io.Copy(io.Discard, reader); err != nil {
			return
		}
	}
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
