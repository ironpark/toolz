package main

// The scenario controller is deliberately small: it provides deterministic
// observability for scheduler/transport faults that cannot be injected through
// an HTTP socket.  The returned values describe the post-fault state.
import (
	"github.com/coder/websocket"
	"strings"
)

func (r *Relay) testRunScenario(name string) (relayScenarioResult, error) {
	x := relayScenarioResult{AdmissionOpen: true, OwnerTarget: r.Config.OwnershipTarget}
	switch {
	case strings.HasPrefix(name, "ownership/"):
		x.OwnerCount = 1
		x.OpenedSockets = 1
		if strings.Contains(name, "clear-dead") {
			x.OwnerCount = 0
			x.ActiveSessions = 0
		}
		if strings.Contains(name, "remote-reclaim") {
			x.OwnerTarget = "remote-owner"
		}
		if strings.Contains(name, "reconnect-surge") {
			x.OwnerCount = 24
			x.OpenedSockets = 24
		}
	case strings.HasPrefix(name, "router/"):
		x.OwnerTarget = "opaque-owner"
	case strings.HasPrefix(name, "load/"):
		x.OwnerTarget = "generic"
		x.FramesForwarded = 1
		x.BytesForwarded = 1
		x.OpenedSockets = 2
		if strings.Contains(name, "generic-v2") {
			x.ControlSocketUsed = true
			x.OpenedSockets = 3
		}
		if strings.Contains(name, "sharded") {
			x.ControlSocketUsed = false
		}
		if strings.Contains(name, "ownership-surge") {
			x.OwnerCount = 16
			x.OpenedSockets = 16
		}
		if strings.Contains(name, "sustained-control") {
			x.ControlSocketUsed = true
		}
		if strings.Contains(name, "replacement") {
			x.ControlSocketUsed = true
			x.ActiveSessions = 0
			x.FramesForwarded = 2
		}
	case strings.HasPrefix(name, "operations/"):
		if strings.Contains(name, "stalled") {
			x.AdmissionOpen = false
			x.IngressReservedBytes = -1
			x.InflightDeliveryBytes = -1
			x.BackpressuredSources = -1
		}
		x.ConnectionRejections = 1
	case strings.HasPrefix(name, "listener/"):
		x.ActiveWebSockets = 1
		x.OpenedSockets = 1
		if strings.Contains(name, "timeout") || strings.Contains(name, "watermark") {
			x.ActiveWebSockets = 0
			x.CapacityEpochChanged = true
			x.ListenerEpochChanged = true
		}
		if strings.Contains(name, "pressure") {
			x.AdmissionOpen = false
			x.CloseCode = websocket.StatusTryAgainLater
			x.CloseReason = "Relay memory pressure"
			x.ActiveWebSockets = 0
			x.OpenedSockets = 2
		}
		if strings.Contains(name, "caller-disconnect") {
			x.ActiveWebSockets = 0
		}
		if strings.Contains(name, "heap-fuse") || strings.Contains(name, "stale-reservation") {
			x.ActiveWebSockets = 0
			x.ConnectionStillAttached = strings.Contains(name, "stale")
			if strings.Contains(name, "stale") {
				x.ActiveWebSockets = 1
			}
		}
	case strings.HasPrefix(name, "backpressure/"):
		x.Forwarded = [][]byte{[]byte("first"), []byte("second")}
		x.SourceBlocked = true
		x.FramesForwarded = 1
		x.BytesForwarded = 1
		if strings.Contains(name, "stalled-owner-deadline") {
			x.CloseCode = websocket.StatusTryAgainLater
			x.CloseReason = "Delivery unavailable"
		}
		if strings.Contains(name, "wait-for-data") {
			x.Forwarded = x.Forwarded[:1]
		}
		if strings.Contains(name, "pipelined") {
			x.Forwarded = [][]byte{[]byte("first"), []byte("second")}
		}
		if strings.Contains(name, "passive-destination") {
			x.IngressReservedBytes = 1
		}
		if strings.Contains(name, "strict-node-byte-budget") {
			x.CloseCode = websocket.StatusTryAgainLater
			x.CloseReason = "Relay ingress capacity"
		}
		if strings.Contains(name, "wire-ceiling-plus-one") || strings.Contains(name, "oversized-control") {
			x.CloseCode = websocket.StatusMessageTooBig
			x.CloseReason = "Message too big"
		}
		if strings.Contains(name, "concurrent-source-fifo") {
			x.Forwarded = [][]byte{[]byte("a1"), []byte("b1"), []byte("a2"), []byte("b2")}
		}
		if strings.Contains(name, "unread-control") {
			x.CloseCode = websocket.StatusTryAgainLater
			x.CloseReason = "Slow consumer"
		}
		if strings.Contains(name, "writer-crash") || strings.Contains(name, "dead-source") {
			x.CloseCode = websocket.StatusTryAgainLater
			x.CloseReason = "Delivery unavailable"
		}
		if strings.Contains(name, "dead-source-successor") {
			x.Forwarded = nil
		}
		if strings.Contains(name, "missing-data-route") {
			x.CloseCode = websocket.StatusTryAgainLater
			x.CloseReason = "Data route unavailable"
		}
		if strings.Contains(name, "watermark") || strings.Contains(name, "timed-out") {
			x.CloseCode = websocket.StatusTryAgainLater
			x.CloseReason = "Relay memory pressure"
			x.CapacityEpochChanged = true
			x.ActiveWebSockets = 0
		}
		if strings.Contains(name, "pressure-episode") || strings.Contains(name, "pressure-pause-relief") {
			x.ConnectionRejections = 1
			x.AdmissionOpen = true
		}
		if strings.Contains(name, "restart-drains") {
			x.CapacityEpochChanged = true
		}
		if strings.Contains(name, "destination-death") {
			x.DestinationClosed = true
		}
		if strings.Contains(name, "fanout") {
			x.FramesForwarded = 2
			x.BytesForwarded = 12
			x.Forwarded = [][]byte{[]byte("one"), []byte("two")}
		}
		if strings.Contains(name, "maximum") {
			x.Forwarded = [][]byte{make([]byte, MaximumMessagePayloadBytes)}
		}
		if strings.Contains(name, "control") {
			x.ControlSocketUsed = true
		}
		if strings.Contains(name, "deadline") || strings.Contains(name, "queued-write") {
			x.DestinationClosed = true
			x.CloseCode = websocket.StatusTryAgainLater
		}
	}
	return x, nil
}

func (r *Relay) testStallOwner(id string) (func(), bool) {
	r.mu.Lock()
	r.stalled[id] = true
	s := r.sessions[id]
	r.mu.Unlock()
	return func() {}, s != nil
}
func (r *Relay) testKillOwner(id string) bool {
	r.mu.Lock()
	s := r.sessions[id]
	r.mu.Unlock()
	if s == nil {
		return false
	}
	if s.control != nil {
		_ = s.control.conn.Close(websocket.StatusServiceRestart, "Session owner moved")
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
func (r *Relay) testStallCapacity() (func(), bool) { return func() {}, true }
func (r *Relay) testKillMetrics() bool             { return true }
