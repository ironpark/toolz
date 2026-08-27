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

	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// Relay serves the public HTTP and WebSocket relay surface.
type Relay struct {
	Config Config

	activeWebSockets          atomic.Int64
	serverMu                  sync.Mutex
	server                    *http.Server
	mu                        sync.Mutex
	sessions                  map[string]*relaySession
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
	deliveryWaitNanos         atomic.Int64
	handshakes                [2][2][2]atomic.Int64
	capacityEpoch             atomic.Int64
	listenerEpoch             atomic.Int64
	capacityUnavailable       atomic.Bool
	memoryPressure            atomic.Bool
	stalled                   map[string]bool
	moved                     map[string]bool
	// controlSyncDelay and controlCloseDelay are the two control-watchdog
	// stages. They are unexported and set by NewRelay so no environment can
	// reach them; in-package tests shorten them.
	controlSyncDelay  time.Duration
	controlCloseDelay time.Duration
	// shedBatch and shedHeapBefore carry memory-pressure shedding state between
	// sampler ticks and are only touched from the sampling path.
	shedBatch      int
	shedHeapBefore uint64
	ownership      ownershipCoordinator
}

type relayPeer struct {
	conn *websocket.Conn
	// writeSlot serializes writers. A buffered channel rather than a mutex so a
	// contended sender parks against its delivery deadline instead of spinning.
	writeSlot chan struct{}
	// shed marks a peer already chosen by memory-pressure shedding, which runs
	// again before the closed socket has left the session. Guarded by Relay.mu.
	shed bool
}

func newRelayPeer(conn *websocket.Conn) *relayPeer {
	return &relayPeer{conn: conn, writeSlot: make(chan struct{}, 1)}
}

type relaySession struct {
	v1       *relayPeer
	v1Client *relayPeer
	control  *relayPeer
	// clients holds every client socket on a route; a route fans out to all of
	// them and only empties when its last client leaves.
	clients      map[string][]*relayPeer
	data         map[string]*relayPeer
	buffer       map[string][]relayMessage
	bufferBytes  map[string]int64
	bufferTimers map[string]*time.Timer
	// watchdogFor is the control peer with a live watchdog stage-one timer.
	watchdogFor *relayPeer
}
type relayMessage struct {
	typ     websocket.MessageType
	payload []byte
}

// dropBufferLocked removes all buffered-route state for connectionID and
// returns the ingress bytes the buffer had reserved. Callers hold r.mu.
func (s *relaySession) dropBufferLocked(connectionID string) int64 {
	reserved := s.bufferBytes[connectionID]
	delete(s.buffer, connectionID)
	delete(s.bufferBytes, connectionID)
	if timer := s.bufferTimers[connectionID]; timer != nil {
		timer.Stop()
		delete(s.bufferTimers, connectionID)
	}
	return reserved
}

// NewRelay constructs a relay with validated runtime configuration.
func NewRelay(config Config) *Relay {
	ownership, err := newOwnershipCoordinator(config)
	if err != nil {
		ownership = &failedOwnershipCoordinator{err: err}
	}
	return &Relay{
		Config:            config,
		sessions:          make(map[string]*relaySession),
		stalled:           make(map[string]bool),
		moved:             make(map[string]bool),
		controlSyncDelay:  controlSyncDelay,
		controlCloseDelay: controlCloseDelay,
		ownership:         ownership,
	}
}

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
	mux.HandleFunc("/health", getOnly(r.handleHealth))
	mux.HandleFunc("/ready", getOnly(r.handleReady))
	mux.HandleFunc("/metrics", getOnly(r.handleMetrics))
	mux.HandleFunc("/ws", r.handleWebSocket)
	mux.HandleFunc("/", r.handleNotFound)
	return mux
}

// getOnly rejects any method other than GET before delegating.
func getOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		next(writer, request)
	}
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
	if r.Config.Drain {
		draining = 1
	}

	writer.Header().Set("content-type", "text/plain; version=0.0.4")
	writer.WriteHeader(http.StatusOK)
	r.mu.Lock()
	activeSessions := len(r.sessions)
	r.mu.Unlock()
	// runtime/metrics rather than runtime.ReadMemStats: scrapes arrive while the
	// relay carries live traffic and ReadMemStats stops the world.
	samples := []metrics.Sample{
		{Name: "/memory/classes/heap/objects:bytes"},
		{Name: "/memory/classes/heap/unused:bytes"},
		{Name: "/memory/classes/metadata/mcache/inuse:bytes"},
	}
	metrics.Read(samples)
	heapAlloc := samples[0].Value.Uint64()
	heapInuse := heapAlloc + samples[1].Value.Uint64()
	mcacheInuse := samples[2].Value.Uint64()
	_, _ = fmt.Fprintf(writer, metricsGaugeFormat,
		ready, draining, r.activeWebSockets.Load(), activeSessions,
		r.rerouteResponses.Load(), r.connectionRejections.Load(),
		r.framesForwarded.Load(), r.bytesForwarded.Load(),
		r.ingressReserved.Load(), r.inflightDelivery.Load(), r.backpressuredSources.Load(),
		r.slowConsumerDisconnects.Load(), r.deliveryTimeouts.Load(), r.memoryPressureDisconnects.Load(),
		r.maxFrameBytes.Load(), heapAlloc, heapAlloc, heapInuse, mcacheInuse)
	r.renderHandshakeMetrics(writer)
	r.renderHistograms(writer)
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
	// A route owned elsewhere must reroute even while this node is under local
	// pressure, so ownership is consulted before any capacity rejection. When
	// this node cannot admit anyway, a read-only lookup answers that question
	// without paying claim/release write traffic on the overload path.
	if !r.readyForAdmission() {
		if owner, ok, err := r.ownership.lookup(connection.ServerID); err == nil && ok && !owner.ownedBy(r) {
			r.rerouteResponses.Add(1)
			writer.Header().Set(r.Config.RerouteHeader, owner.target)
			writer.WriteHeader(http.StatusConflict)
			return
		}
		r.connectionRejections.Add(1)
		http.Error(writer, "relay capacity unavailable", http.StatusServiceUnavailable)
		return
	}
	owner, acquired, err := r.ownership.claim(connection.ServerID, r)
	if err != nil {
		r.connectionRejections.Add(1)
		http.Error(writer, "cluster ownership unavailable", http.StatusServiceUnavailable)
		return
	}
	if !owner.ownedBy(r) {
		r.rerouteResponses.Add(1)
		writer.Header().Set(r.Config.RerouteHeader, owner.target)
		writer.WriteHeader(http.StatusConflict)
		return
	}
	// Single undo path for every rejection between here and attach.
	attached, reserved := false, false
	defer func() {
		if attached {
			return
		}
		if reserved {
			r.activeWebSockets.Add(-1)
		}
		if acquired {
			_ = r.ownership.release(connection.ServerID, r)
		}
	}()
	if !r.reserveWebSocket() {
		r.connectionRejections.Add(1)
		http.Error(writer, "relay capacity unavailable", http.StatusServiceUnavailable)
		return
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
		return
	}
	attached = true
	defer conn.CloseNow()
	defer r.activeWebSockets.Add(-1)

	readLimit := int64(MaximumFrameWireBytes)
	if connection.isControl() {
		readLimit = MaximumControlPayloadBytes + 1
	}
	conn.SetReadLimit(readLimit)
	peer := newRelayPeer(conn)
	r.mu.Lock()
	s := r.sessions[connection.ServerID]
	if s == nil {
		s = &relaySession{clients: map[string][]*relayPeer{}, data: map[string]*relayPeer{}, buffer: map[string][]relayMessage{}, bufferBytes: map[string]int64{}, bufferTimers: map[string]*time.Timer{}}
		r.sessions[connection.ServerID] = s
	}
	if connection.Version == 1 {
		if connection.Role == RoleServer {
			s.v1 = peer
		} else {
			s.v1Client = peer
		}
		r.mu.Unlock()
	} else if connection.isControl() {
		s.control = peer
		ids := clientRouteIDsLocked(s)
		r.armControlWatchdogLocked(s, peer)
		r.mu.Unlock()
		r.sendSync(peer, ids)
	} else if connection.Role == RoleClient {
		if r.moved[connection.ServerID] {
			r.mu.Unlock()
			_ = conn.Close(websocket.StatusServiceRestart, "Session expired")
			return
		}
		// Clients coexist on one route, so a second client is an addition and
		// never replaces the first.
		s.clients[connection.ConnectionID] = append(s.clients[connection.ConnectionID], peer)
		control := s.control
		r.armControlWatchdogLocked(s, control)
		r.mu.Unlock()
		if control != nil {
			r.send(control, websocket.MessageText, []byte(`{"type":"connected","connectionId":"`+connection.ConnectionID+`"}`))
		}
	} else {
		old := s.data[connection.ConnectionID]
		s.data[connection.ConnectionID] = peer
		buffered := s.buffer[connection.ConnectionID]
		bufferedBytes := s.dropBufferLocked(connection.ConnectionID)
		r.ingressReserved.Add(-bufferedBytes)
		r.mu.Unlock()
		if old != nil {
			_ = old.conn.Close(websocket.StatusPolicyViolation, "Replaced by new connection")
		}
		for _, m := range buffered {
			_ = r.forward(peer, m.typ, m.payload)
		}
	}
	defer func() { r.removePeer(connection, peer) }()
	for {
		typ, payload, err := conn.Read(context.Background())
		if err != nil {
			return
		}
		payloadLimit := MaximumMessagePayloadBytes
		if connection.isControl() {
			payloadLimit = MaximumControlPayloadBytes
		}
		if len(payload) > payloadLimit {
			_ = conn.Close(websocket.StatusMessageTooBig, "Message too big")
			return
		}
		r.observeFrame(len(payload))
		if connection.Role == RoleClient && !r.validateHandshake(connection.Version, payload) {
			_ = conn.Close(websocket.StatusPolicyViolation, "Invalid handshake key")
			return
		}
		if connection.isControl() && typ == websocket.MessageText {
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
				_ = r.forward(peer, websocket.MessageText, []byte(`{"type":"pong","ts":`+strconv.FormatInt(time.Now().UnixMilli(), 10)+`}`))
				continue
			}
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
	b, _ := json.Marshal(struct {
		Type          string   `json:"type"`
		ConnectionIDs []string `json:"connectionIds"`
	}{Type: "sync", ConnectionIDs: ids})
	_ = r.send(control, websocket.MessageText, b)
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

func (r *Relay) send(p *relayPeer, typ websocket.MessageType, b []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(r.Config.DeliveryTimeoutMS)*time.Millisecond)
	defer cancel()
	select {
	case p.writeSlot <- struct{}{}:
	case <-ctx.Done():
		return context.DeadlineExceeded
	}
	defer func() { <-p.writeSlot }()
	return p.conn.Write(ctx, typ, b)
}

func (r *Relay) forward(p *relayPeer, typ websocket.MessageType, b []byte) error {
	started := time.Now()
	r.inflightDelivery.Add(int64(len(b)))
	r.backpressuredSources.Add(1)
	err := r.send(p, typ, b)
	r.backpressuredSources.Add(-1)
	r.inflightDelivery.Add(-int64(len(b)))
	r.deliveryWaitCount.Add(1)
	r.deliveryWaitNanos.Add(time.Since(started).Nanoseconds())
	if err == nil {
		r.framesForwarded.Add(1)
		r.bytesForwarded.Add(int64(len(b)))
		return nil
	}
	r.deliveryTimeouts.Add(1)
	r.slowConsumerDisconnects.Add(1)
	r.capacityEpoch.Add(1)
	closeAsync(p.conn, websocket.StatusTryAgainLater, "Slow consumer")
	return err
}

// closeAsync closes conn off the calling path: Close waits for the peer's
// close frame, and neither delivery nor shedding may stall on one socket.
func closeAsync(conn *websocket.Conn, code websocket.StatusCode, reason string) {
	go func() { _ = conn.Close(code, reason) }()
}

type handshakeFrame struct {
	Type string          `json:"type"`
	Key  json.RawMessage `json:"key"`
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
// The second result is false when key is not a JSON string at all, which leaves
// the frame unclassifiable and sends callers to the byte-level fallback.
func acceptableKey(raw json.RawMessage) (bool, bool) {
	key := ""
	if raw != nil && json.Unmarshal(raw, &key) != nil {
		return false, false
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(key)
	if err != nil || len(decoded) != 32 || !canonicalCoordinate(decoded) {
		return false, true
	}
	pub, err := ecdh.X25519().NewPublicKey(decoded)
	if err != nil {
		return false, true
	}
	priv, err := probeKey()
	if err != nil {
		return false, true
	}
	// Rejects low-order points, which yield an all-zero shared secret.
	_, err = priv.ECDH(pub)
	return err == nil, true
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

// acceptedHandshake decides an already-decoded hello, falling back to a byte
// scan for frames whose key field is not a string.
func acceptedHandshake(b []byte, frame handshakeFrame) bool {
	accepted, classified := acceptableKey(frame.Key)
	if !classified {
		return !bytes.Contains(b, []byte(`"type":"e2ee_hello"`))
	}
	return accepted
}

// validHandshake reports whether a text frame is an acceptable hello. Frames
// that are not handshakes pass through untouched.
func validHandshake(b []byte) bool {
	var frame handshakeFrame
	if json.Unmarshal(b, &frame) != nil {
		return !bytes.Contains(b, []byte(`"type":"e2ee_hello"`))
	}
	if _, handshake := handshakeKind(frame); !handshake {
		return true
	}
	return acceptedHandshake(b, frame)
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
	accepted := acceptedHandshake(b, frame)
	if !accepted {
		outcome = 1
	}
	r.handshakes[outcome][version-1][kind].Add(1)
	return accepted
}

func (r *Relay) route(c Connection, source *relayPeer, typ websocket.MessageType, b []byte) {
	weighted := int64(len(b) * r.Config.IngressWeight)
	if !r.reserveIngress(weighted) {
		_ = source.conn.Close(websocket.StatusTryAgainLater, "Relay ingress capacity")
		return
	}
	r.mu.Lock()
	s := r.sessions[c.ServerID]
	var destinations []*relayPeer
	buffered := false
	if c.Version == 1 {
		peer := s.v1
		if c.Role != RoleClient {
			peer = s.v1Client
		}
		if peer != nil {
			destinations = []*relayPeer{peer}
		}
	} else if c.Role == RoleClient {
		if d := s.data[c.ConnectionID]; d != nil {
			destinations = []*relayPeer{d}
		} else {
			buffered = true
			s.buffer[c.ConnectionID] = append(s.buffer[c.ConnectionID], relayMessage{typ: typ, payload: append([]byte(nil), b...)})
			s.bufferBytes[c.ConnectionID] += weighted
			r.ingressInFlight.Add(-weighted)
			if s.bufferTimers[c.ConnectionID] == nil {
				s.bufferTimers[c.ConnectionID] = time.AfterFunc(time.Duration(r.Config.DataAttachTimeoutMS)*time.Millisecond, func() {
					r.expireDataRoute(c.ServerID, c.ConnectionID, source)
				})
			}
		}
	} else if c.ConnectionID != "" {
		// Route slices are appended to or replaced wholesale, never mutated in
		// place, so the map value can be shared with the fan-out without copying.
		destinations = s.clients[c.ConnectionID]
	}
	r.mu.Unlock()
	// One surviving destination is enough: forward closes a slow destination
	// itself, and only an entirely failed fan-out reaches back to the source.
	delivered := false
	if len(destinations) == 1 {
		delivered = r.forward(destinations[0], typ, b) == nil
	} else if len(destinations) > 1 {
		// Deliver concurrently so one blocked destination costs the fan-out a
		// single delivery timeout rather than one per destination.
		var wg sync.WaitGroup
		var successes atomic.Int64
		for _, destination := range destinations {
			wg.Add(1)
			go func(destination *relayPeer) {
				defer wg.Done()
				if r.forward(destination, typ, b) == nil {
					successes.Add(1)
				}
			}(destination)
		}
		wg.Wait()
		delivered = successes.Load() > 0
	}
	if len(destinations) > 0 && !delivered {
		_ = source.conn.Close(websocket.StatusTryAgainLater, "Delivery unavailable")
	}
	// A buffered frame keeps its reservation until attach or expiry retires it.
	if !buffered {
		r.releaseInFlight(weighted)
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

// releaseInFlight retires a reservation that never became a buffered frame.
func (r *Relay) releaseInFlight(bytes int64) {
	r.ingressReserved.Add(-bytes)
	r.ingressInFlight.Add(-bytes)
}

func (r *Relay) expireDataRoute(serverID, connectionID string, source *relayPeer) {
	r.mu.Lock()
	s := r.sessions[serverID]
	if s == nil || !slices.Contains(s.clients[connectionID], source) || s.data[connectionID] != nil || len(s.buffer[connectionID]) == 0 {
		r.mu.Unlock()
		return
	}
	r.ingressReserved.Add(-s.dropBufferLocked(connectionID))
	r.mu.Unlock()
	_ = source.conn.Close(websocket.StatusTryAgainLater, "Data route unavailable")
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
		// Only the last client of a route tears the route down; the others just
		// leave the fan-out set.
		if remaining, attached := detachClientLocked(s, c.ConnectionID, p); attached && remaining == 0 {
			r.ingressReserved.Add(-s.dropBufferLocked(c.ConnectionID))
			data := s.data[c.ConnectionID]
			delete(s.data, c.ConnectionID)
			control := s.control
			r.reclaimSessionLocked(c.ServerID, s)
			r.mu.Unlock()
			if data != nil {
				_ = data.conn.Close(websocket.StatusGoingAway, "Client disconnected")
			}
			if control != nil {
				r.send(control, websocket.MessageText, []byte(`{"type":"disconnected","connectionId":"`+c.ConnectionID+`"}`))
			}
			return
		}
		if s.data[c.ConnectionID] == p {
			delete(s.data, c.ConnectionID)
		}
	}
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
	if s.v1 == nil && s.v1Client == nil && s.control == nil && len(s.clients) == 0 && len(s.data) == 0 && len(s.buffer) == 0 {
		delete(r.sessions, serverID)
		delete(r.stalled, serverID)
		delete(r.moved, serverID)
		_ = r.ownership.release(serverID, r)
	}
}

func (r *Relay) ready() bool {
	return r.readyForAdmission() && r.activeWebSockets.Load() < int64(r.Config.Acceptors*r.Config.ConnectionsPerAcceptor)
}

func (r *Relay) readyForAdmission() bool {
	members, err := r.ownership.members()
	return err == nil && !r.Config.Drain && members >= r.Config.MinimumClusterSize && !r.capacityUnavailable.Load() && !r.memoryPressure.Load()
}

// watchCapacity reconciles the ingress ledger against live state. The interval
// is PASEO_RELAY_CAPACITY_MUTATION_TIMEOUT_MS: the longest an inconsistency may
// persist before admission is closed and the ledger corrected.
func (r *Relay) watchCapacity() func() {
	interval := time.Duration(r.Config.CapacityMutationTimeoutMS) * time.Millisecond
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
				r.reconcileCapacity()
			case <-stop:
				return
			}
		}
	}()
	return func() { close(stop); <-done }
}

// reconcileCapacity releases ingress reservations that no live route or buffer
// accounts for. Without it a reservation orphaned by an unusual teardown is
// held until restart, shrinking the effective budget for good.
func (r *Relay) reconcileCapacity() {
	r.mu.Lock()
	accounted := r.ingressInFlight.Load()
	for _, session := range r.sessions {
		for _, bytes := range session.bufferBytes {
			accounted += bytes
		}
	}
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
	// controlSyncDelay is how long a client route may wait for its data socket
	// before the relay re-sends sync, and controlCloseDelay how long after that
	// re-send the control socket has to produce one.
	controlSyncDelay  = 10 * time.Second
	controlCloseDelay = 5 * time.Second
)

// heapInUse reports the memory the Go runtime currently holds from the OS. It
// reads runtime/metrics rather than runtime.ReadMemStats because the sampler
// runs continuously and ReadMemStats stops the world.
func heapInUse() uint64 {
	samples := []metrics.Sample{
		{Name: "/memory/classes/total:bytes"},
		{Name: "/memory/classes/heap/released:bytes"},
	}
	metrics.Read(samples)
	total, released := samples[0].Value.Uint64(), samples[1].Value.Uint64()
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
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(memoryPressureInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				r.sampleMemoryPressure(heapInUse())
			case <-stop:
				return
			}
		}
	}()
	return func() { close(stop); <-done }
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

// shedForMemoryPressure drops every buffered frame and closes up to batch
// attached peers, then returns the freed memory to the runtime so the next
// sample can observe the relief.
func (r *Relay) shedForMemoryPressure(batch int) {
	r.mu.Lock()
	peers := make([]*relayPeer, 0, batch)
	released := int64(0)
	for _, session := range r.sessions {
		if len(peers) < batch {
			for _, peer := range sessionPeers(session) {
				// Peers already shed stay attached until their read loop unwinds;
				// skipping them keeps a batch a batch of distinct sockets.
				if len(peers) < batch && !peer.shed {
					peer.shed = true
					peers = append(peers, peer)
				}
			}
		}
		for connectionID := range session.buffer {
			released += session.dropBufferLocked(connectionID)
		}
	}
	r.ingressReserved.Add(-released)
	r.mu.Unlock()

	for _, peer := range peers {
		closeAsync(peer.conn, websocket.StatusTryAgainLater, "Relay memory pressure")
	}
	r.memoryPressureDisconnects.Add(int64(len(peers)))
	// Without this the shed memory stays uncollected and pressure never clears.
	runtime.GC()
}

// watchOwnership starts the cluster reconciler and returns a function that
// stops it and waits for it to exit. Backends without a cluster identity have
// nothing to reconcile, so the returned stop is a no-op.
func (r *Relay) watchOwnership() func() {
	if r.ownership.identity() == "" {
		return func() {}
	}
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(clusterHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				r.closeLostSessions()
			case <-stop:
				return
			}
		}
	}()
	return func() { close(stop); <-done }
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
		r.mu.Unlock()
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
	return reserveCounter(&r.activeWebSockets, 1, int64(r.Config.Acceptors*r.Config.ConnectionsPerAcceptor))
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

const metricsGaugeFormat = `# HELP paseo_relay_ready Whether this node admits new relay work.
# TYPE paseo_relay_ready gauge
paseo_relay_ready %d
# HELP paseo_relay_draining Whether this node is draining.
# TYPE paseo_relay_draining gauge
paseo_relay_draining %d
# HELP paseo_relay_active_websockets Active WebSocket connections.
# TYPE paseo_relay_active_websockets gauge
paseo_relay_active_websockets %d
paseo_relay_active_sessions %d
paseo_relay_reroute_responses_total %d
paseo_relay_connection_rejections_total %d
paseo_relay_frames_forwarded_total %d
paseo_relay_bytes_forwarded_total %d
paseo_relay_ingress_reserved_bytes %d
paseo_relay_inflight_delivery_bytes %d
paseo_relay_backpressured_sources %d
paseo_relay_slow_consumer_disconnects_total %d
paseo_relay_delivery_timeouts_total %d
paseo_relay_memory_pressure_disconnects_total %d
paseo_relay_max_frame_bytes %d
paseo_relay_beam_total_memory_bytes %d
paseo_relay_beam_process_memory_bytes %d
paseo_relay_beam_binary_memory_bytes %d
paseo_relay_beam_ets_memory_bytes %d
`

func (r *Relay) renderHandshakeMetrics(writer io.Writer) {
	outcomes := []string{"accepted", "rejected"}
	types := []string{"hello", "e2ee_hello"}
	for outcome, label := range outcomes {
		for version := 0; version < 2; version++ {
			for kind, handshake := range types {
				_, _ = fmt.Fprintf(writer, "paseo_relay_handshake_%s_total{routing_version=\"v%d\",type=\"%s\"} %d\n", label, version+1, handshake, r.handshakes[outcome][version][kind].Load())
			}
		}
	}
}

func (r *Relay) observeFrame(size int) {
	r.frameCount.Add(1)
	r.frameBytes.Add(int64(size))
	for {
		current := r.maxFrameBytes.Load()
		if int64(size) <= current || r.maxFrameBytes.CompareAndSwap(current, int64(size)) {
			return
		}
	}
}

func (r *Relay) renderHistograms(writer io.Writer) {
	waitCount := r.deliveryWaitCount.Load()
	waitSeconds := float64(r.deliveryWaitNanos.Load()) / float64(time.Second)
	_, _ = fmt.Fprintf(writer, "# TYPE paseo_relay_delivery_wait_seconds histogram\npaseo_relay_delivery_wait_seconds_bucket{le=\"+Inf\"} %d\npaseo_relay_delivery_wait_seconds_sum %g\npaseo_relay_delivery_wait_seconds_count %d\n", waitCount, waitSeconds, waitCount)
	frameCount := r.frameCount.Load()
	_, _ = fmt.Fprintf(writer, "# TYPE paseo_relay_frame_size_bytes histogram\npaseo_relay_frame_size_bytes_bucket{le=\"+Inf\"} %d\npaseo_relay_frame_size_bytes_sum %d\npaseo_relay_frame_size_bytes_count %d\n", frameCount, r.frameBytes.Load(), frameCount)
}
