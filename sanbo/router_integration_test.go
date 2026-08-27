package main

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/websocket"
)

// Ported from references/paseo-relay/test/paseo_relay/router_integration_test.exs.
func TestRouterLocallyOwnedWebSocketUpgrades(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	connection := dialRelay(t, server, "local", RoleServer, 1, "")
	if err := connection.Close(websocket.StatusNormalClosure, ""); err != nil {
		t.Fatalf("close websocket: %v", err)
	}
}

func TestRouterRejectsNonWebSocketBeforeClaimingOwnership(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	server := httptestServerForRelay(t, relay)
	serverID := "plain"
	status, _, body := getResponse(t, server.URL+"/ws?serverId="+serverID+"&role=client&v=2")
	if status != http.StatusUpgradeRequired {
		t.Fatalf("status = %d, want %d", status, http.StatusUpgradeRequired)
	}
	if body != "Expected WebSocket upgrade" {
		t.Fatalf("body = %q, want %q", body, "Expected WebSocket upgrade")
	}
	if _, owned, err := relay.ownership.lookup(serverID); err != nil || owned {
		t.Fatalf("non-upgrade request claimed ownership: owned=%t err=%v", owned, err)
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

// A valid upgrade request can still fail after Accept has written the 101
// headers (for example, when hijacking the underlying HTTP connection fails).
// The failed request must not expose a provisional owner to another request.
func TestRouterFailedWebSocketAcceptDoesNotClaimOwnership(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	serverID := "accept-failure"
	writer := &blockingHijackWriter{started: make(chan struct{}), release: make(chan struct{})}
	request := httptest.NewRequest(http.MethodGet, "http://relay.test/ws?serverId="+serverID+"&role=server", nil)
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Sec-WebSocket-Version", "13")
	request.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	done := make(chan bool, 1)
	go func() {
		_, _, _, ok := relay.admit(writer, request)
		done <- ok
	}()
	<-writer.started

	if _, owned, err := relay.ownership.lookup(serverID); err != nil || owned {
		t.Fatalf("failed Accept exposed ownership: owned=%t err=%v", owned, err)
	}
	close(writer.release)
	if ok := <-done; ok {
		t.Fatal("failed Accept returned a live connection")
	}
}

type blockingHijackWriter struct {
	header  http.Header
	started chan struct{}
	release chan struct{}
}

func (w *blockingHijackWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (*blockingHijackWriter) Write(p []byte) (int, error) { return len(p), nil }

func (*blockingHijackWriter) WriteHeader(int) {}

func (w *blockingHijackWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	close(w.started)
	<-w.release
	return nil, nil, errors.New("forced hijack failure")
}

func TestRouterRejectsOversizedRouteIdentifiersBeforeClaimingOwnership(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	server := httptestServerForRelay(t, relay)
	serverID := strings.Repeat("x", 257)
	status, _, _ := getResponse(t, server.URL+"/ws?serverId="+serverID+"&role=client&v=2")
	if status != http.StatusUpgradeRequired {
		t.Fatalf("status = %d, want %d", status, http.StatusUpgradeRequired)
	}
	if _, owned, err := relay.ownership.lookup(serverID); err != nil || owned {
		t.Fatalf("non-upgrade request with invalid query claimed ownership: owned=%t err=%v", owned, err)
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
	r := requireRelayScenario(t, mustNewRelay(t, config), "router/remote-reroute-before-upgrade")
	if r.OwnerTarget != "opaque-owner" || r.OpenedSockets != 0 {
		t.Fatalf("reroute negotiated a websocket or lost target: %#v", r)
	}
}

func TestRouterLocalPressureDoesNotSuppressRemoteReroute(t *testing.T) {
	r := requireRelayScenario(t, mustNewRelay(t, DefaultConfig()), "router/pressure-preserves-reroute")
	if r.OwnerTarget == "" || r.OpenedSockets != 0 || r.ConnectionRejections != 0 {
		t.Fatalf("local pressure suppressed remote reroute: %#v", r)
	}
}

func TestRouterMatchesReference503ReasonBodies(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*Relay)
		want   string
		member int
	}{
		{name: "draining", setup: func(relay *Relay) { relay.BeginDrain() }, want: "draining", member: 1},
		{name: "cluster", setup: func(*Relay) {}, want: "cluster", member: 0},
		{name: "owner", setup: func(relay *Relay) {
			relay.ownership = &admissionOwnershipStub{lookupErr: errors.New("owner lookup failed"), memberCount: 1}
		}, want: "owner", member: 1},
		{name: "connection capacity", setup: func(relay *Relay) {
			relay.activeWebSockets.Store(20_000)
		}, want: "Relay connection capacity", member: 1},
		{name: "memory pressure", setup: func(relay *Relay) {
			relay.memoryPressure.Store(true)
		}, want: "Relay memory pressure", member: 1},
		{name: "capacity configuration", setup: func(relay *Relay) {
			relay.Config.ConnectionsPerAcceptor++
		}, want: "Relay capacity configuration", member: 1},
		{name: "capacity unavailable", setup: func(relay *Relay) {
			relay.capacityUnavailable.Store(true)
		}, want: "Relay capacity unavailable", member: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			relay := mustNewRelay(t, DefaultConfig())
			relay.ownership = &admissionOwnershipStub{memberCount: test.member}
			test.setup(relay)
			serverID := "503-" + strings.ReplaceAll(test.name, " ", "-")
			if got := admission503Body(t, relay, serverID); got != test.want {
				t.Fatalf("503 body = %q, want %q", got, test.want)
			}
		})
	}
}

type admissionOwnershipStub struct {
	lookupErr   error
	memberErr   error
	memberCount int
}

func (*admissionOwnershipStub) identity() string { return "" }

func (stub *admissionOwnershipStub) lookup(string) (ownershipRecord, bool, error) {
	return ownershipRecord{}, false, stub.lookupErr
}

func (*admissionOwnershipStub) claim(string, *Relay) (ownershipRecord, bool, error) {
	return ownershipRecord{}, false, nil
}

func (*admissionOwnershipStub) release(string, *Relay) error { return nil }

func (*admissionOwnershipStub) ownedServers() (map[string]bool, error) { return nil, nil }

func (stub *admissionOwnershipStub) members() (int, error) {
	return stub.memberCount, stub.memberErr
}

func (*admissionOwnershipStub) close() error { return nil }

func admission503Body(t *testing.T, relay *Relay, serverID string) string {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "http://relay.test/ws?serverId="+serverID+"&role=server", nil)
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Sec-WebSocket-Version", "13")
	request.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	writer := httptest.NewRecorder()
	_, _, _, ok := relay.admit(writer, request)
	if ok {
		t.Fatal("admission unexpectedly upgraded a rejected request")
	}
	if writer.Code != http.StatusServiceUnavailable {
		t.Fatalf("admission status = %d, want 503", writer.Code)
	}
	return writer.Body.String()
}

func TestRouterMetricsExposeLocalNamesAndValues(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	status, _, body := getResponse(t, server.URL+"/metrics")
	if status != http.StatusOK || !strings.Contains(body, "paseo_relay_active_websockets 0\n") {
		t.Fatalf("metrics = (%d, %q)", status, body)
	}
}
