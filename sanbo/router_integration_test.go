package main

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/coder/websocket"
)

// Ported from references/paseo-relay/test/paseo_relay/router_integration_test.exs.
func TestRouterLocallyOwnedWebSocketUpgrades(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	endpoint := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?serverId=local&role=server"
	connection, response, err := websocket.Dial(context.Background(), endpoint, nil)
	if err != nil {
		if response != nil {
			t.Fatalf("dial status %d: %v", response.StatusCode, err)
		}
		t.Fatal(err)
	}
	if err := connection.Close(websocket.StatusNormalClosure, ""); err != nil {
		t.Fatalf("close websocket: %v", err)
	}
}

func TestRouterRejectsNonWebSocketBeforeClaimingOwnership(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	status, _, _ := getResponse(t, server.URL+"/ws?serverId=plain&role=client&v=2")
	if status != http.StatusUpgradeRequired {
		t.Fatalf("status = %d, want %d", status, http.StatusUpgradeRequired)
	}
}

func TestRouterRejectsIncompleteWebSocketHandshakeBeforeClaimingOwnership(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	request, err := http.NewRequest(http.MethodGet, server.URL+"/ws?serverId=incomplete&role=client&v=2", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Sec-WebSocket-Version", "13")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusBadRequest)
	}
}

func TestRouterRejectsOversizedRouteIdentifiersBeforeClaimingOwnership(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	status, _, _ := getResponse(t, server.URL+"/ws?serverId="+strings.Repeat("x", 257)+"&role=client&v=2")
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", status, http.StatusBadRequest)
	}
}

func TestRouterHealthIsLiveWhileReadinessBlocksOwnership(t *testing.T) {
	config := DefaultConfig()
	config.Drain = true
	server := newRelayTestServer(t, config)
	healthStatus, _, _ := getResponse(t, server.URL+"/health")
	readyStatus, _, _ := getResponse(t, server.URL+"/ready")
	if healthStatus != http.StatusOK || readyStatus != http.StatusServiceUnavailable {
		t.Fatalf("health/ready = %d/%d", healthStatus, readyStatus)
	}
}

func TestRouterReturnsOpaqueRerouteBeforeWebSocketNegotiation(t *testing.T) {
	config := DefaultConfig()
	config.OwnershipTarget = "opaque-owner"
	r := requireRelayScenario(t, NewRelay(config), "router/remote-reroute-before-upgrade")
	if r.OwnerTarget != "opaque-owner" || r.OpenedSockets != 0 {
		t.Fatalf("reroute negotiated a websocket or lost target: %#v", r)
	}
}

func TestRouterLocalPressureDoesNotSuppressRemoteReroute(t *testing.T) {
	r := requireRelayScenario(t, NewRelay(DefaultConfig()), "router/pressure-preserves-reroute")
	if r.OwnerTarget == "" || r.OpenedSockets != 0 || r.ConnectionRejections != 0 {
		t.Fatalf("local pressure suppressed remote reroute: %#v", r)
	}
}

func TestRouterMetricsExposeLocalNamesAndValues(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	status, _, body := getResponse(t, server.URL+"/metrics")
	if status != http.StatusOK || !strings.Contains(body, "paseo_relay_active_websockets 0\n") {
		t.Fatalf("metrics = (%d, %q)", status, body)
	}
}
