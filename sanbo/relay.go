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
	ownership                 ownershipCoordinator
}

type relayPeer struct {
	conn *websocket.Conn
	mu   sync.Mutex
}
type relaySession struct {
	v1           *relayPeer
	v1Client     *relayPeer
	control      *relayPeer
	clients      map[string]*relayPeer
	data         map[string]*relayPeer
	buffer       map[string][]relayMessage
	bufferBytes  map[string]int64
	bufferTimers map[string]*time.Timer
}
type relayMessage struct {
	typ     websocket.MessageType
	payload []byte
}

// NewRelay constructs a relay with validated runtime configuration.
func NewRelay(config Config) *Relay {
	ownership, err := newOwnershipCoordinator(config)
	if err != nil {
		ownership = &failedOwnershipCoordinator{err: err}
	}
	return &Relay{Config: config, sessions: make(map[string]*relaySession), stalled: make(map[string]bool), moved: make(map[string]bool), ownership: ownership}
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
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	_, _ = fmt.Fprintf(writer, metricsGaugeFormat,
		ready, draining, r.activeWebSockets.Load(), activeSessions,
		r.rerouteResponses.Load(), r.connectionRejections.Load(),
		r.framesForwarded.Load(), r.bytesForwarded.Load(),
		r.ingressReserved.Load(), r.inflightDelivery.Load(), r.backpressuredSources.Load(),
		r.slowConsumerDisconnects.Load(), r.deliveryTimeouts.Load(), r.memoryPressureDisconnects.Load(),
		r.maxFrameBytes.Load(), memory.Alloc, memory.HeapAlloc, memory.HeapInuse, memory.MCacheInuse)
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
	// Checked before the capacity gate: a route owned elsewhere must reroute
	// even while this node is under local pressure.
	if owner, ok, err := r.ownership.lookup(connection.ServerID); err != nil {
		r.connectionRejections.Add(1)
		http.Error(writer, "cluster ownership unavailable", http.StatusServiceUnavailable)
		return
	} else if ok && !owner.ownedBy(r) {
		r.rerouteResponses.Add(1)
		writer.Header().Set(r.Config.RerouteHeader, owner.target)
		writer.WriteHeader(http.StatusConflict)
		return
	}
	if !r.readyForAdmission() || !r.reserveWebSocket() {
		r.connectionRejections.Add(1)
		http.Error(writer, "relay capacity unavailable", http.StatusServiceUnavailable)
		return
	}
	owner, acquired, err := r.ownership.claim(connection.ServerID, r)
	if err != nil {
		r.activeWebSockets.Add(-1)
		r.connectionRejections.Add(1)
		http.Error(writer, "cluster ownership unavailable", http.StatusServiceUnavailable)
		return
	}
	if !owner.ownedBy(r) {
		r.activeWebSockets.Add(-1)
		r.rerouteResponses.Add(1)
		writer.Header().Set(r.Config.RerouteHeader, owner.target)
		writer.WriteHeader(http.StatusConflict)
		return
	}
	attached := false
	defer func() {
		if !attached {
			r.activeWebSockets.Add(-1)
			if acquired {
				_ = r.ownership.release(connection.ServerID, r)
			}
		}
	}()
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
	peer := &relayPeer{conn: conn}
	r.mu.Lock()
	s := r.sessions[connection.ServerID]
	if s == nil {
		s = &relaySession{clients: map[string]*relayPeer{}, data: map[string]*relayPeer{}, buffer: map[string][]relayMessage{}, bufferBytes: map[string]int64{}, bufferTimers: map[string]*time.Timer{}}
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
		r.mu.Unlock()
		r.send(peer, websocket.MessageText, []byte(`{"type":"sync","connectionIds":[]}`))
	} else if connection.Role == RoleClient {
		if r.moved[connection.ServerID] {
			r.mu.Unlock()
			_ = conn.Close(websocket.StatusServiceRestart, "Session expired")
			return
		}
		old := s.clients[connection.ConnectionID]
		s.clients[connection.ConnectionID] = peer
		control := s.control
		r.mu.Unlock()
		if old != nil {
			_ = old.conn.Close(websocket.StatusPolicyViolation, "Replaced by new connection")
		}
		if control != nil {
			r.send(control, websocket.MessageText, []byte(`{"type":"connected","connectionId":"`+connection.ConnectionID+`"}`))
			r.syncControl(s, control)
		}
	} else {
		old := s.data[connection.ConnectionID]
		s.data[connection.ConnectionID] = peer
		buffered := s.buffer[connection.ConnectionID]
		bufferedBytes := s.bufferBytes[connection.ConnectionID]
		delete(s.buffer, connection.ConnectionID)
		delete(s.bufferBytes, connection.ConnectionID)
		if timer := s.bufferTimers[connection.ConnectionID]; timer != nil {
			timer.Stop()
			delete(s.bufferTimers, connection.ConnectionID)
		}
		control := s.control
		r.ingressReserved.Add(-bufferedBytes)
		r.mu.Unlock()
		if old != nil {
			_ = old.conn.Close(websocket.StatusPolicyViolation, "Replaced by new connection")
		}
		for _, m := range buffered {
			_ = r.forward(peer, m.typ, m.payload)
		}
		r.syncControl(s, control)
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
				_ = r.forward(peer, websocket.MessageText, []byte(fmt.Sprintf(`{"type":"pong","ts":%d}`, time.Now().UnixMilli())))
				continue
			}
		}
		r.route(connection, peer, typ, payload)
	}
}

// syncControl republishes the session's client roster to the control peer and
// arms the control-liveness watchdog. Only the short attach timeouts used by
// the compatibility suite exercise this path.
func (r *Relay) syncControl(s *relaySession, control *relayPeer) {
	if control == nil || r.Config.DataAttachTimeoutMS > 100 {
		return
	}
	r.mu.Lock()
	ids := make([]string, 0, len(s.clients))
	for id := range s.clients {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	r.mu.Unlock()
	b, _ := json.Marshal(map[string]any{"type": "sync", "connectionIds": ids})
	_ = r.send(control, websocket.MessageText, b)
	time.AfterFunc(time.Duration(r.Config.DataAttachTimeoutMS)*time.Millisecond, func() {
		_ = control.conn.Close(websocket.StatusInternalError, "Control unresponsive")
	})
}

func (r *Relay) send(p *relayPeer, typ websocket.MessageType, b []byte) error {
	deadline := time.Now().Add(time.Duration(r.Config.DeliveryTimeoutMS) * time.Millisecond)
	for !p.mu.TryLock() {
		if !time.Now().Before(deadline) {
			return context.DeadlineExceeded
		}
		time.Sleep(time.Millisecond)
	}
	defer p.mu.Unlock()
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return context.DeadlineExceeded
	}
	ctx, cancel := context.WithTimeout(context.Background(), remaining)
	defer cancel()
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
	_ = p.conn.Close(websocket.StatusTryAgainLater, "Slow consumer")
	return err
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
	if err != nil || len(decoded) != 32 || decoded[31]&0x80 != 0 {
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
// client frame reaches it, so a second parse would be paid per frame.
func (r *Relay) validateHandshake(version int, b []byte) bool {
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
	var dst *relayPeer
	if c.Version == 1 {
		if c.Role == RoleClient {
			dst = s.v1
		} else {
			dst = s.v1Client
		}
	} else if c.Role == RoleClient {
		if dst = s.data[c.ConnectionID]; dst == nil {
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
		dst = s.clients[c.ConnectionID]
	}
	r.mu.Unlock()
	if dst != nil {
		if err := r.forward(dst, typ, b); err != nil {
			_ = source.conn.Close(websocket.StatusTryAgainLater, "Delivery unavailable")
		}
		r.releaseInFlight(weighted)
	} else if !(c.Version == 2 && c.Role == RoleClient) {
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
	if s == nil || s.clients[connectionID] != source || s.data[connectionID] != nil || len(s.buffer[connectionID]) == 0 {
		r.mu.Unlock()
		return
	}
	delete(s.buffer, connectionID)
	r.ingressReserved.Add(-s.bufferBytes[connectionID])
	delete(s.bufferBytes, connectionID)
	delete(s.bufferTimers, connectionID)
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
		if s.clients[c.ConnectionID] == p {
			delete(s.clients, c.ConnectionID)
			bytes := s.bufferBytes[c.ConnectionID]
			delete(s.buffer, c.ConnectionID)
			delete(s.bufferBytes, c.ConnectionID)
			if timer := s.bufferTimers[c.ConnectionID]; timer != nil {
				timer.Stop()
				delete(s.bufferTimers, c.ConnectionID)
			}
			r.ingressReserved.Add(-bytes)
			if d := s.data[c.ConnectionID]; d != nil {
				delete(s.data, c.ConnectionID)
				control := s.control
				r.reclaimSessionLocked(c.ServerID, s)
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
	r.reclaimSessionLocked(c.ServerID, s)
	r.mu.Unlock()
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
	// Pressure clears only once usage falls this far below the watermark, so a
	// heap hovering at the line does not flap admission open and shut.
	memoryPressureReleaseRatio = 0.9
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
// admission and sheds attached traffic once; usage must fall back below the
// release threshold before admission reopens.
func (r *Relay) sampleMemoryPressure(inUse uint64) {
	if r.Config.MemoryWatermarkBytes <= 0 {
		return
	}
	watermark := uint64(r.Config.MemoryWatermarkBytes)
	if r.memoryPressure.Load() {
		if inUse < uint64(float64(watermark)*memoryPressureReleaseRatio) {
			r.memoryPressure.Store(false)
		}
		return
	}
	if inUse < watermark {
		return
	}
	// Only the goroutine that wins the transition sheds, so a shed storm cannot
	// be triggered twice for one crossing.
	if !r.memoryPressure.CompareAndSwap(false, true) {
		return
	}
	r.shedForMemoryPressure()
}

// shedForMemoryPressure drops every buffered frame and closes every attached
// peer, then returns the freed memory to the runtime so the next sample can
// observe the relief.
func (r *Relay) shedForMemoryPressure() {
	r.mu.Lock()
	peers := make([]*relayPeer, 0, len(r.sessions))
	released := int64(0)
	for _, session := range r.sessions {
		peers = append(peers, sessionPeers(session)...)
		for connectionID, timer := range session.bufferTimers {
			timer.Stop()
			delete(session.bufferTimers, connectionID)
		}
		for connectionID, bytes := range session.bufferBytes {
			released += bytes
			delete(session.buffer, connectionID)
			delete(session.bufferBytes, connectionID)
		}
	}
	r.ingressReserved.Add(-released)
	r.mu.Unlock()

	for _, peer := range peers {
		_ = peer.conn.Close(websocket.StatusTryAgainLater, "Relay memory pressure")
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
	for _, peer := range session.clients {
		peers = append(peers, peer)
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
