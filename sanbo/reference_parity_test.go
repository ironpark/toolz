package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/coder/websocket"
)

// The cases here cover behaviors ported from references/paseo-relay one by one.
// Each names the reference source it mirrors so the two can be diffed.

// --- ownership.ex:327-341 — one socket per single-holder role ---

func TestV1ServerAttachReplacesPreviousSocket(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	first := dialRelay(t, server, "v1-replace-server", RoleServer, 1, "")
	second := dialRelay(t, server, "v1-replace-server", RoleServer, 1, "")
	defer second.CloseNow()

	assertRelayClose(t, first, websocket.StatusPolicyViolation, "Replaced by new connection")
}

func TestV1ClientAttachReplacesPreviousSocket(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	first := dialRelay(t, server, "v1-replace-client", RoleClient, 1, "")
	second := dialRelay(t, server, "v1-replace-client", RoleClient, 1, "")
	defer second.CloseNow()

	assertRelayClose(t, first, websocket.StatusPolicyViolation, "Replaced by new connection")
}

func TestControlAttachReplacesPreviousControlSocket(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	first := dialRelay(t, server, "control-replace", RoleServer, 2, "")
	assertControlMessage(t, first, map[string]any{"type": "sync"})
	second := dialRelay(t, server, "control-replace", RoleServer, 2, "")
	defer second.CloseNow()
	assertControlMessage(t, second, map[string]any{"type": "sync"})

	assertRelayClose(t, first, websocket.StatusPolicyViolation, "Replaced by new connection")
}

// A replaced socket must not tear down the state its replacement now holds.
func TestReplacedControlSocketDoesNotDetachItsReplacement(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	server := httptestServerForRelay(t, relay)
	first := dialRelay(t, server, "control-replace-detach", RoleServer, 2, "")
	assertControlMessage(t, first, map[string]any{"type": "sync"})
	second := dialRelay(t, server, "control-replace-detach", RoleServer, 2, "")
	defer second.CloseNow()
	assertControlMessage(t, second, map[string]any{"type": "sync"})
	assertRelayClose(t, first, websocket.StatusPolicyViolation, "Replaced by new connection")

	// The surviving control socket still receives route notifications.
	client := dialRelay(t, server, "control-replace-detach", RoleClient, 2, "route-1")
	defer client.CloseNow()
	assertControlMessage(t, second, map[string]any{"type": "connected", "connectionId": "route-1"})
}

// --- ownership.ex:393-409 — data detach orphans its clients ---

func TestDataSocketDetachClosesRouteClients(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	client := dialRelay(t, server, "data-detach", RoleClient, 2, "route-a")
	data := dialRelay(t, server, "data-detach", RoleServer, 2, "route-a")

	_ = data.CloseNow()

	assertRelayClose(t, client, websocket.StatusServiceRestart, "Server disconnected")
}

func TestDataSocketDetachClosesEveryClientOnItsRoute(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	first := dialRelay(t, server, "data-detach-fanout", RoleClient, 2, "route-b")
	second := dialRelay(t, server, "data-detach-fanout", RoleClient, 2, "route-b")
	data := dialRelay(t, server, "data-detach-fanout", RoleServer, 2, "route-b")

	_ = data.CloseNow()

	assertRelayClose(t, first, websocket.StatusServiceRestart, "Server disconnected")
	assertRelayClose(t, second, websocket.StatusServiceRestart, "Server disconnected")
}

// A replaced data socket hands the route to its replacement, so its clients
// stay open.
func TestReplacedDataSocketDoesNotOrphanRouteClients(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	client := dialRelay(t, server, "data-replace", RoleClient, 2, "route-c")
	defer client.CloseNow()
	first := dialRelay(t, server, "data-replace", RoleServer, 2, "route-c")
	second := dialRelay(t, server, "data-replace", RoleServer, 2, "route-c")
	defer second.CloseNow()

	assertRelayClose(t, first, websocket.StatusPolicyViolation, "Replaced by new connection")
	writeRelayMessage(t, second, websocket.MessageBinary, []byte("still routed"))
	assertRelayMessage(t, client, websocket.MessageBinary, []byte("still routed"))
}

// --- delivery/writer.ex:95-112 — bounded control queue, slow consumer ---

// controlPeer returns the live control socket of a session.
func controlPeer(t *testing.T, relay *Relay, serverID string) *relayPeer {
	t.Helper()
	var peer *relayPeer
	eventually(t, relayTestTimeout, func() bool {
		relay.mu.Lock()
		defer relay.mu.Unlock()
		if s := relay.sessions[serverID]; s != nil {
			peer = s.control
		}
		return peer != nil
	})
	return peer
}

func TestControlQueueOverflowClosesDestinationAsSlowConsumer(t *testing.T) {
	config := DefaultConfig()
	config.ControlQueueBytes = 64
	relay := mustNewRelay(t, config)
	server := httptestServerForRelay(t, relay)
	control := dialRelay(t, server, "control-queue-overflow", RoleServer, 2, "")
	defer control.CloseNow()
	assertControlMessage(t, control, map[string]any{"type": "sync"})

	// Hold the destination's writer so the next notification has to queue.
	peer := controlPeer(t, relay, "control-queue-overflow")
	peer.writeSlot <- struct{}{}
	defer func() { <-peer.writeSlot }()

	err := relay.sendControl(peer, []byte(strings.Repeat("x", 65)))

	if err != errControlQueue {
		t.Fatalf("sendControl over the queue bound = %v, want %v", err, errControlQueue)
	}
	if got := relay.slowConsumerDisconnects.Load(); got != 1 {
		t.Fatalf("slow consumer disconnects = %d, want 1", got)
	}
	assertRelayClose(t, control, websocket.StatusTryAgainLater, "Slow consumer")
}

// Control frames queue behind an in-flight write instead of being dropped.
func TestControlFrameQueuesBehindAnInFlightWrite(t *testing.T) {
	config := DefaultConfig()
	config.ControlQueueBytes = 1024
	relay := mustNewRelay(t, config)
	server := httptestServerForRelay(t, relay)
	control := dialRelay(t, server, "control-queue-order", RoleServer, 2, "")
	defer control.CloseNow()
	assertControlMessage(t, control, map[string]any{"type": "sync"})

	peer := controlPeer(t, relay, "control-queue-order")
	peer.writeSlot <- struct{}{}
	queued := make(chan error, 1)
	go func() { queued <- relay.sendControl(peer, []byte(`{"type":"queued"}`)) }()

	eventually(t, relayTestTimeout, func() bool { return peer.controlQueued.Load() > 0 })
	<-peer.writeSlot

	if err := <-queued; err != nil {
		t.Fatalf("queued control frame: %v", err)
	}
	assertControlMessage(t, control, map[string]any{"type": "queued"})
	if got := relay.slowConsumerDisconnects.Load(); got != 0 {
		t.Fatalf("slow consumer disconnects = %d, want 0", got)
	}
}

// --- capacity.ex:158-236, 527-563 — admission and delivery under pressure ---

func TestMemoryPressureRefusesInboundFramesFromAttachedSockets(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	server := httptestServerForRelay(t, relay)
	client := dialRelay(t, server, "pressure-inbound", RoleClient, 1, "")
	defer client.CloseNow()

	relay.memoryPressure.Store(true)
	writeRelayMessage(t, client, websocket.MessageBinary, []byte("blocked"))

	assertRelayClose(t, client, websocket.StatusTryAgainLater, "Relay ingress capacity")
}

func TestShedSocketRefusesFurtherFrames(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	server := httptestServerForRelay(t, relay)
	client := dialRelay(t, server, "shed-inbound", RoleClient, 1, "")
	defer client.CloseNow()

	eventually(t, relayTestTimeout, func() bool {
		relay.mu.Lock()
		defer relay.mu.Unlock()
		s := relay.sessions["shed-inbound"]
		if s == nil || s.v1Client == nil {
			return false
		}
		s.v1Client.shed.Store(true)
		return true
	})
	writeRelayMessage(t, client, websocket.MessageBinary, []byte("blocked"))

	assertRelayClose(t, client, websocket.StatusTryAgainLater, "Relay ingress capacity")
}

// Pressure that engages between admission and delivery is reported with the
// delivery-stage reason rather than the ingress one.
func TestMemoryPressureAtDeliveryStartClosesWithPressureReason(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	server := httptestServerForRelay(t, relay)
	client := dialRelay(t, server, "pressure-delivery", RoleClient, 1, "")
	defer client.CloseNow()

	source := relayV1ClientPeer(t, relay, "pressure-delivery")
	relay.memoryPressure.Store(true)
	relay.route(Connection{ServerID: "pressure-delivery", Role: RoleClient, Version: 1}, source, websocket.MessageBinary, []byte("late"))

	assertRelayClose(t, client, websocket.StatusTryAgainLater, "Relay memory pressure")
}

func relayV1ClientPeer(t *testing.T, relay *Relay, serverID string) *relayPeer {
	t.Helper()
	var peer *relayPeer
	eventually(t, relayTestTimeout, func() bool {
		relay.mu.Lock()
		defer relay.mu.Unlock()
		if s := relay.sessions[serverID]; s != nil {
			peer = s.v1Client
		}
		return peer != nil
	})
	return peer
}

// --- capacity.ex:519-563 — victim order ---

func TestShedPicksLongestBlockedSourceBeforeNewestActiveSocket(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	oldBlocked, newBlocked := newRelayPeer(nil), newRelayPeer(nil)
	oldActive, newActive := newRelayPeer(nil), newRelayPeer(nil)
	for _, peer := range []*relayPeer{oldBlocked, newBlocked, oldActive, newActive} {
		peer.attachSeq.Store(relay.nextSeq())
	}
	oldBlocked.blockSeq.Store(relay.nextSeq())
	newBlocked.blockSeq.Store(relay.nextSeq())
	relay.mu.Lock()
	relay.sessions["shed-order"] = &relaySession{
		clients: map[string][]*relayPeer{"route": {oldBlocked, newBlocked, oldActive, newActive}},
		data:    map[string]*relayPeer{},
		buffer:  map[string][]relayMessage{},
	}
	got := relay.shedCandidatesLocked()
	relay.mu.Unlock()

	want := []*relayPeer{oldBlocked, newBlocked, newActive, oldActive}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("shed candidate %d is not the expected socket; order = %v", i, got)
		}
	}
}

func TestShedSkipsSocketsAlreadyChosen(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	already, next := newRelayPeer(nil), newRelayPeer(nil)
	already.shed.Store(true)
	next.attachSeq.Store(relay.nextSeq())
	relay.mu.Lock()
	relay.sessions["shed-skip"] = &relaySession{
		clients: map[string][]*relayPeer{"route": {already, next}},
		data:    map[string]*relayPeer{},
		buffer:  map[string][]relayMessage{},
	}
	got := relay.shedCandidatesLocked()
	relay.mu.Unlock()

	if len(got) != 1 || got[0] != next {
		t.Fatalf("shed candidates = %v, want only the socket not already shed", got)
	}
}

// --- drain.ex + operations.ex:63-70 + ownership.ex:58-63 — runtime drain ---

func TestDrainBeginsAndCancelsAtRuntime(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	server := httptestServerForRelay(t, relay)
	if status, _, _ := getResponse(t, server.URL+"/ready"); status != http.StatusOK {
		t.Fatalf("readiness before drain = %d, want 200", status)
	}

	relay.BeginDrain()

	if !relay.Draining() {
		t.Fatal("BeginDrain did not engage drain")
	}
	if status, _, _ := getResponse(t, server.URL+"/ready"); status != http.StatusServiceUnavailable {
		t.Fatalf("readiness during drain = %d, want 503", status)
	}
	if metrics := relayMetrics(t, server); metricValue(t, metrics, "paseo_relay_draining") != 1 {
		t.Fatal("paseo_relay_draining did not follow the runtime drain")
	}

	relay.CancelDrain()

	if status, _, _ := getResponse(t, server.URL+"/ready"); status != http.StatusOK {
		t.Fatalf("readiness after cancel = %d, want 200", status)
	}
	conn := dialRelay(t, server, "drain-cancelled", RoleServer, 1, "")
	_ = conn.CloseNow()
}

func TestDrainRefusesNewSessionClaimsWith503Draining(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	server := httptestServerForRelay(t, relay)
	relay.BeginDrain()

	ctx, cancel := context.WithTimeout(context.Background(), relayTestTimeout)
	defer cancel()
	conn, response, err := websocket.Dial(ctx, relayWebSocketURL(server, "drain-new-session", RoleServer, 1, ""), nil)
	if conn != nil {
		_ = conn.CloseNow()
		t.Fatal("a draining node claimed a new session")
	}
	if err == nil || response == nil || response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("dial during drain: response=%#v err=%v, want 503", response, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "draining" {
		t.Fatalf("503 body = %q (err=%v), want %q", body, err, "draining")
	}
}

// A drain empties the node gradually: sessions it already owns keep accepting
// sockets so their traffic can finish.
func TestDrainKeepsServingSessionsThisNodeAlreadyOwns(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	server := httptestServerForRelay(t, relay)
	existing := dialRelay(t, server, "drain-existing", RoleServer, 1, "")
	defer existing.CloseNow()

	relay.BeginDrain()

	client := dialRelay(t, server, "drain-existing", RoleClient, 1, "")
	defer client.CloseNow()
	writeRelayMessage(t, client, websocket.MessageBinary, []byte("still relayed"))
	assertRelayMessage(t, existing, websocket.MessageBinary, []byte("still relayed"))
}

// --- socket.ex:65-69 — per-socket heap fuse ---

func TestSocketHeapFuseKillsTheSocketWithoutACloseFrame(t *testing.T) {
	config := DefaultConfig()
	// One word: any frame this socket routes is over its ceiling.
	config.WebsocketMaxHeapWords = 1
	server := newRelayTestServer(t, config)
	client := dialRelay(t, server, "heap-fuse", RoleClient, 1, "")
	defer client.CloseNow()

	writeRelayMessage(t, client, websocket.MessageBinary, []byte("over the per-socket ceiling"))

	ctx, cancel := context.WithTimeout(context.Background(), relayTestTimeout)
	defer cancel()
	_, _, err := client.Read(ctx)
	if err == nil {
		t.Fatal("a socket over its heap fuse stayed open")
	}
	if code := websocket.CloseStatus(err); code != -1 {
		t.Fatalf("heap fuse close status = %d, want no close frame", code)
	}
}

func TestSocketHeapFuseAdmitsFramesWithinTheCeiling(t *testing.T) {
	config := DefaultConfig()
	config.WebsocketMaxHeapWords = 1024
	server := newRelayTestServer(t, config)
	daemon := dialRelay(t, server, "heap-fuse-ok", RoleServer, 1, "")
	defer daemon.CloseNow()
	client := dialRelay(t, server, "heap-fuse-ok", RoleClient, 1, "")
	defer client.CloseNow()

	writeRelayMessage(t, client, websocket.MessageBinary, []byte("within the ceiling"))

	assertRelayMessage(t, daemon, websocket.MessageBinary, []byte("within the ceiling"))
}

// The heap charge of a frame is retired once it is routed, so a socket can
// keep sending indefinitely.
func TestSocketHeapChargeIsRetiredAfterRouting(t *testing.T) {
	config := DefaultConfig()
	config.WebsocketMaxHeapWords = 4
	relay := mustNewRelay(t, config)
	server := httptestServerForRelay(t, relay)
	daemon := dialRelay(t, server, "heap-release", RoleServer, 1, "")
	defer daemon.CloseNow()
	client := dialRelay(t, server, "heap-release", RoleClient, 1, "")
	defer client.CloseNow()

	for range 4 {
		writeRelayMessage(t, client, websocket.MessageBinary, []byte("32 bytes exactly ................"[:32]))
		assertRelayMessage(t, daemon, websocket.MessageBinary, []byte("32 bytes exactly ................"[:32]))
	}

	source := relayV1ClientPeer(t, relay, "heap-release")
	if held := source.heapBytes.Load(); held != 0 {
		t.Fatalf("heap held after routing = %d, want 0", held)
	}
}
