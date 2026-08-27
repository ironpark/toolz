package main

import (
	"bytes"
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
	"runtime"
	"runtime/metrics"
	"slices"
	"sort"
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
	// ownerSessions is the lock-free index used by the owner-call watchdog. A
	// timeout must be able to find and close a session even when its owner call
	// is the goroutine currently waiting for mu.
	ownerSessions             sync.Map
	framesForwarded           atomic.Int64
	bytesForwarded            atomic.Int64
	connectionRejections      atomic.Int64
	rerouteResponses          atomic.Int64
	ingressReserved           atomic.Int64
	ingressInFlight           atomic.Int64
	inflightDelivery          atomic.Int64
	backpressuredSources      atomic.Int64
	slowConsumerDisconnects   atomic.Int64
	deliveryTimeouts          atomic.Int64
	memoryPressureDisconnects atomic.Int64
	maxFrameBytes             atomic.Int64
	frameCount                atomic.Int64
	frameBytes                atomic.Int64
	deliveryWaitCount         atomic.Int64
	deliveryWaitMicroseconds  atomic.Int64
	deliveryWaitBuckets       [len(deliveryWaitBucketDefinitions)]atomic.Int64
	frameSizeBuckets          [len(frameSizeBucketDefinitions)]atomic.Int64
	handshakes                [2][2][2]atomic.Int64
	capacityEpoch             atomic.Int64
	listenerEpoch             atomic.Int64
	capacityUnavailable       atomic.Bool
	memoryPressure            atomic.Bool
	// draining is process-local admission state, seeded from PASEO_RELAY_DRAIN
	// and toggled at runtime through BeginDrain/CancelDrain. As in the
	// reference there is no drain HTTP endpoint.
	draining atomic.Bool
	// seq issues the monotonic ordering keys peers are shed by.
	seq   atomic.Int64
	moved map[string]bool
	// controlSyncDelay and controlCloseDelay are the two control-watchdog
	// stages. They are unexported and set by NewRelay so no environment can
	// reach them; in-package tests shorten them.
	controlSyncDelay  time.Duration
	controlCloseDelay time.Duration
	ownerCallTimeout  time.Duration
	// connectionLimit is the capacity namespace's first observed limit. A
	// later limit change is the Go equivalent of the reference capacity
	// registry's configuration-mismatch response.
	connectionLimit      int64
	connectionLimitValid bool
	// shedBatch and shedHeapBefore carry memory-pressure shedding state between
	// sampler ticks and are only touched from the sampling path.
	shedBatch      int
	shedHeapBefore uint64
	// shedGCAt is when this pressure episode last forced a collection; it is
	// zero between episodes so the first batch of a crossing always collects.
	shedGCAt  time.Time
	ownership ownershipCoordinator
}

type relayPeer struct {
	conn *websocket.Conn
	// writeSlot serializes writers. A buffered channel rather than a mutex so a
	// contended sender parks against its delivery deadline instead of spinning.
	writeSlot chan struct{}
	// shed marks a peer already chosen by memory-pressure shedding, which runs
	// again before the closed socket has left the session. Atomic rather than
	// guarded by Relay.mu so inbound admission stays lock-free; shedding
	// tolerates one extra frame racing through.
	shed atomic.Bool
	// attachSeq and blockSeq are the monotonic ordering keys memory-pressure
	// shedding picks victims by: the longest-blocked source first, then the
	// newest attached socket. blockSeq is zero while the socket has no delivery
	// in flight.
	attachSeq atomic.Int64
	blockSeq  atomic.Int64
	// controlQueued is the byte size of control notifications waiting behind an
	// in-flight write on this socket, bounded by PASEO_RELAY_CONTROL_QUEUE_BYTES.
	controlQueued atomic.Int64
	// heapBytes is the payload memory currently held on this socket's behalf:
	// its in-flight frame, source frames waiting for data attachment, and the
	// control notifications queued to it. It stands in for the reference's
	// per-process max_heap_size fuse.
	heapBytes atomic.Int64
}

func newRelayPeer(conn *websocket.Conn) *relayPeer {
	return &relayPeer{conn: conn, writeSlot: make(chan struct{}, 1)}
}

// chargeHeapOrKill accounts bytes against this socket's heap fuse and kills the
// socket when the charge exceeds it, reporting false in that case. A socket over the fuse is killed outright
// rather than closed, mirroring a BEAM socket process reaching max_heap_size
// with kill: true — the peer sees a transport-level disconnect, not a close
// frame.
func (p *relayPeer) chargeHeapOrKill(bytes, limit int64) bool {
	if reserveCounter(&p.heapBytes, bytes, limit) {
		return true
	}
	go p.conn.CloseNow()
	return false
}

func (p *relayPeer) releaseHeap(bytes int64) { p.heapBytes.Add(-bytes) }

type relaySession struct {
	v1       *relayPeer
	v1Client *relayPeer
	control  *relayPeer
	// clients holds every client socket on a route; a route fans out to all of
	// them and only empties when its last client leaves.
	clients map[string][]*relayPeer
	data    map[string]*relayPeer
	// waiting holds at most one in-flight delivery per source. A source's read
	// loop waits on its waiter instead of allowing frames to accumulate in a
	// relay-owned per-route queue.
	waiting map[string][]*relayDataWaiter
	// watchdogFor is the control peer with a live watchdog stage-one timer.
	watchdogFor *relayPeer
	// ownerDone closes when the session owner has been killed by the owner-call
	// watchdog. peers is published after every topology mutation so the watchdog
	// never needs to take Relay.mu before closing attached sockets.
	ownerDone   chan struct{}
	ownerClosed atomic.Bool
	peers       atomic.Value // stores []*relayPeer
}

type relayDataWaiter struct {
	source *relayPeer
	// ready is buffered so attaching data never waits for a source read loop to
	// be scheduled while holding Relay.mu.
	ready chan relayDataWaitResult
}

type relayDataWaitResult struct {
	destination *relayPeer
	code        websocket.StatusCode
	reason      string
}

type relayDataWaitResultWithWaiter struct {
	waiter *relayDataWaiter
	result relayDataWaitResult
}

// NewRelay constructs a relay with validated runtime configuration. A cluster
// store this node cannot reach is fatal rather than degraded: a relay whose
// ownership coordinator is broken answers every upgrade with 503 forever, so
// failing construction keeps it out of an orchestrator's rotation instead of
// leaving a healthy-looking node that serves nothing.
func NewRelay(config Config) (*Relay, error) {
	ownership, err := newOwnershipCoordinator(config)
	if err != nil {
		return nil, fmt.Errorf("cluster ownership coordinator: %w", err)
	}
	relay := &Relay{
		Config:            config,
		sessions:          make(map[string]*relaySession),
		moved:             make(map[string]bool),
		controlSyncDelay:  controlSyncDelay,
		controlCloseDelay: controlCloseDelay,
		ownerCallTimeout:  ownerCallTimeout,
		ownership:         ownership,
	}
	relay.connectionLimit, relay.connectionLimitValid = connectionCapacityLimit(config)
	relay.draining.Store(config.Drain)
	return relay, nil
}

func newRelaySession() *relaySession {
	session := &relaySession{
		clients:   map[string][]*relayPeer{},
		data:      map[string]*relayPeer{},
		waiting:   map[string][]*relayDataWaiter{},
		ownerDone: make(chan struct{}),
	}
	session.peers.Store([]*relayPeer{})
	return session
}

func (s *relaySession) publishPeersLocked() {
	s.peers.Store(sessionPeers(s))
}

func (s *relaySession) peerSnapshot() []*relayPeer {
	if value := s.peers.Load(); value != nil {
		return append([]*relayPeer(nil), value.([]*relayPeer)...)
	}
	return nil
}

func (s *relaySession) markOwnerClosed() bool {
	if !s.ownerClosed.CompareAndSwap(false, true) {
		return false
	}
	if s.ownerDone != nil {
		close(s.ownerDone)
	}
	return true
}

// BeginDrain closes this node to new sessions. Existing sessions are left to
// the routing layer, which keeps serving them.
func (r *Relay) BeginDrain() { r.draining.Store(true) }

// CancelDrain reopens this node to new sessions.
func (r *Relay) CancelDrain() { r.draining.Store(false) }

// Draining reports the current process-local drain state.
func (r *Relay) Draining() bool { return r.draining.Load() }

// nextSeq issues the next monotonic ordering key.
func (r *Relay) nextSeq() int64 { return r.seq.Add(1) }

// heapFuse is the per-socket memory ceiling in bytes.
func (r *Relay) heapFuse() int64 { return int64(r.Config.WebsocketMaxHeapWords) * 8 }

// Start listens and blocks until the relay is stopped or fails.
func (r *Relay) Start() error {
	address := net.JoinHostPort(r.Config.Host, strconv.Itoa(r.Config.Port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		r.closeCoordinator()
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

	stopWatching := r.watchOwnership()
	stopSampling := r.watchMemoryPressure()
	stopReconciling := r.watchCapacity()
	err = server.Serve(listener)
	stopReconciling()
	stopSampling()
	stopWatching()
	r.closeCoordinator()
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
		r.closeCoordinator()
		return nil
	}
	err := server.Shutdown(ctx)
	r.closeCoordinator()
	return err
}

func (r *Relay) closeCoordinator() {
	_ = r.ownership.close()
}

// Handler returns the platform-neutral public HTTP handler.
func (r *Relay) Handler() http.Handler {
	mux := http.NewServeMux()
	// Operations are dispatched by path, not method. The reference listener
	// sends every non-WebSocket request through Operations.response/1, so POST
	// and other methods have the same result as GET on these paths.
	mux.HandleFunc("/health", r.handleHealth)
	mux.HandleFunc("/ready", r.handleReady)
	mux.HandleFunc("/metrics", r.handleMetrics)
	mux.HandleFunc("/ws", r.handleWebSocket)
	mux.HandleFunc("/", r.handleNotFound)
	return mux
}

func isWebSocketUpgradeRequest(request *http.Request) bool {
	return headerContainsToken(request.Header.Values("Connection"), "Upgrade") &&
		headerContainsToken(request.Header.Values("Upgrade"), "websocket")
}

func headerContainsToken(values []string, want string) bool {
	for _, value := range values {
		for {
			token, rest, more := strings.Cut(value, ",")
			if strings.EqualFold(strings.TrimSpace(token), want) {
				return true
			}
			if !more {
				break
			}
			value = rest
		}
	}
	return false
}

func (r *Relay) handleHealth(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, `{"status":"ok"}`)
}

func (r *Relay) handleReady(writer http.ResponseWriter, _ *http.Request) {
	if r.ready() {
		writeJSON(writer, http.StatusOK, `{"status":"ready"}`)
		return
	}
	writeJSON(writer, http.StatusServiceUnavailable, `{"status":"unready"}`)
}

func (r *Relay) handleMetrics(writer http.ResponseWriter, _ *http.Request) {
	ready := 0
	if r.ready() {
		ready = 1
	}
	draining := 0
	if r.Draining() {
		draining = 1
	}

	writer.Header().Set("content-type", "text/plain; version=0.0.4")
	writer.WriteHeader(http.StatusOK)
	r.mu.Lock()
	activeSessions := len(r.sessions)
	r.mu.Unlock()
	// runtime/metrics rather than runtime.ReadMemStats: scrapes arrive while the
	// relay carries live traffic and ReadMemStats stops the world.
	read := readMemoryMetrics(
		"/memory/classes/heap/objects:bytes",
		"/memory/classes/heap/unused:bytes",
		"/memory/classes/metadata/mcache/inuse:bytes",
	)
	snapshot := metricsSnapshot{
		ready:          int64(ready),
		draining:       int64(draining),
		activeSessions: int64(activeSessions),
		heapAlloc:      int64(read[0]),
		heapInuse:      int64(read[0] + read[1]),
		mcacheInuse:    int64(read[2]),
	}

	capacityUnavailable := r.capacityUnavailable.Load()
	for _, metric := range relayMetricDefinitions {
		if metric.capacity && capacityUnavailable {
			continue
		}
		writeMetric(writer, metric.name, metric.metricType, metric.help, metric.read(r, snapshot))
	}
	r.renderHandshakeMetrics(writer)
	r.renderHistograms(writer)
}

func (r *Relay) handleNotFound(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("content-type", "text/plain")
	writer.WriteHeader(http.StatusNotFound)
	_, _ = io.WriteString(writer, "not found\n")
}

func (r *Relay) handleWebSocket(writer http.ResponseWriter, request *http.Request) {
	connection, conn, release, ok := r.admit(writer, request)
	if !ok {
		return
	}
	defer release()
	peer := r.attach(connection, conn)
	if peer == nil {
		return
	}
	defer func() { r.removePeer(connection, peer) }()
	r.readLoop(connection, peer)
}

// admit runs every check between the request line and a live socket: upgrade
// detection, query parsing, reroute and drain policy, the connection
// reservation, the upgrade itself, and finally the session claim. It answers
// the request itself on every pre-upgrade rejection. Claiming only after
// websocket.Accept succeeds keeps a failed handshake from becoming an
// observable owner to a concurrent request.
func (r *Relay) admit(writer http.ResponseWriter, request *http.Request) (Connection, *websocket.Conn, func(), bool) {
	if !isWebSocketUpgradeRequest(request) {
		writeText(writer, http.StatusUpgradeRequired, "Expected WebSocket upgrade")
		return Connection{}, nil, nil, false
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
		return Connection{}, nil, nil, false
	}
	owner, found, err := r.ownership.lookup(connection.ServerID)
	if err != nil {
		r.connectionRejections.Add(1)
		writeText(writer, http.StatusServiceUnavailable, "owner")
		return Connection{}, nil, nil, false
	}
	if found && !owner.ownedBy(r) {
		r.rerouteResponses.Add(1)
		writer.Header().Set(r.Config.RerouteHeader, owner.target)
		writer.WriteHeader(http.StatusConflict)
		return Connection{}, nil, nil, false
	}
	if reason := r.capacityAdmissionReason(); reason != "" {
		r.connectionRejections.Add(1)
		writeText(writer, http.StatusServiceUnavailable, reason)
		return Connection{}, nil, nil, false
	}
	if r.sessionOwnerClosed(connection.ServerID) {
		r.connectionRejections.Add(1)
		writeText(writer, http.StatusServiceUnavailable, "owner")
		return Connection{}, nil, nil, false
	}
	// A drain refuses to claim a session this node does not already own;
	// sockets joining an existing local session continue below.
	if r.Draining() && !found {
		writeText(writer, http.StatusServiceUnavailable, "draining")
		return Connection{}, nil, nil, false
	}
	if !found {
		members, err := r.ownership.members()
		if err != nil {
			r.connectionRejections.Add(1)
			writeText(writer, http.StatusServiceUnavailable, "owner")
			return Connection{}, nil, nil, false
		}
		if members < r.Config.MinimumClusterSize {
			writeText(writer, http.StatusServiceUnavailable, "cluster")
			return Connection{}, nil, nil, false
		}
	}

	// The reservation is rolled back if the HTTP/WebSocket handshake or the
	// post-upgrade ownership claim fails. There is deliberately no ownership
	// rollback on the failed-upgrade path because no claim has happened yet.
	claimed, reserved := false, false
	defer func() {
		if claimed {
			return
		}
		if reserved {
			r.activeWebSockets.Add(-1)
		}
	}()
	if !r.reserveWebSocket() {
		r.connectionRejections.Add(1)
		reason := r.capacityAdmissionReason()
		if reason == "" {
			reason = "Relay connection capacity"
		}
		writeText(writer, http.StatusServiceUnavailable, reason)
		return Connection{}, nil, nil, false
	}
	reserved = true
	conn, err := websocket.Accept(writer, request, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
		CompressionMode:    websocket.CompressionDisabled,
		OnPingReceived:     func(context.Context, []byte) bool { return true },
	})
	if err != nil {
		r.capacityEpoch.Add(1)
		r.listenerEpoch.Add(1)
		if conn != nil {
			_ = conn.CloseNow()
		}
		return Connection{}, nil, nil, false
	}

	// A second node can win an unowned route while this request is negotiating.
	// The request is already an upgraded WebSocket at this point, so converge by
	// closing it rather than emitting a second HTTP reroute response.
	owner, _, err = r.ownership.claim(connection.ServerID, r)
	if err != nil {
		r.connectionRejections.Add(1)
		closeAsync(conn, websocket.StatusServiceRestart, "Session expired")
		return Connection{}, nil, nil, false
	}
	if !owner.ownedBy(r) {
		closeAsync(conn, websocket.StatusServiceRestart, "Session expired")
		return Connection{}, nil, nil, false
	}
	claimed = true
	conn.SetReadLimit(readLimit(connection))
	return connection, conn, func() {
		r.activeWebSockets.Add(-1)
		conn.CloseNow()
	}, true
}

func (r *Relay) sessionOwnerClosed(serverID string) bool {
	value, ok := r.ownerSessions.Load(serverID)
	if !ok {
		return false
	}
	session, ok := value.(*relaySession)
	return ok && session.ownerClosed.Load()
}

// attach places an upgraded socket into its session topology and performs the
// notification each kind owes on arrival. It returns nil when the socket was
// refused after the upgrade, in which case it is already closed and has no
// session state to tear down.
func (r *Relay) attach(connection Connection, conn *websocket.Conn) *relayPeer {
	peer := newRelayPeer(conn)
	r.mu.Lock()
	peer.attachSeq.Store(r.nextSeq())
	s := r.sessions[connection.ServerID]
	if s == nil && r.moved[connection.ServerID] {
		r.mu.Unlock()
		_ = conn.Close(websocket.StatusServiceRestart, "Session expired")
		return nil
	}
	if s == nil {
		s = newRelaySession()
		r.sessions[connection.ServerID] = s
		r.ownerSessions.Store(connection.ServerID, s)
	} else if s.ownerClosed.Load() {
		r.mu.Unlock()
		_ = conn.Close(websocket.StatusServiceRestart, "Session owner moved")
		return nil
	} else if r.moved[connection.ServerID] {
		r.mu.Unlock()
		_ = conn.Close(websocket.StatusServiceRestart, "Session expired")
		return nil
	}
	switch connection.kind() {
	case peerV1Server, peerV1Client:
		// One socket per role per session: attaching replaces whatever held the
		// role before.
		var replaced *relayPeer
		if connection.Role == RoleServer {
			replaced, s.v1 = s.v1, peer
		} else {
			replaced, s.v1Client = s.v1Client, peer
		}
		s.publishPeersLocked()
		r.mu.Unlock()
		closeReplaced(replaced)
	case peerControl:
		replaced := s.control
		s.control = peer
		ids := clientRouteIDsLocked(s)
		r.armControlWatchdogLocked(s, peer)
		s.publishPeersLocked()
		r.mu.Unlock()
		closeReplaced(replaced)
		r.sendSync(peer, ids)
	case peerV2Client:
		// Clients coexist on one route, so a second client is an addition and
		// never replaces the first.
		s.clients[connection.ConnectionID] = append(s.clients[connection.ConnectionID], peer)
		control := s.control
		r.armControlWatchdogLocked(s, control)
		s.publishPeersLocked()
		r.mu.Unlock()
		if control != nil {
			_ = r.sendControl(control, connectedFrame(connection.ConnectionID))
		}
	case peerV2Data:
		replaced := s.data[connection.ConnectionID]
		s.data[connection.ConnectionID] = peer
		waiting := append([]*relayDataWaiter(nil), s.waiting[connection.ConnectionID]...)
		delete(s.waiting, connection.ConnectionID)
		s.publishPeersLocked()
		r.mu.Unlock()
		closeReplaced(replaced)
		for _, waiter := range waiting {
			waiter.ready <- relayDataWaitResult{destination: peer}
		}
	}
	return peer
}

// readLoop serves one attached socket until it closes or is closed. Every exit
// is a return: the caller's deferred teardown retires the peer.
func (r *Relay) readLoop(connection Connection, peer *relayPeer) {
	conn := peer.conn
	for {
		typ, payload, err := conn.Read(context.Background())
		if err != nil {
			return
		}
		if len(payload) > payloadLimit(connection) {
			_ = conn.Close(websocket.StatusMessageTooBig, "Message too big")
			return
		}
		// The v2 control socket accepts only text application messages. Binary
		// frames are discarded by the reference before frame observation or
		// capacity admission, and do not affect the connection's state.
		if connection.isControl() && typ != websocket.MessageText {
			continue
		}
		r.observeFrame(len(payload))
		// Message admission runs before the handshake check, as in the
		// reference: a node under memory pressure, or a socket already picked
		// by shedding, stops accepting inbound work entirely.
		if !r.admitsMessage(connection, typ, peer) {
			_ = conn.Close(websocket.StatusTryAgainLater, "Relay ingress capacity")
			return
		}
		if connection.Role == RoleClient && !r.validateHandshake(connection.Version, payload) {
			_ = conn.Close(websocket.StatusPolicyViolation, "Invalid handshake key")
			return
		}
		if connection.isControl() && typ == websocket.MessageText && controlPing(payload) {
			pong := pongFrame(time.Now())
			if err := r.sendPong(peer, pong); err != nil {
				_ = conn.Close(websocket.StatusTryAgainLater, "Delivery unavailable")
				return
			}
			continue
		}
		r.route(connection, peer, typ, payload)
	}
}

// clientRouteIDsLocked lists the route IDs that currently have a client
// attached; empty routes are deleted on detach, so every key qualifies.
// Callers hold r.mu.
func clientRouteIDsLocked(s *relaySession) []string {
	ids := make([]string, 0, len(s.clients))
	for id := range s.clients {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// sendSync publishes a client roster to the control socket.
func (r *Relay) sendSync(control *relayPeer, ids []string) {
	_ = r.sendControl(control, syncFrame(ids))
}

func waitingForDataLocked(s *relaySession) bool {
	for connectionID := range s.clients {
		if s.data[connectionID] == nil {
			return true
		}
	}
	return false
}

// controlStalledLocked is the liveness predicate both watchdog stages re-check
// when they fire: control still fronts the session and a client route is still
// waiting for its data socket.
func controlStalledLocked(s *relaySession, control *relayPeer) bool {
	return s.control == control && waitingForDataLocked(s)
}

// armControlWatchdogLocked starts the two-stage control-liveness deadline that
// runs while a client route waits for its data socket: a sync re-send, then a
// close. Both timers are fire-and-forget and re-check live state when they
// fire, so an attach or a control replacement in the meantime turns them into
// no-ops and no cancellation bookkeeping is needed. watchdogFor keeps a burst
// of attaches from stacking one timer chain per attach.
func (r *Relay) armControlWatchdogLocked(s *relaySession, control *relayPeer) {
	if control == nil || s.watchdogFor == control || !waitingForDataLocked(s) {
		return
	}
	s.watchdogFor = control
	time.AfterFunc(r.controlSyncDelay, func() {
		r.mu.Lock()
		if s.watchdogFor == control {
			s.watchdogFor = nil
		}
		waiting := controlStalledLocked(s, control)
		var ids []string
		if waiting {
			ids = clientRouteIDsLocked(s)
		}
		r.mu.Unlock()
		if !waiting {
			return
		}
		r.sendSync(control, ids)
		time.AfterFunc(r.controlCloseDelay, func() {
			r.mu.Lock()
			unresponsive := controlStalledLocked(s, control)
			r.mu.Unlock()
			if unresponsive {
				_ = control.conn.Close(websocket.StatusInternalError, "Control unresponsive")
			}
		})
	})
}

// deliveryContext bounds one write by PASEO_RELAY_DELIVERY_TIMEOUT_MS.
func (r *Relay) deliveryContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), time.Duration(r.Config.DeliveryTimeoutMS)*time.Millisecond)
}

// dropSlowConsumer retires a destination that could not keep up. timedOut
// distinguishes a missed write deadline, which also counts as a delivery
// timeout, from a queue bound reached before any write was attempted.
func (r *Relay) dropSlowConsumer(p *relayPeer, timedOut bool) {
	if timedOut {
		r.deliveryTimeouts.Add(1)
	}
	r.slowConsumerDisconnects.Add(1)
	closeAsync(p.conn, websocket.StatusTryAgainLater, "Slow consumer")
}

func (r *Relay) send(p *relayPeer, typ websocket.MessageType, b []byte) error {
	return r.sendUntil(p, typ, b, time.Now().Add(time.Duration(r.Config.DeliveryTimeoutMS)*time.Millisecond))
}

// sendUntil performs one destination write against an absolute delivery
// deadline. V2 data attachment and the eventual write share this deadline, as
// they do in the reference implementation.
func (r *Relay) sendUntil(p *relayPeer, typ websocket.MessageType, b []byte, deadline time.Time) error {
	if !deadline.After(time.Now()) {
		return context.DeadlineExceeded
	}
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	select {
	case p.writeSlot <- struct{}{}:
	case <-ctx.Done():
		return context.DeadlineExceeded
	}
	defer func() { <-p.writeSlot }()
	if !p.chargeHeapOrKill(int64(len(b)), r.heapFuse()) {
		return errHeapFuse
	}
	defer p.releaseHeap(int64(len(b)))
	return p.conn.Write(ctx, typ, b)
}

var (
	// errControlQueue reports a destination whose queued control notifications
	// passed PASEO_RELAY_CONTROL_QUEUE_BYTES.
	errControlQueue = errors.New("control queue exhausted")
	// errHeapFuse reports a socket killed by its per-socket memory ceiling.
	errHeapFuse = errors.New("socket heap fuse")
)

// sendControl delivers a control notification. Control frames queue behind an
// in-flight payload write rather than being dropped, but the queue is bounded
// by PASEO_RELAY_CONTROL_QUEUE_BYTES: a destination that cannot keep up is a
// slow consumer and is closed instead of allowed to accumulate.
func (r *Relay) sendControl(p *relayPeer, b []byte) error {
	return r.sendControlWithFailure(p, b, true)
}

// sendPong uses the same bounded control queue as roster notifications, but
// lets the control reader report its own compatibility close reason instead of
// racing a Slow consumer close from the queue implementation.
func (r *Relay) sendPong(p *relayPeer, b []byte) error {
	if err := r.sendControlWithFailure(p, b, false); err != nil {
		return err
	}
	r.framesForwarded.Add(1)
	r.bytesForwarded.Add(int64(len(b)))
	return nil
}

func (r *Relay) sendControlWithFailure(p *relayPeer, b []byte, closeSlowConsumer bool) error {
	if p == nil {
		return nil
	}
	// An idle writer starts the notification immediately, which is the only
	// path the queue bound does not apply to.
	select {
	case p.writeSlot <- struct{}{}:
		defer func() { <-p.writeSlot }()
		err := r.writeControl(p, b)
		if err != nil && closeSlowConsumer {
			r.dropSlowConsumer(p, true)
		}
		return err
	default:
	}
	queued := int64(len(b))
	if !reserveCounter(&p.controlQueued, queued, int64(r.Config.ControlQueueBytes)) {
		if closeSlowConsumer {
			r.dropSlowConsumer(p, false)
		}
		return errControlQueue
	}
	defer p.controlQueued.Add(-queued)
	if !p.chargeHeapOrKill(queued, r.heapFuse()) {
		return errHeapFuse
	}
	defer p.releaseHeap(queued)

	ctx, cancel := r.deliveryContext()
	defer cancel()
	select {
	case p.writeSlot <- struct{}{}:
	case <-ctx.Done():
		if closeSlowConsumer {
			r.dropSlowConsumer(p, true)
		}
		return context.DeadlineExceeded
	}
	defer func() { <-p.writeSlot }()
	err := r.writeControl(p, b)
	if err != nil && closeSlowConsumer {
		r.dropSlowConsumer(p, true)
	}
	return err
}

// writeControl performs the write itself; callers hold the destination's write
// slot. A write that misses its deadline is returned to the caller, which
// decides whether it is a slow-consumer close or a control-pong failure.
func (r *Relay) writeControl(p *relayPeer, b []byte) error {
	ctx, cancel := r.deliveryContext()
	defer cancel()
	return p.conn.Write(ctx, websocket.MessageText, b)
}

func (r *Relay) forward(p *relayPeer, typ websocket.MessageType, b []byte) error {
	return r.forwardUntil(p, typ, b, time.Now().Add(time.Duration(r.Config.DeliveryTimeoutMS)*time.Millisecond))
}

func (r *Relay) forwardUntil(p *relayPeer, typ websocket.MessageType, b []byte, deadline time.Time) error {
	err := r.sendUntil(p, typ, b, deadline)
	if err == nil {
		r.framesForwarded.Add(1)
		r.bytesForwarded.Add(int64(len(b)))
		return nil
	}
	r.capacityEpoch.Add(1)
	r.dropSlowConsumer(p, true)
	return err
}

func (r *Relay) observeDeliveryWait(duration time.Duration) {
	microseconds := duration.Microseconds()
	r.deliveryWaitCount.Add(1)
	r.deliveryWaitMicroseconds.Add(microseconds)
	observeHistogramBucket(r.deliveryWaitBuckets[:], deliveryWaitBucketDefinitions[:], microseconds)
}

// observeHistogramBucket increments only the first fitting bucket; render-time
// running sums restore the cumulative le counts Prometheus expects. This keeps
// the per-frame cost at one atomic add instead of one per bucket.
func observeHistogramBucket(buckets []atomic.Int64, definitions []histogramBucketDefinition, value int64) {
	for i := range definitions {
		if value <= definitions[i].limit {
			buckets[i].Add(1)
			return
		}
	}
}

// closeReplaced retires the socket a new attach displaced. Every role that
// holds exactly one socket per session — both v1 peers, the v2 control socket,
// and a v2 data socket — is replaced this way.
func closeReplaced(replaced *relayPeer) {
	if replaced != nil {
		closeAsync(replaced.conn, websocket.StatusPolicyViolation, "Replaced by new connection")
	}
}

// closeAsync closes conn off the calling path: Close waits for the peer's
// close frame, and neither delivery nor shedding may stall on one socket.
func closeAsync(conn *websocket.Conn, code websocket.StatusCode, reason string) {
	go func() { _ = conn.Close(code, reason) }()
}

// timeoutSessionOwner is the kill half of the owner-call watchdog. It closes
// the published peer snapshot before taking Relay.mu, because the stalled
// owner call may itself be waiting for that mutex.
func (r *Relay) timeoutSessionOwner(serverID string, expected *relaySession) {
	var session *relaySession
	if expected != nil {
		session = expected
		if value, ok := r.ownerSessions.Load(serverID); ok && value != session {
			return
		}
	} else {
		value, ok := r.ownerSessions.Load(serverID)
		if !ok {
			return
		}
		session, _ = value.(*relaySession)
	}
	if session == nil || !session.markOwnerClosed() {
		return
	}
	for _, peer := range session.peerSnapshot() {
		closeAsync(peer.conn, websocket.StatusServiceRestart, "Session owner moved")
	}
	go r.finishTimedOutSession(serverID, session)
}

// finishTimedOutSession drains waiters and releases the ownership record after
// the immediate socket kill. Keeping the moved marker until release prevents a
// new upgrade from claiming the old owner while cleanup is still in flight.
func (r *Relay) finishTimedOutSession(serverID string, session *relaySession) {
	r.mu.Lock()
	if r.sessions[serverID] != session {
		r.mu.Unlock()
		return
	}
	r.moved[serverID] = true
	var waiting []*relayDataWaiter
	for connectionID, routeWaiters := range session.waiting {
		waiting = append(waiting, routeWaiters...)
		delete(session.waiting, connectionID)
	}
	peers := session.peerSnapshot()
	r.mu.Unlock()

	notifyDataWaiters(waiting, relayDataWaitResult{
		code:   websocket.StatusServiceRestart,
		reason: "Session owner moved",
	})
	for _, peer := range peers {
		closeAsync(peer.conn, websocket.StatusServiceRestart, "Session owner moved")
	}
	_ = r.ownership.release(serverID, r)

	r.mu.Lock()
	if r.sessions[serverID] == session {
		delete(r.sessions, serverID)
		if value, ok := r.ownerSessions.Load(serverID); ok && value == session {
			r.ownerSessions.Delete(serverID)
		}
		delete(r.moved, serverID)
	}
	r.mu.Unlock()
}

// handshakeKind classifies a decoded frame, reporting false when the frame is
// not a handshake at all.
func handshakeKind(frame handshakeFrame) (int, bool) {
	switch frame.Type {
	case "hello":
		return 0, true
	case "e2ee_hello":
		return 1, true
	default:
		return 0, false
	}
}

// probeKey is only used to reject low-order points, so its value is irrelevant
// and one key per process gives the same answer as one per handshake.
var probeKey = sync.OnceValues(func() (*ecdh.PrivateKey, error) {
	return ecdh.X25519().GenerateKey(rand.Reader)
})

// acceptableKey reports whether a hello carries a usable X25519 public key.
// A key that is missing or not a JSON string is invalid, as in the reference.
func acceptableKey(raw json.RawMessage) bool {
	key := ""
	if raw != nil && json.Unmarshal(raw, &key) != nil {
		return false
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(key)
	if err != nil || len(decoded) != 32 || !canonicalCoordinate(decoded) {
		return false
	}
	pub, err := ecdh.X25519().NewPublicKey(decoded)
	if err != nil {
		return false
	}
	priv, err := probeKey()
	if err != nil {
		return false
	}
	// Rejects low-order points, which yield an all-zero shared secret.
	_, err = priv.ECDH(pub)
	return err == nil
}

// fieldOrder is 2^255 - 19 little-endian, the exclusive upper bound of a
// canonical X25519 coordinate.
var fieldOrder = [32]byte{
	0xed, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f,
}

// canonicalCoordinate reports whether 32 little-endian bytes are below the
// field order. X25519 silently reduces larger encodings, so accepting them
// would admit several spellings of the same key.
func canonicalCoordinate(decoded []byte) bool {
	for i := 31; i >= 0; i-- {
		if decoded[i] != fieldOrder[i] {
			return decoded[i] < fieldOrder[i]
		}
	}
	return false
}

// validateHandshake decodes the frame once and records the outcome. Every
// client frame reaches it, so frames that cannot decode to a JSON object —
// anything not starting with '{' — are passed through without paying for a
// full parse; such frames could never classify as a handshake anyway.
func (r *Relay) validateHandshake(version int, b []byte) bool {
	trimmed := bytes.TrimLeft(b, " \t\r\n")
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return true
	}
	var frame handshakeFrame
	if json.Unmarshal(b, &frame) != nil {
		return true
	}
	kind, handshake := handshakeKind(frame)
	if !handshake {
		return true
	}
	outcome := 0
	accepted := acceptableKey(frame.Key)
	if !accepted {
		outcome = 1
	}
	r.handshakes[outcome][version-1][kind].Add(1)
	return accepted
}

// admitsMessage is the inbound admission gate. Memory pressure and a socket
// already picked by shedding both close the door; the reference discards a
// control socket's binary frames before admission, so those are never charged.
func (r *Relay) admitsMessage(c Connection, typ websocket.MessageType, source *relayPeer) bool {
	if c.isControl() && typ != websocket.MessageText {
		return true
	}
	if r.memoryPressure.Load() {
		return false
	}
	return !source.shed.Load()
}

func (r *Relay) route(c Connection, source *relayPeer, typ websocket.MessageType, b []byte) {
	weighted := int64(len(b) * r.Config.IngressWeight)
	if !r.reserveIngress(weighted) {
		closeAsync(source.conn, websocket.StatusTryAgainLater, "Relay ingress capacity")
		return
	}
	// Pressure that arrives between admission and delivery stops the message
	// here instead, which the reference reports with its own close reason.
	if !r.admitsMessage(c, typ, source) {
		r.releaseInFlight(weighted)
		closeAsync(source.conn, websocket.StatusTryAgainLater, "Relay memory pressure")
		return
	}
	if !source.chargeHeapOrKill(int64(len(b)), r.heapFuse()) {
		r.releaseInFlight(weighted)
		return
	}
	defer r.releaseInFlight(weighted)
	defer source.releaseHeap(int64(len(b)))

	// Capacity marks a source blocked once when start_delivery admits its
	// message, before owner/data lookup and fan-out begin. The reference keeps
	// that one blocked entry until the whole message finishes, so the gauge and
	// in-flight payload accounting belong to the route rather than each write.
	deliveryStarted := time.Now()
	r.inflightDelivery.Add(int64(len(b)))
	r.backpressuredSources.Add(1)
	defer func() {
		r.backpressuredSources.Add(-1)
		r.inflightDelivery.Add(-int64(len(b)))
		r.observeDeliveryWait(time.Since(deliveryStarted))
	}()
	// A source with a delivery in flight is blocked, and shedding drops the
	// longest-blocked source first.
	source.blockSeq.Store(r.nextSeq())
	defer source.blockSeq.Store(0)
	deadline := time.Now().Add(time.Duration(r.Config.DeliveryTimeoutMS) * time.Millisecond)
	ownerResult := r.ownerDestinations(c, source, deadline)
	if ownerResult.code != 0 {
		closeAsync(source.conn, ownerResult.code, ownerResult.reason)
		return
	}
	destinations := ownerResult.destinations

	// One surviving destination is enough: forward closes a slow destination
	// itself, and only an entirely failed fan-out reaches back to the source.
	delivered := false
	if len(destinations) == 1 {
		delivered = r.forwardUntil(destinations[0], typ, b, deadline) == nil
	} else if len(destinations) > 1 {
		// Deliver concurrently so one blocked destination costs the fan-out a
		// single delivery timeout rather than one per destination.
		var wg sync.WaitGroup
		var successes atomic.Int64
		for _, destination := range destinations {
			wg.Add(1)
			go func(destination *relayPeer) {
				defer wg.Done()
				if r.forwardUntil(destination, typ, b, deadline) == nil {
					successes.Add(1)
				}
			}(destination)
		}
		wg.Wait()
		delivered = successes.Load() > 0
	}
	if len(destinations) > 0 && !delivered {
		closeAsync(source.conn, websocket.StatusTryAgainLater, "Delivery unavailable")
	}
}

// dataDestinationOrWait returns the data peer immediately when attached, or
// registers one waiter for this source and blocks until that peer attaches. The
// timeout is the earlier of the remaining delivery deadline and the configured
// data-attach timeout, so waiting cannot grant the subsequent write a fresh
// delivery budget.
type ownerDestinationsResult struct {
	destinations []*relayPeer
	code         websocket.StatusCode
	reason       string
}

func ownerMovedResult() ownerDestinationsResult {
	return ownerDestinationsResult{
		code:   websocket.StatusServiceRestart,
		reason: "Session owner moved",
	}
}

// ownerDestinations bounds the session-owner operation independently of the
// delivery deadline. The reference kills an owner whose call does not return
// within five seconds; the timeout path therefore closes the whole session,
// not only the source that happened to make the call.
func (r *Relay) ownerDestinations(c Connection, source *relayPeer, deadline time.Time) ownerDestinationsResult {
	var expected *relaySession
	if value, ok := r.ownerSessions.Load(c.ServerID); ok {
		expected, _ = value.(*relaySession)
	}
	result := make(chan ownerDestinationsResult, 1)
	go func() {
		result <- r.ownerDestinationsCall(c, source, deadline)
	}()
	timeout := r.ownerCallTimeout
	if timeout <= 0 {
		timeout = ownerCallTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case value := <-result:
		return value
	case <-timer.C:
		r.timeoutSessionOwner(c.ServerID, expected)
		return ownerMovedResult()
	}
}

func (r *Relay) ownerDestinationsCall(c Connection, source *relayPeer, deadline time.Time) ownerDestinationsResult {
	r.mu.Lock()
	s := r.sessions[c.ServerID]
	if s == nil || s.ownerClosed.Load() || r.moved[c.ServerID] {
		r.mu.Unlock()
		return ownerMovedResult()
	}
	var destinations []*relayPeer
	switch c.kind() {
	case peerV1Server, peerV1Client:
		peer := s.v1
		if c.Role != RoleClient {
			peer = s.v1Client
		}
		if peer != nil {
			destinations = []*relayPeer{peer}
		}
	case peerV2Client:
		if s.data[c.ConnectionID] != nil {
			destinations = []*relayPeer{s.data[c.ConnectionID]}
		}
	case peerV2Data:
		// Route slices are appended to or replaced wholesale, never mutated in
		// place, so the map value can be shared with the fan-out without copying.
		destinations = s.clients[c.ConnectionID]
	case peerControl:
		// A control frame that is not a ping has nowhere to go.
	}
	r.mu.Unlock()

	if c.kind() == peerV2Client && len(destinations) == 0 {
		destination, code, reason := r.dataDestinationOrWait(s, c.ConnectionID, source, deadline)
		if code != 0 {
			return ownerDestinationsResult{code: code, reason: reason}
		}
		destinations = []*relayPeer{destination}
	}
	return ownerDestinationsResult{destinations: destinations}
}

func (r *Relay) dataDestinationOrWait(s *relaySession, connectionID string, source *relayPeer, deadline time.Time) (*relayPeer, websocket.StatusCode, string) {
	now := time.Now()
	attachDeadline := now.Add(time.Duration(r.Config.DataAttachTimeoutMS) * time.Millisecond)
	timeoutAt := deadline
	timeoutCode, timeoutReason := websocket.StatusTryAgainLater, "Delivery unavailable"
	if attachDeadline.Before(timeoutAt) {
		timeoutAt = attachDeadline
		timeoutReason = "Data route unavailable"
	}
	if !timeoutAt.After(now) {
		return nil, timeoutCode, timeoutReason
	}

	waiter := &relayDataWaiter{source: source, ready: make(chan relayDataWaitResult, 1)}
	r.mu.Lock()
	if s.ownerClosed.Load() {
		r.mu.Unlock()
		return nil, websocket.StatusServiceRestart, "Session owner moved"
	}
	if destination := s.data[connectionID]; destination != nil {
		r.mu.Unlock()
		return destination, 0, ""
	}
	if s.waiting == nil {
		s.waiting = make(map[string][]*relayDataWaiter)
	}
	s.waiting[connectionID] = append(s.waiting[connectionID], waiter)
	r.mu.Unlock()

	timer := time.NewTimer(time.Until(timeoutAt))
	defer timer.Stop()
	select {
	case result := <-waiter.ready:
		return result.destination, result.code, result.reason
	case <-s.ownerDone:
		r.mu.Lock()
		removeDataWaiterLocked(s, connectionID, waiter)
		r.mu.Unlock()
		return nil, websocket.StatusServiceRestart, "Session owner moved"
	case <-timer.C:
		r.mu.Lock()
		ownerClosed := s.ownerClosed.Load()
		removed := removeDataWaiterLocked(s, connectionID, waiter)
		r.mu.Unlock()
		if ownerClosed {
			return nil, websocket.StatusServiceRestart, "Session owner moved"
		}
		if removed {
			return nil, timeoutCode, timeoutReason
		}
		// An attach or teardown won the race with the timer and has already
		// placed the result in the buffered channel.
		result := <-waiter.ready
		return result.destination, result.code, result.reason
	}
}

func removeDataWaiterLocked(s *relaySession, connectionID string, wanted *relayDataWaiter) bool {
	waiters := s.waiting[connectionID]
	for i, waiter := range waiters {
		if waiter != wanted {
			continue
		}
		waiters = append(waiters[:i:i], waiters[i+1:]...)
		if len(waiters) == 0 {
			delete(s.waiting, connectionID)
		} else {
			s.waiting[connectionID] = waiters
		}
		return true
	}
	return false
}

func notifyDataWaiters(waiters []*relayDataWaiter, result relayDataWaitResult) {
	for _, waiter := range waiters {
		waiter.ready <- result
	}
}

// reserveIngress takes a routing reservation. The in-flight share is published
// before the reservation itself and retired after it, so a capacity snapshot
// taken mid-route always over-accounts rather than under-accounts and can never
// mistake a live route for a leak.
func (r *Relay) reserveIngress(bytes int64) bool {
	if bytes < 0 {
		return false
	}
	r.ingressInFlight.Add(bytes)
	if reserveCounter(&r.ingressReserved, bytes, int64(r.Config.IngressBudgetBytes)) {
		return true
	}
	r.ingressInFlight.Add(-bytes)
	return false
}

// releaseInFlight retires a reservation when its delivery or data-attach wait
// completes.
func (r *Relay) releaseInFlight(bytes int64) {
	r.ingressReserved.Add(-bytes)
	r.ingressInFlight.Add(-bytes)
}

func (r *Relay) removePeer(c Connection, p *relayPeer) {
	r.mu.Lock()
	s := r.sessions[c.ServerID]
	if s == nil {
		r.mu.Unlock()
		return
	}
	switch c.kind() {
	case peerV1Server:
		if s.v1 == p {
			s.v1 = nil
		}
	case peerV1Client:
		if s.v1Client == p {
			s.v1Client = nil
		}
	case peerControl:
		if s.control == p {
			s.control = nil
		}
	case peerV2Client:
		// Only the last client of a route tears the route down; the others just
		// leave the fan-out set.
		if remaining, attached := detachClientLocked(s, c.ConnectionID, p); attached && remaining == 0 {
			data := s.data[c.ConnectionID]
			delete(s.data, c.ConnectionID)
			control := s.control
			s.publishPeersLocked()
			r.reclaimSessionLocked(c.ServerID, s)
			r.mu.Unlock()
			if data != nil {
				_ = data.conn.Close(websocket.StatusGoingAway, "Client disconnected")
			}
			if control != nil {
				_ = r.sendControl(control, disconnectedFrame(c.ConnectionID))
			}
			return
		}
	case peerV2Data:
		// A route without its data socket cannot be served, so the clients on it
		// are told the daemon side is gone rather than left waiting.
		if s.data[c.ConnectionID] == p {
			delete(s.data, c.ConnectionID)
			orphaned := s.clients[c.ConnectionID]
			waiting := append([]*relayDataWaiter(nil), s.waiting[c.ConnectionID]...)
			delete(s.waiting, c.ConnectionID)
			s.publishPeersLocked()
			r.reclaimSessionLocked(c.ServerID, s)
			r.mu.Unlock()
			notifyDataWaiters(waiting, relayDataWaitResult{
				code:   websocket.StatusServiceRestart,
				reason: "Server disconnected",
			})
			for _, client := range orphaned {
				closeAsync(client.conn, websocket.StatusServiceRestart, "Server disconnected")
			}
			return
		}
	}
	s.publishPeersLocked()
	r.reclaimSessionLocked(c.ServerID, s)
	r.mu.Unlock()
}

// detachClientLocked removes p from its route, deleting the route when p was
// its last client, and reports how many clients remain and whether p was
// attached at all. The replacement slice is freshly built so in-flight fan-outs
// holding the old value are unaffected.
func detachClientLocked(s *relaySession, connectionID string, p *relayPeer) (remaining int, attached bool) {
	peers := s.clients[connectionID]
	i := slices.Index(peers, p)
	if i < 0 {
		return len(peers), false
	}
	rest := append(peers[:i:i], peers[i+1:]...)
	if len(rest) == 0 {
		delete(s.clients, connectionID)
	} else {
		s.clients[connectionID] = rest
	}
	return len(rest), true
}

func (r *Relay) reclaimSessionLocked(serverID string, s *relaySession) {
	if s.v1 == nil && s.v1Client == nil && s.control == nil && len(s.clients) == 0 && len(s.data) == 0 && len(s.waiting) == 0 {
		delete(r.sessions, serverID)
		if value, ok := r.ownerSessions.Load(serverID); ok && value == s {
			r.ownerSessions.Delete(serverID)
		}
		delete(r.moved, serverID)
		_ = r.ownership.release(serverID, r)
	}
}

func (r *Relay) ready() bool {
	limit, valid := connectionCapacityLimit(r.Config)
	return !r.Draining() && r.readyForAdmission() && valid && r.activeWebSockets.Load() < limit
}

// readyForAdmission excludes drain: a drain only refuses new session claims,
// so sockets joining a session this node already owns are still admitted.
func (r *Relay) readyForAdmission() bool {
	members, err := r.ownership.members()
	return err == nil && members >= r.Config.MinimumClusterSize &&
		!r.capacityConfigurationMismatch() && !r.capacityUnavailable.Load() && !r.memoryPressure.Load()
}

// capacityAdmissionReason returns the exact reference body for a local
// capacity rejection. Ownership is checked before this function so a remote
// owner still receives a 409 reroute during local pressure.
func (r *Relay) capacityAdmissionReason() string {
	if r.memoryPressure.Load() {
		return "Relay memory pressure"
	}
	if r.capacityConfigurationMismatch() {
		return "Relay capacity configuration"
	}
	if limit, valid := connectionCapacityLimit(r.Config); !valid {
		return "Relay capacity configuration"
	} else if r.activeWebSockets.Load() >= limit {
		return "Relay connection capacity"
	}
	if r.capacityUnavailable.Load() {
		return "Relay capacity unavailable"
	}
	return ""
}

func (r *Relay) capacityConfigurationMismatch() bool {
	limit, valid := connectionCapacityLimit(r.Config)
	return !valid || !r.connectionLimitValid || limit != r.connectionLimit
}

func connectionCapacityLimit(config Config) (int64, bool) {
	if config.Acceptors <= 0 || config.ConnectionsPerAcceptor <= 0 {
		return 0, false
	}
	acceptors := int64(config.Acceptors)
	connections := int64(config.ConnectionsPerAcceptor)
	maxInt64 := int64(^uint64(0) >> 1)
	if acceptors > maxInt64/connections {
		return 0, false
	}
	return acceptors * connections, true
}

// startTicker runs tick on interval until the returned stop is called, which
// waits for the goroutine to exit so a stopped watcher never races a test's
// next assertion. A non-positive interval disables the ticker entirely.
func startTicker(interval time.Duration, tick func()) func() {
	if interval <= 0 {
		return func() {}
	}
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				tick()
			case <-stop:
				return
			}
		}
	}()
	return func() { close(stop); <-done }
}

// watchCapacity reconciles the ingress ledger against live state. The interval
// is PASEO_RELAY_CAPACITY_MUTATION_TIMEOUT_MS: the longest an inconsistency may
// persist before admission is closed and the ledger corrected.
func (r *Relay) watchCapacity() func() {
	interval := time.Duration(r.Config.CapacityMutationTimeoutMS) * time.Millisecond
	return startTicker(interval, r.reconcileCapacity)
}

// reconcileCapacity releases ingress reservations that no live delivery or
// data-attach wait accounts for. Without it a reservation orphaned by an
// unusual teardown is held until restart, shrinking the effective budget for
// good.
func (r *Relay) reconcileCapacity() {
	r.mu.Lock()
	accounted := r.ingressInFlight.Load()
	r.mu.Unlock()

	leaked := r.ingressReserved.Load() - accounted
	if leaked <= 0 {
		return
	}
	// Admission stays shut across the correction so no upgrade reserves against
	// a budget that is mid-mutation.
	r.capacityUnavailable.Store(true)
	r.capacityEpoch.Add(1)
	r.ingressReserved.Add(-leaked)
	r.capacityUnavailable.Store(false)
}

const (
	memoryPressureInterval = 250 * time.Millisecond
	// initialShedBatch keeps the first shed of a crossing small; the sampler
	// grows it only while shedding fails to reclaim anything.
	initialShedBatch = 8
	// shedGCInterval is the shortest gap between the forced collections that
	// let a pressure episode observe its own relief.
	shedGCInterval = time.Second
	// controlSyncDelay is how long a client route may wait for its data socket
	// before the relay re-sends sync, and controlCloseDelay how long after that
	// re-send the control socket has to produce one.
	controlSyncDelay  = 10 * time.Second
	controlCloseDelay = 5 * time.Second
	ownerCallTimeout  = 5 * time.Second
)

// readMemoryMetrics reads the named runtime/metrics uint64 samples in order.
// runtime/metrics rather than runtime.ReadMemStats because both callers run
// while the relay carries live traffic and ReadMemStats stops the world.
func readMemoryMetrics(names ...string) []uint64 {
	samples := make([]metrics.Sample, len(names))
	for i, name := range names {
		samples[i].Name = name
	}
	metrics.Read(samples)
	values := make([]uint64, len(samples))
	for i, sample := range samples {
		values[i] = sample.Value.Uint64()
	}
	return values
}

// heapInUse reports the memory the Go runtime currently holds from the OS. It
// reads runtime/metrics rather than runtime.ReadMemStats because the sampler
// runs continuously and ReadMemStats stops the world.
func heapInUse() uint64 {
	read := readMemoryMetrics("/memory/classes/total:bytes", "/memory/classes/heap/released:bytes")
	total, released := read[0], read[1]
	if released > total {
		return 0
	}
	return total - released
}

// watchMemoryPressure samples memory use against the configured watermark and
// returns a function that stops the sampler. A zero watermark disables it.
func (r *Relay) watchMemoryPressure() func() {
	if r.Config.MemoryWatermarkBytes <= 0 {
		return func() {}
	}
	return startTicker(memoryPressureInterval, func() { r.sampleMemoryPressure(heapInUse()) })
}

// sampleMemoryPressure applies one reading. Crossing the watermark closes
// admission and starts shedding; every further sample above the recovery
// threshold sheds another batch, so a crossing drains gradually instead of
// disconnecting the whole node at once.
func (r *Relay) sampleMemoryPressure(inUse uint64) {
	if r.Config.MemoryWatermarkBytes <= 0 {
		return
	}
	if r.memoryPressure.Load() {
		if inUse <= memoryRecoveryThreshold(r.Config.MemoryWatermarkBytes) {
			r.memoryPressure.Store(false)
			r.shedBatch = 0
			r.shedGCAt = time.Time{}
			return
		}
		r.shedNextBatch(inUse, inUse >= r.shedHeapBefore)
		return
	}
	if inUse < uint64(r.Config.MemoryWatermarkBytes) {
		return
	}
	// Only the goroutine that wins the transition sheds, so a shed storm cannot
	// be triggered twice for one crossing.
	if !r.memoryPressure.CompareAndSwap(false, true) {
		return
	}
	r.shedNextBatch(inUse, false)
}

// shedNextBatch owns the batch-size lifecycle: a batch that reclaimed nothing
// was too small to matter, so grow doubles it until shedding starts moving
// memory; the floor applies on the first batch of a crossing. It records the
// reading the next reclaim is measured against and sheds.
func (r *Relay) shedNextBatch(inUse uint64, grow bool) {
	if grow {
		r.shedBatch *= 2
	}
	if r.shedBatch < initialShedBatch {
		r.shedBatch = initialShedBatch
	}
	r.shedHeapBefore = inUse
	r.shedForMemoryPressure(r.shedBatch)
}

// memoryRecoveryThreshold is the level pressure holds until, one maximum
// message below the watermark so a node that recovers has room to accept one.
func memoryRecoveryThreshold(watermark int) uint64 {
	if watermark <= MaximumMessagePayloadBytes {
		return 0
	}
	return uint64(watermark - MaximumMessagePayloadBytes)
}

// attachedPeersLocked snapshots every socket attached to this node. Callers
// hold r.mu; the ordering work shedding does with the result is deliberately
// left outside the lock.
func (r *Relay) attachedPeersLocked() []*relayPeer {
	var peers []*relayPeer
	for _, session := range r.sessions {
		peers = append(peers, sessionPeers(session)...)
	}
	return peers
}

// shedCandidates picks up to batch sockets to close, ordered by sources that
// have been blocked on a delivery longest first, then the most recently
// attached sockets. Blocking longest means holding memory longest, and among
// the rest the newest arrival is the one whose loss costs the least
// established work. Peers already shed stay attached until their read loop
// unwinds, so they are skipped to keep a batch a batch of distinct sockets.
func shedCandidates(peers []*relayPeer, batch int) []*relayPeer {
	var blocked, active []*relayPeer
	for _, peer := range peers {
		switch {
		case peer.shed.Load():
		case peer.blockSeq.Load() != 0:
			blocked = append(blocked, peer)
		default:
			active = append(active, peer)
		}
	}
	sort.Slice(blocked, func(i, j int) bool { return blocked[i].blockSeq.Load() < blocked[j].blockSeq.Load() })
	if len(blocked) >= batch {
		return blocked[:batch]
	}
	// Only the sockets the batch can still reach need ordering at all.
	sort.Slice(active, func(i, j int) bool { return active[i].attachSeq.Load() > active[j].attachSeq.Load() })
	candidates := append(blocked, active...)
	if len(candidates) > batch {
		candidates = candidates[:batch]
	}
	return candidates
}

// shedForMemoryPressure wakes any selected source waiting for data and closes
// up to batch attached peers, then returns the freed memory to the runtime so
// the next sample can observe the relief.
func (r *Relay) shedForMemoryPressure(batch int) {
	r.mu.Lock()
	attached := r.attachedPeersLocked()
	r.mu.Unlock()
	// Ordering a batch walks and sorts every socket on the node, which is too
	// much work to hold the global lock for four times a second.
	peers := shedCandidates(attached, batch)

	r.mu.Lock()
	for _, peer := range peers {
		peer.shed.Store(true)
	}
	var waiting []relayDataWaitResultWithWaiter
	for _, session := range r.sessions {
		for connectionID, waiters := range session.waiting {
			kept := waiters[:0]
			for _, waiter := range waiters {
				if slices.Contains(peers, waiter.source) {
					waiting = append(waiting, relayDataWaitResultWithWaiter{
						waiter: waiter,
						result: relayDataWaitResult{
							code:   websocket.StatusTryAgainLater,
							reason: "Relay memory pressure",
						},
					})
					continue
				}
				kept = append(kept, waiter)
			}
			if len(kept) == 0 {
				delete(session.waiting, connectionID)
			} else {
				session.waiting[connectionID] = kept
			}
		}
	}
	r.mu.Unlock()

	for _, wake := range waiting {
		wake.waiter.ready <- wake.result
	}
	for _, peer := range peers {
		closeAsync(peer.conn, websocket.StatusTryAgainLater, "Relay memory pressure")
	}
	r.memoryPressureDisconnects.Add(int64(len(peers)))
	// Without a collection the shed memory stays uncollected and pressure never
	// clears, because the pressure reading only falls once the runtime frees
	// memory. It is rate-limited rather than run on every batch so a long
	// episode does not stop the world four times a second.
	if now := time.Now(); r.shedGCAt.IsZero() || now.Sub(r.shedGCAt) >= shedGCInterval {
		r.shedGCAt = now
		runtime.GC()
	}
}

// watchOwnership starts the cluster reconciler and returns a function that
// stops it and waits for it to exit. Backends without a cluster identity have
// nothing to reconcile, so the returned stop is a no-op.
func (r *Relay) watchOwnership() func() {
	if r.ownership.identity() == "" {
		return func() {}
	}
	return startTicker(clusterHeartbeatInterval, r.closeLostSessions)
}

func (r *Relay) closeLostSessions() {
	r.mu.Lock()
	serverIDs := make([]string, 0, len(r.sessions))
	for serverID := range r.sessions {
		serverIDs = append(serverIDs, serverID)
	}
	r.mu.Unlock()
	if len(serverIDs) == 0 {
		return
	}
	owned, err := r.ownership.ownedServers()
	if err != nil {
		return
	}
	for _, serverID := range serverIDs {
		if owned[serverID] {
			continue
		}
		r.mu.Lock()
		session := r.sessions[serverID]
		peers := sessionPeers(session)
		r.moved[serverID] = true
		var waiting []*relayDataWaiter
		if session != nil {
			session.markOwnerClosed()
			for connectionID, routeWaiters := range session.waiting {
				waiting = append(waiting, routeWaiters...)
				delete(session.waiting, connectionID)
			}
		}
		r.mu.Unlock()
		for _, waiter := range waiting {
			waiter.ready <- relayDataWaitResult{
				code:   websocket.StatusServiceRestart,
				reason: "Session owner moved",
			}
		}
		for _, peer := range peers {
			_ = peer.conn.Close(websocket.StatusServiceRestart, "Session owner moved")
		}
	}
}

func sessionPeers(session *relaySession) []*relayPeer {
	if session == nil {
		return nil
	}
	peers := []*relayPeer{session.v1, session.v1Client, session.control}
	for _, route := range session.clients {
		peers = append(peers, route...)
	}
	for _, peer := range session.data {
		peers = append(peers, peer)
	}
	result := peers[:0]
	for _, peer := range peers {
		if peer != nil {
			result = append(result, peer)
		}
	}
	return result
}

func (r *Relay) reserveWebSocket() bool {
	limit, valid := connectionCapacityLimit(r.Config)
	return valid && !r.capacityConfigurationMismatch() && reserveCounter(&r.activeWebSockets, 1, limit)
}

// reserveCounter adds amount to counter unless that would exceed limit. A
// non-positive limit means unbounded.
func reserveCounter(counter *atomic.Int64, amount, limit int64) bool {
	for {
		current := counter.Load()
		if limit > 0 && current+amount > limit {
			return false
		}
		if counter.CompareAndSwap(current, current+amount) {
			return true
		}
	}
}

// writeText writes an exact plain-text body, without http.Error's trailing
// newline, so 503 diagnostics match the reference byte for byte.
func writeText(writer http.ResponseWriter, status int, body string) {
	writer.Header().Set("x-content-type-options", "nosniff")
	writeBody(writer, status, "text/plain; charset=utf-8", body)
}

func writeJSON(writer http.ResponseWriter, status int, body string) {
	writeBody(writer, status, "application/json", body)
}

func writeBody(writer http.ResponseWriter, status int, contentType, body string) {
	writer.Header().Set("content-type", contentType)
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

// metricsSnapshot carries the per-scrape values that do not live in Relay
// atomics: readiness, the session count taken under r.mu, and the runtime
// memory classes backing the beam_* compatibility gauges.
type metricsSnapshot struct {
	ready          int64
	draining       int64
	activeSessions int64
	heapAlloc      int64
	heapInuse      int64
	mcacheInuse    int64
}

type metricDefinition struct {
	name       string
	metricType string
	help       string
	capacity   bool
	read       func(*Relay, metricsSnapshot) int64
}

var relayMetricDefinitions = [...]metricDefinition{
	{"paseo_relay_ready", "gauge", "Whether this node admits new relay work.", false,
		func(_ *Relay, s metricsSnapshot) int64 { return s.ready }},
	{"paseo_relay_draining", "gauge", "Whether this node is draining.", false,
		func(_ *Relay, s metricsSnapshot) int64 { return s.draining }},
	{"paseo_relay_active_websockets", "gauge", "Open WebSocket connections on this node.", true,
		func(r *Relay, _ metricsSnapshot) int64 { return r.activeWebSockets.Load() }},
	{"paseo_relay_active_sessions", "gauge", "Relay sessions owned by this node.", false,
		func(_ *Relay, s metricsSnapshot) int64 { return s.activeSessions }},
	{"paseo_relay_reroute_responses_total", "counter", "WebSocket upgrades rerouted to another owner.", false,
		func(r *Relay, _ metricsSnapshot) int64 { return r.rerouteResponses.Load() }},
	{"paseo_relay_connection_rejections_total", "counter", "WebSocket upgrades rejected at configured capacity or during memory pressure.", false,
		func(r *Relay, _ metricsSnapshot) int64 { return r.connectionRejections.Load() }},
	{"paseo_relay_frames_forwarded_total", "counter", "WebSocket frames forwarded by this node.", false,
		func(r *Relay, _ metricsSnapshot) int64 { return r.framesForwarded.Load() }},
	{"paseo_relay_bytes_forwarded_total", "counter", "WebSocket payload bytes forwarded by this node.", false,
		func(r *Relay, _ metricsSnapshot) int64 { return r.bytesForwarded.Load() }},
	{"paseo_relay_ingress_reserved_bytes", "gauge", "Weighted ingress bytes admitted on this node.", true,
		func(r *Relay, _ metricsSnapshot) int64 { return r.ingressReserved.Load() }},
	{"paseo_relay_inflight_delivery_bytes", "gauge", "Payload bytes currently held by synchronous downstream delivery.", true,
		func(r *Relay, _ metricsSnapshot) int64 { return r.inflightDelivery.Load() }},
	{"paseo_relay_backpressured_sources", "gauge", "Source WebSockets currently waiting for downstream delivery.", true,
		func(r *Relay, _ metricsSnapshot) int64 { return r.backpressuredSources.Load() }},
	{"paseo_relay_slow_consumer_disconnects_total", "counter", "Destinations disconnected after exceeding a delivery deadline.", false,
		func(r *Relay, _ metricsSnapshot) int64 { return r.slowConsumerDisconnects.Load() }},
	{"paseo_relay_delivery_timeouts_total", "counter", "Synchronous downstream deliveries that exceeded their deadline.", false,
		func(r *Relay, _ metricsSnapshot) int64 { return r.deliveryTimeouts.Load() }},
	{"paseo_relay_memory_pressure_disconnects_total", "counter", "WebSockets closed by node memory-pressure recovery.", false,
		func(r *Relay, _ metricsSnapshot) int64 { return r.memoryPressureDisconnects.Load() }},
	{"paseo_relay_max_frame_bytes", "gauge", "Largest WebSocket frame payload observed since node start.", false,
		func(r *Relay, _ metricsSnapshot) int64 { return r.maxFrameBytes.Load() }},
	{"paseo_relay_beam_total_memory_bytes", "gauge", "Total memory allocated by BEAM.", false,
		func(_ *Relay, s metricsSnapshot) int64 { return s.heapAlloc }},
	{"paseo_relay_beam_process_memory_bytes", "gauge", "Memory allocated by BEAM processes.", false,
		func(_ *Relay, s metricsSnapshot) int64 { return s.heapAlloc }},
	{"paseo_relay_beam_binary_memory_bytes", "gauge", "Memory allocated for BEAM binaries.", false,
		func(_ *Relay, s metricsSnapshot) int64 { return s.heapInuse }},
	{"paseo_relay_beam_ets_memory_bytes", "gauge", "Memory allocated for BEAM ETS tables.", false,
		func(_ *Relay, s metricsSnapshot) int64 { return s.mcacheInuse }},
}

type histogramBucketDefinition struct {
	limit int64
	label string
}

var deliveryWaitBucketDefinitions = [...]histogramBucketDefinition{
	{1_000, "0.001"},
	{10_000, "0.01"},
	{100_000, "0.1"},
	{1_000_000, "1"},
	{10_000_000, "10"},
}

var frameSizeBucketDefinitions = [...]histogramBucketDefinition{
	{1024, "1024"},
	{64 * 1024, "65536"},
	{1024 * 1024, "1048576"},
	{8 * 1024 * 1024, "8388608"},
	{MaximumMessagePayloadBytes, "33554418"},
}

func writeMetric(writer io.Writer, name, metricType, help string, value int64) {
	_, _ = fmt.Fprintf(writer, "# HELP %s %s\n# TYPE %s %s\n%s %d\n", name, help, name, metricType, name, value)
}

func (r *Relay) renderHandshakeMetrics(writer io.Writer) {
	outcomes := []string{"accepted", "rejected"}
	types := []string{"hello", "e2ee_hello"}
	for outcome, label := range outcomes {
		name := "paseo_relay_handshake_" + label + "_total"
		_, _ = fmt.Fprintf(writer, "# HELP %s Client E2EE handshake frames %s by the handshake input validator.\n# TYPE %s counter\n", name, label, name)
		for version := 0; version < 2; version++ {
			for kind, handshake := range types {
				_, _ = fmt.Fprintf(writer, "%s{routing_version=\"v%d\",type=\"%s\"} %d\n", name, version+1, handshake, r.handshakes[outcome][version][kind].Load())
			}
		}
	}
}

func (r *Relay) observeFrame(size int) {
	r.frameCount.Add(1)
	r.frameBytes.Add(int64(size))
	observeHistogramBucket(r.frameSizeBuckets[:], frameSizeBucketDefinitions[:], int64(size))
	for {
		current := r.maxFrameBytes.Load()
		if int64(size) <= current || r.maxFrameBytes.CompareAndSwap(current, int64(size)) {
			return
		}
	}
}

func (r *Relay) renderHistograms(writer io.Writer) {
	renderHistogram(writer, "paseo_relay_delivery_wait_seconds",
		"Time a source waits for synchronous downstream delivery.",
		deliveryWaitBucketDefinitions[:], r.deliveryWaitBuckets[:],
		r.deliveryWaitCount.Load(), formatPrometheusFloat(float64(r.deliveryWaitMicroseconds.Load())/1_000_000))
	renderHistogram(writer, "paseo_relay_frame_size_bytes",
		"WebSocket payload-size distribution.",
		frameSizeBucketDefinitions[:], r.frameSizeBuckets[:],
		r.frameCount.Load(), strconv.FormatInt(r.frameBytes.Load(), 10))
}

func renderHistogram(writer io.Writer, name, help string, definitions []histogramBucketDefinition, buckets []atomic.Int64, count int64, sum string) {
	_, _ = fmt.Fprintf(writer, "# HELP %s %s\n# TYPE %s histogram\n", name, help, name)
	cumulative := int64(0)
	for i := range definitions {
		cumulative += buckets[i].Load()
		_, _ = fmt.Fprintf(writer, "%s_bucket{le=\"%s\"} %d\n", name, definitions[i].label, cumulative)
	}
	_, _ = fmt.Fprintf(writer, "%s_bucket{le=\"+Inf\"} %d\n%s_sum %s\n%s_count %d\n", name, count, name, sum, name, count)
}

func formatPrometheusFloat(value float64) string {
	formatted := strconv.FormatFloat(value, 'f', -1, 64)
	if !strings.Contains(formatted, ".") {
		formatted += ".0"
	}
	return formatted
}
