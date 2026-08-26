package main

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/coder/websocket"
)

// These tests exercise production Relay state through real HTTP/WebSocket
// connections. They intentionally remain red until the corresponding runtime
// behavior exists; scenario_controller_test.go must not satisfy them.

func TestProductionReadinessBecomesUnavailableAtWebSocketCeiling(t *testing.T) {
	config := DefaultConfig()
	config.Acceptors = 1
	config.ConnectionsPerAcceptor = 1
	relay := NewRelay(config)
	server := httptestServerForRelay(t, relay)
	_ = dialRelay(t, server, "ready-capacity", RoleServer, 1, "")

	status, _, body := getResponse(t, server.URL+"/ready")
	if status != http.StatusServiceUnavailable || body != `{"status":"unready"}` {
		t.Fatalf("ready at websocket ceiling = (%d, %q), want (503, unready)", status, body)
	}
}

func TestProductionMetricsExposeCompleteStableSurface(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	metrics := relayMetrics(t, server)
	for _, family := range []string{
		"paseo_relay_ready",
		"paseo_relay_draining",
		"paseo_relay_active_websockets",
		"paseo_relay_active_sessions",
		"paseo_relay_reroute_responses_total",
		"paseo_relay_connection_rejections_total",
		"paseo_relay_frames_forwarded_total",
		"paseo_relay_bytes_forwarded_total",
		"paseo_relay_ingress_reserved_bytes",
		"paseo_relay_inflight_delivery_bytes",
		"paseo_relay_backpressured_sources",
		"paseo_relay_delivery_wait_seconds",
		"paseo_relay_frame_size_bytes",
		"paseo_relay_beam_binary_memory_bytes",
	} {
		if !strings.Contains(metrics, family) {
			t.Errorf("metrics missing family %q", family)
		}
	}
}

func TestProductionMetricsTrackActiveSessionsAndRemoveThemOnClose(t *testing.T) {
	relay := NewRelay(DefaultConfig())
	server := httptestServerForRelay(t, relay)
	conn := dialRelay(t, server, "metrics-session", RoleServer, 1, "")
	if got := metricValue(t, relayMetrics(t, server), "paseo_relay_active_sessions"); got != 1 {
		t.Fatalf("active sessions = %v, want 1", got)
	}

	if err := conn.Close(websocket.StatusNormalClosure, ""); err != nil {
		t.Fatal(err)
	}
	eventually(t, relayTestTimeout, func() bool {
		return !relayHasSession(relay, "metrics-session")
	})
	if got := metricValue(t, relayMetrics(t, server), "paseo_relay_active_sessions"); got != 0 {
		t.Fatalf("active sessions after close = %v, want 0", got)
	}
}

func TestProductionMetricsCountForwardedBytes(t *testing.T) {
	relay := NewRelay(DefaultConfig())
	server := httptestServerForRelay(t, relay)
	daemon := dialRelay(t, server, "byte-metric", RoleServer, 1, "")
	client := dialRelay(t, server, "byte-metric", RoleClient, 1, "")
	payload := []byte("count every forwarded byte")
	writeRelayMessage(t, client, websocket.MessageBinary, payload)
	assertRelayMessage(t, daemon, websocket.MessageBinary, payload)

	metrics := relayMetrics(t, server)
	if got := metricValue(t, metrics, "paseo_relay_bytes_forwarded_total"); got != float64(len(payload)) {
		t.Fatalf("forwarded bytes = %v, want %d", got, len(payload))
	}
}

func TestProductionMetricsCountAcceptedAndRejectedHandshakes(t *testing.T) {
	relay := NewRelay(DefaultConfig())
	server := httptestServerForRelay(t, relay)
	daemon := dialRelay(t, server, "handshake-metric", RoleServer, 1, "")
	accepted := dialRelay(t, server, "handshake-metric", RoleClient, 1, "")
	hello := handshakePayload(t, "hello", validPublicKey(t))
	writeRelayMessage(t, accepted, websocket.MessageText, hello)
	assertRelayMessage(t, daemon, websocket.MessageText, hello)

	rejected := dialRelay(t, server, "handshake-metric", RoleClient, 1, "")
	writeRelayMessage(t, rejected, websocket.MessageText, handshakePayload(t, "e2ee_hello", make([]byte, 32)))
	assertRelayClose(t, rejected, websocket.StatusPolicyViolation, "Invalid handshake key")

	metrics := relayMetrics(t, server)
	if got := metricValue(t, metrics, `paseo_relay_handshake_accepted_total{routing_version="v1",type="hello"}`); got != 1 {
		t.Fatalf("accepted hello count = %v, want 1", got)
	}
	if got := metricValue(t, metrics, `paseo_relay_handshake_rejected_total{routing_version="v1",type="e2ee_hello"}`); got != 1 {
		t.Fatalf("rejected e2ee_hello count = %v, want 1", got)
	}
}

func TestProductionHandshakeValidationAppliesOnlyToClientFrames(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	daemon := dialRelay(t, server, "server-opaque-handshake", RoleServer, 1, "")
	client := dialRelay(t, server, "server-opaque-handshake", RoleClient, 1, "")
	payload := handshakePayload(t, "hello", make([]byte, 32))

	writeRelayMessage(t, daemon, websocket.MessageBinary, payload)
	assertRelayMessage(t, client, websocket.MessageBinary, payload)
}

func TestProductionV2MissingDataRouteExpiresAndReleasesBuffer(t *testing.T) {
	config := DefaultConfig()
	config.DataAttachTimeoutMS = 50
	relay := NewRelay(config)
	server := httptestServerForRelay(t, relay)
	client := dialRelay(t, server, "missing-data", RoleClient, 2, "missing")
	writeRelayMessage(t, client, websocket.MessageBinary, []byte("retained"))
	assertRelayClose(t, client, websocket.StatusTryAgainLater, "Data route unavailable")

	eventually(t, relayTestTimeout, func() bool {
		return relayBufferedMessages(relay, "missing-data", "missing") == 0
	})
}

func TestProductionBufferedMessagesCannotExceedWeightedIngressBudget(t *testing.T) {
	config := DefaultConfig()
	// A deliberately tiny direct Config keeps this boundary test cheap. The
	// production ledger must apply the same accounting regardless of scale.
	config.IngressBudgetBytes = 8
	config.IngressWeight = 1
	config.DataAttachTimeoutMS = 5_000
	relay := NewRelay(config)
	server := httptestServerForRelay(t, relay)
	client := dialRelay(t, server, "bounded-buffer", RoleClient, 2, "waiting")
	writeRelayMessage(t, client, websocket.MessageBinary, []byte("12345678"))
	eventually(t, relayTestTimeout, func() bool {
		return relayBufferedMessages(relay, "bounded-buffer", "waiting") == 1
	})

	writeRelayMessage(t, client, websocket.MessageBinary, []byte("x"))
	assertRelayClose(t, client, websocket.StatusTryAgainLater, "Relay ingress capacity")
	if buffered := relayBufferedMessages(relay, "bounded-buffer", "waiting"); buffered > 1 {
		t.Fatalf("buffer retained %d messages after budget exhaustion", buffered)
	}
}

func TestProductionClosingV2ClientDropsUndeliveredFrames(t *testing.T) {
	relay := NewRelay(DefaultConfig())
	server := httptestServerForRelay(t, relay)
	client := dialRelay(t, server, "buffer-cleanup", RoleClient, 2, "gone")
	writeRelayMessage(t, client, websocket.MessageText, []byte("must-not-survive"))
	eventually(t, relayTestTimeout, func() bool {
		return relayBufferedMessages(relay, "buffer-cleanup", "gone") == 1
	})
	if err := client.Close(websocket.StatusNormalClosure, ""); err != nil {
		t.Fatal(err)
	}
	eventually(t, relayTestTimeout, func() bool {
		return relayBufferedMessages(relay, "buffer-cleanup", "gone") == 0
	})
}

func TestProductionEmptySessionStateIsReclaimed(t *testing.T) {
	relay := NewRelay(DefaultConfig())
	server := httptestServerForRelay(t, relay)
	conn := dialRelay(t, server, "reclaim-empty", RoleServer, 1, "")
	if err := conn.Close(websocket.StatusNormalClosure, ""); err != nil {
		t.Fatal(err)
	}
	eventually(t, relayTestTimeout, func() bool { return !relayHasSession(relay, "reclaim-empty") })
}

func TestProductionConnectionRejectionIncrementsMetricWithoutCreatingSession(t *testing.T) {
	config := DefaultConfig()
	config.Acceptors = 1
	config.ConnectionsPerAcceptor = 1
	relay := NewRelay(config)
	server := httptestServerForRelay(t, relay)
	_ = dialRelay(t, server, "capacity-held", RoleServer, 1, "")

	ctx, cancel := context.WithTimeout(context.Background(), relayTestTimeout)
	defer cancel()
	conn, response, err := websocket.Dial(ctx, relayWebSocketURL(server, "capacity-rejected", RoleServer, 1, ""), nil)
	if conn != nil {
		_ = conn.CloseNow()
		t.Fatal("connection above capacity upgraded")
	}
	if err == nil || response == nil || response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("capacity rejection response = %#v, err=%v", response, err)
	}
	if relayHasSession(relay, "capacity-rejected") {
		t.Fatal("rejected connection created session state")
	}
	if got := metricValue(t, relayMetrics(t, server), "paseo_relay_connection_rejections_total"); got != 1 {
		t.Fatalf("connection rejections = %v, want 1", got)
	}
}

func TestProductionControlReadLimitRejectsBeforeJSONHandling(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	control := dialRelay(t, server, "control-limit", RoleServer, 2, "")
	assertControlMessage(t, control, map[string]any{"type": "sync"})
	writeRelayMessage(t, control, websocket.MessageText, []byte(strings.Repeat("x", MaximumControlPayloadBytes+1)))

	ctx, cancel := context.WithTimeout(context.Background(), relayTestTimeout)
	defer cancel()
	_, _, err := control.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusMessageTooBig {
		t.Fatalf("oversized control close = %d, want 1009: %v", websocket.CloseStatus(err), err)
	}
}

func relayHasSession(relay *Relay, serverID string) bool {
	relay.mu.Lock()
	defer relay.mu.Unlock()
	_, ok := relay.sessions[serverID]
	return ok
}

func relayBufferedMessages(relay *Relay, serverID, connectionID string) int {
	relay.mu.Lock()
	defer relay.mu.Unlock()
	session := relay.sessions[serverID]
	if session == nil {
		return 0
	}
	return len(session.buffer[connectionID])
}
