package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/coder/websocket"
)

// Ported from test/paseo_relay/listener_test.exs.
func listenerScenario(t *testing.T, name string) relayScenarioResult {
	t.Helper()
	return requireRelayScenario(t, NewRelay(DefaultConfig()), "listener/"+name)
}

func TestListenerNativeHTTPServerServesRelayOperations(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	status, _, body := getResponse(t, server.URL+"/health")
	if status != http.StatusOK || body != `{"status":"ok"}` {
		t.Fatalf("health = (%d, %q)", status, body)
	}
}

func TestListenerStalledHTTPBodyReleasesSlotWithoutExpiringWebSockets(t *testing.T) {
	r := listenerScenario(t, "stalled-http-body")
	if r.ActiveWebSockets != 1 || r.CloseCode != 0 || !r.AdmissionOpen {
		t.Fatalf("HTTP idle handling = %#v", r)
	}
}

func TestListenerActiveWebSocketCeilingRejectsExactlyAtCapacityAndReleases(t *testing.T) {
	config := DefaultConfig()
	config.Acceptors, config.ConnectionsPerAcceptor = 1, 2
	relay := NewRelay(config)
	server := httptestServerForRelay(t, relay)
	first := dialRelay(t, server, "ceiling-1", RoleServer, 1, "")
	second := dialRelay(t, server, "ceiling-2", RoleServer, 1, "")
	ctx, cancel := context.WithTimeout(context.Background(), relayTestTimeout)
	defer cancel()
	third, response, err := websocket.Dial(ctx, relayWebSocketURL(server, "ceiling-3", RoleServer, 1, ""), nil)
	if third != nil {
		_ = third.CloseNow()
		t.Fatal("third websocket upgraded at the exact capacity ceiling")
	}
	if err == nil || response == nil || response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("capacity response = %#v, err=%v", response, err)
	}
	_ = first.Close(websocket.StatusNormalClosure, "")
	eventually(t, relayTestTimeout, func() bool { return relay.activeWebSockets.Load() == 1 })
	replacement := dialRelay(t, server, "ceiling-3", RoleServer, 1, "")
	_ = second.Close(websocket.StatusNormalClosure, "")
	_ = replacement.Close(websocket.StatusNormalClosure, "")
}

func TestListenerTimedOutUpgradeInvalidatesCapacityAndListenerEpoch(t *testing.T) {
	r := listenerScenario(t, "upgrade-timeout-epoch")
	if !r.CapacityEpochChanged || !r.ListenerEpochChanged || r.ActiveWebSockets != 0 || r.OwnerCount != 0 {
		t.Fatalf("upgrade timeout did not invalidate epochs: %#v", r)
	}
}

func TestListenerCallerDisconnectDuringStalledAdmissionLeavesNoReservation(t *testing.T) {
	r := listenerScenario(t, "caller-disconnect-stalled-admission")
	if r.ActiveWebSockets != 0 || r.IngressReservedBytes != 0 || !r.AdmissionOpen {
		t.Fatalf("stale public reservation: %#v", r)
	}
}

func TestListenerTimedOutWatermarkMutationInvalidatesCapacityEpoch(t *testing.T) {
	r := listenerScenario(t, "watermark-timeout-epoch")
	if !r.CapacityEpochChanged || !r.AdmissionOpen {
		t.Fatalf("watermark mutation did not restart Capacity: %#v", r)
	}
}

func TestListenerActiveWebSocketGaugeReconcilesAfterHeapFuseKill(t *testing.T) {
	r := listenerScenario(t, "heap-fuse-gauge")
	if r.ActiveWebSockets != 0 || r.OpenedSockets != 1 {
		t.Fatalf("active websocket gauge did not reconcile: %#v", r)
	}
}

func TestListenerQueuedReservationExpiryCannotReleaseAttachedConnection(t *testing.T) {
	r := listenerScenario(t, "stale-reservation-expiry")
	if !r.ConnectionStillAttached || r.ActiveWebSockets != 1 {
		t.Fatalf("expiry released attached connection: %#v", r)
	}
}

func TestListenerPressureSheddingMakesAdmissionTerminalForSelectedSocket(t *testing.T) {
	r := listenerScenario(t, "pressure-terminal")
	if r.AdmissionOpen || r.CloseCode != websocket.StatusTryAgainLater || r.CloseReason != "Relay memory pressure" {
		t.Fatalf("selected socket admission was not terminal: %#v", r)
	}
}

func TestListenerOnePressureCheckShedsEnoughSocketsForOvershoot(t *testing.T) {
	r := listenerScenario(t, "pressure-batch")
	if r.OpenedSockets != 2 || r.ActiveWebSockets != 0 || r.CloseCode != websocket.StatusTryAgainLater || r.CloseReason != "Relay memory pressure" {
		t.Fatalf("pressure batch = %#v", r)
	}
}
