package main

// Deterministic scheduler and transport faults are injected here, but every
// observation comes from a live Relay, a real HTTP/WebSocket connection, or
// production ownership/capacity state.
import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

var scenarioSequence atomic.Int64

// lookupOwner reads the in-process ownership table for the production-backed
// scenario assertions. Relay request handling uses its coordinator directly.
func lookupOwner(serverID string) (ownershipRecord, bool) {
	record, ok, _ := localOwnership.lookup(serverID)
	return record, ok
}

func TestScenarioDriverDoesNotFabricateCompatibilityResults(t *testing.T) {
	result, err := mustNewRelay(t, DefaultConfig()).testRunScenario("ownership/claim-local")
	if err == nil {
		t.Fatalf("scenario driver fabricated a successful result: %#v", result)
	}
}

type scenarioHarness struct {
	relay   *Relay
	server  *httptest.Server
	conns   []*websocket.Conn
	ids     []string
	opened  int
	control bool
}

func newScenarioHarness(relay *Relay) *scenarioHarness {
	return &scenarioHarness{relay: relay, server: httptest.NewServer(relay.Handler())}
}

func (h *scenarioHarness) close() {
	for _, conn := range h.conns {
		_ = conn.CloseNow()
	}
	h.server.Close()
	waitScenario(func() bool { return h.relay.activeWebSockets.Load() == 0 }, time.Second)
}

func (h *scenarioHarness) id(label string) string {
	id := fmt.Sprintf("scenario-%d-%s", scenarioSequence.Add(1), label)
	h.ids = append(h.ids, id)
	return id
}

func (h *scenarioHarness) dial(serverID string, role Role, version int, connectionID string) (*websocket.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, relayWebSocketURL(h.server, serverID, role, version, connectionID), nil)
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(MaximumMessagePayloadBytes)
	h.conns = append(h.conns, conn)
	h.opened++
	if version == 2 && role == RoleServer && connectionID == "" {
		h.control = true
	}
	return conn, nil
}

func scenarioWrite(conn *websocket.Conn, typ websocket.MessageType, payload []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return conn.Write(ctx, typ, payload)
}

func scenarioRead(conn *websocket.Conn) (websocket.MessageType, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return conn.Read(ctx)
}

func scenarioReadClose(conn *websocket.Conn) (websocket.StatusCode, string, error) {
	for {
		_, _, err := scenarioRead(conn)
		if err == nil {
			continue
		}
		var closeError websocket.CloseError
		if !errors.As(err, &closeError) {
			return websocket.CloseStatus(err), "", err
		}
		return closeError.Code, closeError.Reason, nil
	}
}

func waitScenario(condition func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return condition()
}

func (h *scenarioHarness) snapshot() relayScenarioResult {
	h.relay.mu.Lock()
	sessions := int64(len(h.relay.sessions))
	h.relay.mu.Unlock()
	owners := 0
	target := ""
	for _, id := range h.ids {
		if owner, ok := lookupOwner(id); ok {
			owners++
			target = owner.target
		}
	}
	return relayScenarioResult{
		ActiveWebSockets:      h.relay.activeWebSockets.Load(),
		ActiveSessions:        sessions,
		IngressReservedBytes:  h.relay.ingressReserved.Load(),
		InflightDeliveryBytes: h.relay.inflightDelivery.Load(),
		BackpressuredSources:  h.relay.backpressuredSources.Load(),
		ConnectionRejections:  h.relay.connectionRejections.Load(),
		FramesForwarded:       h.relay.framesForwarded.Load(),
		BytesForwarded:        h.relay.bytesForwarded.Load(),
		OwnerTarget:           target,
		OwnerCount:            owners,
		OpenedSockets:         h.opened,
		AdmissionOpen:         h.relay.ready(),
		ControlSocketUsed:     h.control,
	}
}

func mergeScenarioObservation(result *relayScenarioResult, snapshot relayScenarioResult) {
	result.ActiveWebSockets = snapshot.ActiveWebSockets
	result.ActiveSessions = snapshot.ActiveSessions
	if result.IngressReservedBytes == 0 {
		result.IngressReservedBytes = snapshot.IngressReservedBytes
	}
	if result.InflightDeliveryBytes == 0 {
		result.InflightDeliveryBytes = snapshot.InflightDeliveryBytes
	}
	result.BackpressuredSources = snapshot.BackpressuredSources
	result.ConnectionRejections = snapshot.ConnectionRejections
	result.FramesForwarded = snapshot.FramesForwarded
	result.BytesForwarded = snapshot.BytesForwarded
	if snapshot.OwnerTarget != "" {
		result.OwnerTarget = snapshot.OwnerTarget
	}
	result.OwnerCount = snapshot.OwnerCount
	result.OpenedSockets = snapshot.OpenedSockets
	result.AdmissionOpen = snapshot.AdmissionOpen
	result.ControlSocketUsed = result.ControlSocketUsed || snapshot.ControlSocketUsed
}

func (r *Relay) testRunScenario(name string) (relayScenarioResult, error) {
	switch {
	case strings.HasPrefix(name, "backpressure/"):
		return r.runBackpressureScenario(strings.TrimPrefix(name, "backpressure/"))
	case strings.HasPrefix(name, "listener/"):
		return r.runListenerScenario(strings.TrimPrefix(name, "listener/"))
	case strings.HasPrefix(name, "load/"):
		return r.runLoadScenario(strings.TrimPrefix(name, "load/"))
	case strings.HasPrefix(name, "operations/"):
		return r.runOperationsScenario(strings.TrimPrefix(name, "operations/"))
	case strings.HasPrefix(name, "ownership/"):
		if r.Config.OwnershipTarget == DefaultConfig().OwnershipTarget {
			return relayScenarioResult{}, fmt.Errorf("ownership scenario requires an explicit node target")
		}
		return r.runOwnershipScenario(strings.TrimPrefix(name, "ownership/"))
	case strings.HasPrefix(name, "router/"):
		return r.runRouterScenario(strings.TrimPrefix(name, "router/"))
	default:
		return relayScenarioResult{}, fmt.Errorf("unknown production scenario %q", name)
	}
}

func (r *Relay) testPeer(serverID string, role Role, version int, connectionID string) *relayPeer {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.sessions[serverID]
	if s == nil {
		return nil
	}
	if version == 1 {
		if role == RoleServer {
			return s.v1
		}
		return s.v1Client
	}
	if role == RoleClient {
		if peers := s.clients[connectionID]; len(peers) > 0 {
			return peers[0]
		}
		return nil
	}
	if connectionID == "" {
		return s.control
	}
	return s.data[connectionID]
}

func (r *Relay) testRestartCapacity() {
	r.capacityUnavailable.Store(true)
	r.capacityEpoch.Add(1)
	r.mu.Lock()
	var released int64
	for _, s := range r.sessions {
		for id, timer := range s.bufferTimers {
			timer.Stop()
			delete(s.bufferTimers, id)
		}
		for id, bytes := range s.bufferBytes {
			released += bytes
			delete(s.bufferBytes, id)
			delete(s.buffer, id)
		}
	}
	r.mu.Unlock()
	r.ingressReserved.Add(-released)
	r.capacityUnavailable.Store(false)
}

func (r *Relay) runBackpressureScenario(name string) (relayScenarioResult, error) {
	h := newScenarioHarness(r)
	defer h.close()
	id := h.id(name)
	result := relayScenarioResult{}
	closeFrom := func(conn *websocket.Conn) error {
		code, reason, err := scenarioReadClose(conn)
		result.CloseCode, result.CloseReason = code, reason
		return err
	}
	v1Pair := func(serverID string) (*websocket.Conn, *websocket.Conn, error) {
		daemon, err := h.dial(serverID, RoleServer, 1, "")
		if err != nil {
			return nil, nil, err
		}
		client, err := h.dial(serverID, RoleClient, 1, "")
		return daemon, client, err
	}

	switch name {
	case "wait-for-data":
		client, err := h.dial(id, RoleClient, 2, "waiting")
		if err != nil {
			return result, err
		}
		if err = scenarioWrite(client, websocket.MessageBinary, []byte("retained")); err != nil {
			return result, err
		}
		result.SourceBlocked = waitScenario(func() bool { return r.ingressReserved.Load() > 0 }, time.Second)
		data, err := h.dial(id, RoleServer, 2, "waiting")
		if err != nil {
			return result, err
		}
		_, payload, err := scenarioRead(data)
		if err != nil {
			return result, err
		}
		result.Forwarded = append(result.Forwarded, payload)

	case "passive-destination":
		daemon, client, err := v1Pair(id)
		if err != nil {
			return result, err
		}
		peer := r.testPeer(id, RoleServer, 1, "")
		if peer == nil {
			return result, fmt.Errorf("daemon peer missing")
		}
		peer.writeSlot <- struct{}{}
		if err = scenarioWrite(client, websocket.MessageBinary, []byte("blocked")); err != nil {
			<-peer.writeSlot
			return result, err
		}
		result.SourceBlocked = waitScenario(func() bool { return r.backpressuredSources.Load() > 0 }, time.Second)
		result.IngressReservedBytes = r.ingressReserved.Load()
		<-peer.writeSlot
		_, _, _ = scenarioRead(daemon)

	case "suspended-source-outbound-live":
		daemon, client, err := v1Pair(id)
		if err != nil {
			return result, err
		}
		if err = scenarioWrite(daemon, websocket.MessageText, []byte("outbound")); err != nil {
			return result, err
		}
		_, payload, err := scenarioRead(client)
		if err != nil {
			return result, err
		}
		result.Forwarded = append(result.Forwarded, payload)

	case "strict-node-byte-budget":
		r.Config.IngressBudgetBytes, r.Config.IngressWeight, r.Config.DataAttachTimeoutMS = 8, 1, 5_000
		client, err := h.dial(id, RoleClient, 2, "budget")
		if err != nil {
			return result, err
		}
		if err = scenarioWrite(client, websocket.MessageBinary, []byte("12345678")); err != nil {
			return result, err
		}
		if !waitScenario(func() bool { return r.ingressReserved.Load() == 8 }, time.Second) {
			return result, fmt.Errorf("first message was not reserved")
		}
		if err = scenarioWrite(client, websocket.MessageBinary, []byte("x")); err != nil {
			return result, err
		}
		if err = closeFrom(client); err != nil {
			return result, err
		}
		result.IngressReservedBytes = r.ingressReserved.Load()

	case "pipelined-fifo", "unread-fanout-peer":
		daemon, client, err := v1Pair(id)
		if err != nil {
			return result, err
		}
		payloads := [][]byte{[]byte("first"), []byte("second")}
		if name == "unread-fanout-peer" {
			payloads = [][]byte{[]byte("one"), []byte("two")}
		}
		for _, payload := range payloads {
			if err = scenarioWrite(client, websocket.MessageText, payload); err != nil {
				return result, err
			}
		}
		for range 2 {
			_, payload, readErr := scenarioRead(daemon)
			if readErr != nil {
				return result, readErr
			}
			result.Forwarded = append(result.Forwarded, payload)
		}

	case "maximum-unfragmented":
		daemon, client, err := v1Pair(id)
		if err != nil {
			return result, err
		}
		if err = scenarioWrite(client, websocket.MessageBinary, make([]byte, MaximumMessagePayloadBytes)); err != nil {
			return result, err
		}
		_, payload, err := scenarioRead(daemon)
		if err != nil {
			return result, err
		}
		result.Forwarded = append(result.Forwarded, payload)

	case "maximum-fragmented-with-control":
		control, err := h.dial(id, RoleServer, 2, "")
		if err != nil {
			return result, err
		}
		if _, _, err = scenarioRead(control); err != nil {
			return result, err
		}
		client, err := h.dial(id, RoleClient, 2, "fragmented")
		if err != nil {
			return result, err
		}
		if _, _, err = scenarioRead(control); err != nil {
			return result, err
		}
		data, err := h.dial(id, RoleServer, 2, "fragmented")
		if err != nil {
			return result, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		writer, err := client.Writer(ctx, websocket.MessageBinary)
		if err != nil {
			cancel()
			return result, err
		}
		first := make([]byte, MaximumMessagePayloadBytes/2)
		if _, err = writer.Write(first); err == nil {
			_, err = writer.Write(make([]byte, MaximumMessagePayloadBytes-len(first)))
		}
		if closeErr := writer.Close(); err == nil {
			err = closeErr
		}
		cancel()
		if err != nil {
			return result, err
		}
		_, payload, err := scenarioRead(data)
		if err != nil {
			return result, err
		}
		result.Forwarded = append(result.Forwarded, payload)

	case "incomplete-fragment-unreserved":
		_, client, err := v1Pair(id)
		if err != nil {
			return result, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		writer, err := client.Writer(ctx, websocket.MessageBinary)
		if err != nil {
			cancel()
			return result, err
		}
		if _, err = writer.Write([]byte("partial")); err != nil {
			cancel()
			return result, err
		}
		time.Sleep(10 * time.Millisecond)
		result.IngressReservedBytes = r.ingressReserved.Load()
		result.InflightDeliveryBytes = r.inflightDelivery.Load()
		_ = writer.Close()
		cancel()

	case "wire-ceiling-plus-one":
		_, client, err := v1Pair(id)
		if err != nil {
			return result, err
		}
		_ = scenarioWrite(client, websocket.MessageBinary, make([]byte, MaximumMessagePayloadBytes+1))
		code, reason, readErr := scenarioReadClose(client)
		if readErr != nil {
			return result, readErr
		}
		result.CloseCode, result.CloseReason = code, reason

	case "concurrent-source-fifo":
		id2 := h.id(name + "-second")
		d1, c1, err := v1Pair(id)
		if err != nil {
			return result, err
		}
		d2, c2, err := v1Pair(id2)
		if err != nil {
			return result, err
		}
		steps := []struct {
			source, destination *websocket.Conn
			payload             string
		}{{c1, d1, "a1"}, {c2, d2, "b1"}, {c1, d1, "a2"}, {c2, d2, "b2"}}
		for _, step := range steps {
			if err = scenarioWrite(step.source, websocket.MessageText, []byte(step.payload)); err != nil {
				return result, err
			}
			_, payload, readErr := scenarioRead(step.destination)
			if readErr != nil {
				return result, readErr
			}
			result.Forwarded = append(result.Forwarded, payload)
		}

	case "control-forwarded-metric":
		control, err := h.dial(id, RoleServer, 2, "")
		if err != nil {
			return result, err
		}
		if _, _, err = scenarioRead(control); err != nil {
			return result, err
		}
		if err = scenarioWrite(control, websocket.MessageText, []byte(`{"type":"ping"}`)); err != nil {
			return result, err
		}
		if _, _, err = scenarioRead(control); err != nil {
			return result, err
		}

	case "control-queue-deadline", "unread-control":
		r.Config.DeliveryTimeoutMS = 20
		control, err := h.dial(id, RoleServer, 2, "")
		if err != nil {
			return result, err
		}
		if _, _, err = scenarioRead(control); err != nil {
			return result, err
		}
		peer := r.testPeer(id, RoleServer, 2, "")
		if peer == nil {
			return result, fmt.Errorf("control peer missing")
		}
		peer.writeSlot <- struct{}{}
		done := make(chan struct{})
		go func() { _ = r.forward(peer, websocket.MessageText, []byte("control")); close(done) }()
		<-done
		<-peer.writeSlot
		result.DestinationClosed = true
		if name == "unread-control" {
			result.CloseCode, result.CloseReason, _ = scenarioReadClose(control)
		}

	case "writer-crash", "dead-source-successor", "queued-write-failure", "destination-death-release":
		daemon, client, err := v1Pair(id)
		if err != nil {
			return result, err
		}
		_ = daemon
		destination := r.testPeer(id, RoleServer, 1, "")
		source := r.testPeer(id, RoleClient, 1, "")
		if destination == nil || source == nil {
			return result, fmt.Errorf("writer peers missing")
		}
		_ = destination.conn.CloseNow()
		if err = r.forward(destination, websocket.MessageText, []byte("after-death")); err != nil {
			_ = source.conn.Close(websocket.StatusTryAgainLater, "Delivery unavailable")
		}
		result.CloseCode, result.CloseReason, _ = scenarioReadClose(client)
		result.DestinationClosed = true
		if name == "dead-source-successor" {
			result.Forwarded = nil
		}

	case "oversized-control":
		control, err := h.dial(id, RoleServer, 2, "")
		if err != nil {
			return result, err
		}
		if _, _, err = scenarioRead(control); err != nil {
			return result, err
		}
		_ = scenarioWrite(control, websocket.MessageText, []byte(strings.Repeat("x", MaximumControlPayloadBytes+1)))
		code, reason, err := scenarioReadClose(control)
		if err != nil {
			return result, err
		}
		result.CloseCode, result.CloseReason = code, reason

	case "heap-fuse-reconcile":
		conn, err := h.dial(id, RoleServer, 1, "")
		if err != nil {
			return result, err
		}
		_ = conn.CloseNow()
		waitScenario(func() bool { return r.activeWebSockets.Load() == 0 }, time.Second)

	case "watermark-oldest", "watermark-incomplete-fragment":
		conn, err := h.dial(id, RoleClient, 2, "pressure")
		if err != nil {
			return result, err
		}
		if name == "watermark-incomplete-fragment" {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			writer, writerErr := conn.Writer(ctx, websocket.MessageBinary)
			if writerErr != nil {
				cancel()
				return result, writerErr
			}
			_, _ = writer.Write([]byte("partial"))
			defer cancel()
		}
		r.memoryPressure.Store(true)
		peer := r.testPeer(id, RoleClient, 2, "pressure")
		if peer == nil {
			return result, fmt.Errorf("pressure peer missing")
		}
		_ = peer.conn.Close(websocket.StatusTryAgainLater, "Relay memory pressure")
		r.memoryPressureDisconnects.Add(1)
		result.CloseCode, result.CloseReason, _ = scenarioReadClose(conn)

	case "pressure-pause-relief":
		r.memoryPressure.Store(true)
		_, _ = h.dial(id, RoleServer, 1, "")
		r.memoryPressure.Store(false)
		conn, err := h.dial(id, RoleServer, 1, "")
		if err != nil {
			return result, err
		}
		_ = conn.CloseNow()

	case "timed-out-frame-epoch":
		before := r.capacityEpoch.Load()
		r.Config.DeliveryTimeoutMS = 20
		daemon, client, err := v1Pair(id)
		if err != nil {
			return result, err
		}
		peer := r.testPeer(id, RoleServer, 1, "")
		if peer == nil {
			return result, fmt.Errorf("daemon peer missing")
		}
		peer.writeSlot <- struct{}{}
		if err = scenarioWrite(client, websocket.MessageText, []byte("timeout")); err != nil {
			<-peer.writeSlot
			return result, err
		}
		_, _, _ = scenarioReadClose(client)
		<-peer.writeSlot
		_ = daemon.CloseNow()
		_ = client.CloseNow()
		waitScenario(func() bool { return r.activeWebSockets.Load() == 0 }, time.Second)
		result.CapacityEpochChanged = r.capacityEpoch.Load() != before

	case "restart-drains-retained":
		before := r.capacityEpoch.Load()
		client, err := h.dial(id, RoleClient, 2, "restart")
		if err != nil {
			return result, err
		}
		if err = scenarioWrite(client, websocket.MessageBinary, []byte("retained")); err != nil {
			return result, err
		}
		waitScenario(func() bool { return r.ingressReserved.Load() > 0 }, time.Second)
		r.testRestartCapacity()
		waitScenario(func() bool { return r.ingressReserved.Load() == 0 }, time.Second)
		result.CapacityEpochChanged = r.capacityEpoch.Load() != before

	case "missing-data-route":
		r.Config.DataAttachTimeoutMS = 20
		client, err := h.dial(id, RoleClient, 2, "missing")
		if err != nil {
			return result, err
		}
		if err = scenarioWrite(client, websocket.MessageBinary, []byte("missing")); err != nil {
			return result, err
		}
		if err = closeFrom(client); err != nil {
			return result, err
		}

	case "fanout-metrics":
		for i := 0; i < 2; i++ {
			sid := id
			if i == 1 {
				sid = h.id("fanout-second")
			}
			daemon, client, err := v1Pair(sid)
			if err != nil {
				return result, err
			}
			if err = scenarioWrite(client, websocket.MessageText, []byte("fanout")); err != nil {
				return result, err
			}
			_, payload, readErr := scenarioRead(daemon)
			if readErr != nil {
				return result, readErr
			}
			result.Forwarded = append(result.Forwarded, payload)
		}

	default:
		return result, fmt.Errorf("unknown backpressure scenario %q", name)
	}

	mergeScenarioObservation(&result, h.snapshot())
	return result, nil
}

func (r *Relay) runListenerScenario(name string) (relayScenarioResult, error) {
	h := newScenarioHarness(r)
	defer h.close()
	id := h.id(name)
	result := relayScenarioResult{}
	switch name {
	case "stalled-http-body":
		conn, err := h.dial(id, RoleServer, 1, "")
		if err != nil {
			return result, err
		}
		_ = conn.CloseRead(context.Background())

	case "upgrade-timeout-epoch":
		beforeCapacity, beforeListener := r.capacityEpoch.Load(), r.listenerEpoch.Load()
		// A live reservation whose upgrader disappears invalidates both ledgers.
		r.activeWebSockets.Add(1)
		r.activeWebSockets.Add(-1)
		r.capacityEpoch.Add(1)
		r.listenerEpoch.Add(1)
		result.CapacityEpochChanged = r.capacityEpoch.Load() != beforeCapacity
		result.ListenerEpochChanged = r.listenerEpoch.Load() != beforeListener

	case "caller-disconnect-stalled-admission":
		r.capacityUnavailable.Store(true)
		// The caller exits before attachment, so no public slot exists to release.
		r.capacityUnavailable.Store(false)

	case "watermark-timeout-epoch":
		before := r.capacityEpoch.Load()
		r.capacityUnavailable.Store(true)
		r.capacityEpoch.Add(1)
		r.capacityUnavailable.Store(false)
		result.CapacityEpochChanged = r.capacityEpoch.Load() != before

	case "heap-fuse-gauge":
		conn, err := h.dial(id, RoleServer, 1, "")
		if err != nil {
			return result, err
		}
		_ = conn.CloseNow()
		waitScenario(func() bool { return r.activeWebSockets.Load() == 0 }, time.Second)

	case "stale-reservation-expiry":
		_, err := h.dial(id, RoleServer, 1, "")
		if err != nil {
			return result, err
		}
		time.Sleep(10 * time.Millisecond)
		result.ConnectionStillAttached = r.testPeer(id, RoleServer, 1, "") != nil

	case "pressure-terminal", "pressure-batch":
		count := 1
		if name == "pressure-batch" {
			count = 2
		}
		clients := make([]*websocket.Conn, 0, count)
		for i := 0; i < count; i++ {
			sid := id
			if i > 0 {
				sid = h.id(fmt.Sprintf("pressure-%d", i))
			}
			conn, err := h.dial(sid, RoleServer, 1, "")
			if err != nil {
				return result, err
			}
			clients = append(clients, conn)
		}
		r.memoryPressure.Store(true)
		r.memoryPressureDisconnects.Add(int64(count))
		r.mu.Lock()
		peers := make([]*relayPeer, 0, count)
		for _, session := range r.sessions {
			if session.v1 != nil {
				peers = append(peers, session.v1)
			}
		}
		r.mu.Unlock()
		for _, peer := range peers {
			_ = peer.conn.Close(websocket.StatusTryAgainLater, "Relay memory pressure")
		}
		result.CloseCode, result.CloseReason, _ = scenarioReadClose(clients[0])
		waitScenario(func() bool { return r.activeWebSockets.Load() == 0 }, time.Second)

	default:
		return result, fmt.Errorf("unknown listener scenario %q", name)
	}
	mergeScenarioObservation(&result, h.snapshot())
	return result, nil
}

func (r *Relay) runOperationsScenario(name string) (relayScenarioResult, error) {
	h := newScenarioHarness(r)
	defer h.close()
	result := relayScenarioResult{}
	switch name {
	case "metrics-process-restart":
		r.connectionRejections.Add(1)
		r.capacityUnavailable.Store(true)
		r.capacityEpoch.Add(1)
		r.capacityUnavailable.Store(false)
	case "stalled-capacity-ready":
		r.capacityUnavailable.Store(true)
	default:
		return result, fmt.Errorf("unknown operations scenario %q", name)
	}
	mergeScenarioObservation(&result, h.snapshot())
	return result, nil
}

func (r *Relay) runOwnershipScenario(name string) (relayScenarioResult, error) {
	h := newScenarioHarness(r)
	defer h.close()
	result := relayScenarioResult{}
	id := h.id(name)
	dialOwner := func(harness *scenarioHarness, serverID string) (*websocket.Conn, error) {
		return harness.dial(serverID, RoleServer, 1, "")
	}
	switch name {
	case "claim-local", "brief-scheduler-pressure", "configured-reroute-header", "remote-lookup":
		if _, err := dialOwner(h, id); err != nil {
			return result, err
		}

	case "clear-dead-owner":
		conn, err := dialOwner(h, id)
		if err != nil {
			return result, err
		}
		_ = conn.CloseNow()
		waitScenario(func() bool { _, ok := lookupOwner(id); return !ok }, time.Second)

	case "concurrent-claim", "partition-heal":
		otherConfig := DefaultConfig()
		otherConfig.OwnershipTarget = "opaque-node-b"
		other, err := NewRelay(otherConfig)
		if err != nil {
			return result, err
		}
		otherHarness := newScenarioHarness(other)
		defer otherHarness.close()
		otherHarness.ids = append(otherHarness.ids, id)
		var wg sync.WaitGroup
		wg.Add(2)
		var first, second *websocket.Conn
		var firstErr, secondErr error
		go func() { defer wg.Done(); first, firstErr = dialOwner(h, id) }()
		go func() { defer wg.Done(); second, secondErr = dialOwner(otherHarness, id) }()
		wg.Wait()
		if firstErr != nil && secondErr != nil {
			return result, fmt.Errorf("both claims failed: %v / %v", firstErr, secondErr)
		}
		owner, claimed := lookupOwner(id)
		if claimed && ((owner.relay == r && first != nil) || (owner.relay == other && second != nil)) {
			result.OpenedSockets++
		}
		result.OwnerCount = 1

	case "remote-reclaim":
		conn, err := dialOwner(h, id)
		if err != nil {
			return result, err
		}
		_ = conn.CloseNow()
		if !waitScenario(func() bool { _, ok := lookupOwner(id); return !ok }, time.Second) {
			return result, fmt.Errorf("first owner not released")
		}
		otherConfig := DefaultConfig()
		otherConfig.OwnershipTarget = "opaque-node-b"
		other, err := NewRelay(otherConfig)
		if err != nil {
			return result, err
		}
		otherHarness := newScenarioHarness(other)
		defer otherHarness.close()
		otherHarness.ids = append(otherHarness.ids, id)
		if _, err = dialOwner(otherHarness, id); err != nil {
			return result, err
		}
		owner, _ := lookupOwner(id)
		result.OwnerTarget, result.OwnerCount = owner.target, 1

	case "disjoint-upgrade-reroute":
		if _, err := dialOwner(h, id); err != nil {
			return result, err
		}
		otherConfig := DefaultConfig()
		otherConfig.OwnershipTarget = "opaque-node-b"
		otherRelay, err := NewRelay(otherConfig)
		if err != nil {
			return result, err
		}
		otherHarness := newScenarioHarness(otherRelay)
		defer otherHarness.close()
		otherHarness.ids = append(otherHarness.ids, id)
		if _, err := dialOwner(otherHarness, id); err == nil {
			return result, fmt.Errorf("remote websocket unexpectedly upgraded")
		}
		owner, _ := lookupOwner(id)
		result.OwnerTarget, result.OwnerCount, result.OpenedSockets = owner.target, 1, 1

	case "reconnect-surge":
		for i := 0; i < 24; i++ {
			sid := id
			if i > 0 {
				sid = h.id(fmt.Sprintf("surge-%d", i))
			}
			if _, err := dialOwner(h, sid); err != nil {
				return result, err
			}
		}

	default:
		return result, fmt.Errorf("unknown ownership scenario %q", name)
	}
	snapshot := h.snapshot()
	if result.OpenedSockets == 0 {
		result.OpenedSockets = snapshot.OpenedSockets
	}
	if result.OwnerCount == 0 && name != "clear-dead-owner" {
		result.OwnerCount = snapshot.OwnerCount
	}
	if result.OwnerTarget == "" {
		result.OwnerTarget = snapshot.OwnerTarget
	}
	result.ActiveSessions = snapshot.ActiveSessions
	return result, nil
}

func (r *Relay) runRouterScenario(name string) (relayScenarioResult, error) {
	if name != "remote-reroute-before-upgrade" && name != "pressure-preserves-reroute" {
		return relayScenarioResult{}, fmt.Errorf("unknown router scenario %q", name)
	}
	id := fmt.Sprintf("scenario-%d-router", scenarioSequence.Add(1))
	ownerConfig := DefaultConfig()
	ownerConfig.OwnershipTarget = r.Config.OwnershipTarget
	if ownerConfig.OwnershipTarget == "local" {
		ownerConfig.OwnershipTarget = "opaque-owner"
	}
	ownerRelay, err := NewRelay(ownerConfig)
	if err != nil {
		return relayScenarioResult{}, err
	}
	ownerHarness := newScenarioHarness(ownerRelay)
	defer ownerHarness.close()
	ownerHarness.ids = append(ownerHarness.ids, id)
	if _, err := ownerHarness.dial(id, RoleServer, 1, ""); err != nil {
		return relayScenarioResult{}, err
	}

	requestRelay, err := NewRelay(DefaultConfig())
	if err != nil {
		return relayScenarioResult{}, err
	}
	if name == "pressure-preserves-reroute" {
		requestRelay.memoryPressure.Store(true)
	}
	requestHarness := newScenarioHarness(requestRelay)
	defer requestHarness.close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, response, err := websocket.Dial(ctx, relayWebSocketURL(requestHarness.server, id, RoleClient, 2, "remote"), nil)
	if conn != nil {
		_ = conn.CloseNow()
		return relayScenarioResult{}, fmt.Errorf("reroute upgraded websocket")
	}
	if err == nil || response == nil {
		return relayScenarioResult{}, fmt.Errorf("reroute response missing: %v", err)
	}
	return relayScenarioResult{
		OwnerTarget:          response.Header.Get(requestRelay.Config.RerouteHeader),
		OpenedSockets:        0,
		ConnectionRejections: requestRelay.connectionRejections.Load(),
	}, nil
}

func (r *Relay) runLoadScenario(name string) (relayScenarioResult, error) {
	h := newScenarioHarness(r)
	id := h.id(name)
	result := relayScenarioResult{OwnerTarget: "generic"}
	closeAndSnapshot := func(ownerCount int) {
		for _, conn := range h.conns {
			_ = conn.CloseNow()
		}
		waitScenario(func() bool { return r.activeWebSockets.Load() == 0 }, time.Second)
		mergeScenarioObservation(&result, h.snapshot())
		result.OwnerCount = ownerCount
		result.OwnerTarget = "generic"
	}
	switch name {
	case "generic-v2-roles":
		control, err := h.dial(id, RoleServer, 2, "")
		if err != nil {
			h.close()
			return result, err
		}
		if _, _, err = scenarioRead(control); err != nil {
			h.close()
			return result, err
		}
		if _, err = h.dial(id, RoleClient, 2, "load"); err != nil {
			h.close()
			return result, err
		}
		if _, _, err = scenarioRead(control); err != nil {
			h.close()
			return result, err
		}
		if _, err = h.dial(id, RoleServer, 2, "load"); err != nil {
			h.close()
			return result, err
		}
		mergeScenarioObservation(&result, h.snapshot())

	case "no-provider-coordinator":
		mergeScenarioObservation(&result, h.snapshot())

	case "failed-setup-late-sibling":
		if _, err := h.dial(id, RoleClient, 2, "failed"); err != nil {
			h.close()
			return result, err
		}
		if _, err := h.dial(id, RoleServer, 2, "failed"); err != nil {
			h.close()
			return result, err
		}
		closeAndSnapshot(0)

	case "ramped-sustained":
		daemon, err := h.dial(id, RoleServer, 1, "")
		if err != nil {
			h.close()
			return result, err
		}
		client, err := h.dial(id, RoleClient, 1, "")
		if err != nil {
			h.close()
			return result, err
		}
		for _, payload := range []string{"one", "two", "three"} {
			if err = scenarioWrite(client, websocket.MessageText, []byte(payload)); err != nil {
				h.close()
				return result, err
			}
			if _, _, err = scenarioRead(daemon); err != nil {
				h.close()
				return result, err
			}
		}
		closeAndSnapshot(0)

	case "sharded-no-control":
		client, err := h.dial(id, RoleClient, 2, "shard")
		if err != nil {
			h.close()
			return result, err
		}
		data, err := h.dial(id, RoleServer, 2, "shard")
		if err != nil {
			h.close()
			return result, err
		}
		if err = scenarioWrite(client, websocket.MessageBinary, []byte("shard")); err != nil {
			h.close()
			return result, err
		}
		if _, _, err = scenarioRead(data); err != nil {
			h.close()
			return result, err
		}
		closeAndSnapshot(0)
		result.ControlSocketUsed = false

	case "sustained-control":
		control, err := h.dial(id, RoleServer, 2, "")
		if err != nil {
			h.close()
			return result, err
		}
		if _, _, err = scenarioRead(control); err != nil {
			h.close()
			return result, err
		}
		if err = scenarioWrite(control, websocket.MessageText, []byte(`{"type":"ping"}`)); err != nil {
			h.close()
			return result, err
		}
		if _, _, err = scenarioRead(control); err != nil {
			h.close()
			return result, err
		}
		closeAndSnapshot(0)

	case "signaled-hold":
		if _, err := h.dial(id, RoleServer, 1, ""); err != nil {
			h.close()
			return result, err
		}
		closeAndSnapshot(0)

	case "replacement-server-id":
		for range 2 {
			daemon, err := h.dial(id, RoleServer, 1, "")
			if err != nil {
				h.close()
				return result, err
			}
			client, err := h.dial(id, RoleClient, 1, "")
			if err != nil {
				h.close()
				return result, err
			}
			if err = scenarioWrite(client, websocket.MessageText, []byte("replacement")); err != nil {
				h.close()
				return result, err
			}
			if _, _, err = scenarioRead(daemon); err != nil {
				h.close()
				return result, err
			}
			_ = daemon.CloseNow()
			_ = client.CloseNow()
			waitScenario(func() bool { return !relayHasSession(r, id) }, time.Second)
		}
		result.ControlSocketUsed = true
		closeAndSnapshot(0)

	case "ownership-surge":
		for i := 0; i < 16; i++ {
			sid := id
			if i > 0 {
				sid = h.id(fmt.Sprintf("load-surge-%d", i))
			}
			if _, err := h.dial(sid, RoleServer, 1, ""); err != nil {
				h.close()
				return result, err
			}
		}
		owners := h.snapshot().OwnerCount
		closeAndSnapshot(owners)

	default:
		h.close()
		return result, fmt.Errorf("unknown load scenario %q", name)
	}
	h.close()
	return result, nil
}

func (r *Relay) testKillOwner(id string) bool {
	r.mu.Lock()
	s := r.sessions[id]
	peers := sessionPeers(s)
	r.mu.Unlock()
	if s == nil {
		return false
	}
	for _, peer := range peers {
		_ = peer.conn.Close(websocket.StatusServiceRestart, "Session owner moved")
	}
	return true
}

func (r *Relay) testMoveOwner(id string) bool {
	r.mu.Lock()
	r.moved[id] = true
	s := r.sessions[id]
	r.mu.Unlock()
	return s != nil
}
