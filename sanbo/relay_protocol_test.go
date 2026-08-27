package main

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// Ported from references/paseo-relay/test/relay_protocol_test.exs.
func TestRelayV1ForwardsOrderedTextAndBinaryFrames(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	serverID := "v1-ordered"
	daemon := dialRelay(t, server, serverID, RoleServer, 1, "")
	client := dialRelay(t, server, serverID, RoleClient, 1, "")
	writeRelayMessage(t, client, websocket.MessageText, []byte("one"))
	writeRelayMessage(t, client, websocket.MessageBinary, []byte{0, 255, 1})
	assertRelayMessage(t, daemon, websocket.MessageText, []byte("one"))
	assertRelayMessage(t, daemon, websocket.MessageBinary, []byte{0, 255, 1})
}

func TestRelayV2ControlPairsClientsAndDeliversWaitingFramesInOrder(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	serverID := "v2-waiting"
	control := dialRelay(t, server, serverID, RoleServer, 2, "")
	assertControlMessage(t, control, map[string]any{"type": "sync", "connectionIds": []string{}})
	client := dialRelay(t, server, serverID, RoleClient, 2, "client-1")
	assertControlMessage(t, control, map[string]any{"type": "connected", "connectionId": "client-1"})
	writeRelayMessage(t, client, websocket.MessageText, []byte("before-data"))
	writeRelayMessage(t, client, websocket.MessageBinary, []byte{2, 3, 5})
	data := dialRelay(t, server, serverID, RoleServer, 2, "client-1")
	assertRelayMessage(t, data, websocket.MessageText, []byte("before-data"))
	assertRelayMessage(t, data, websocket.MessageBinary, []byte{2, 3, 5})
	writeRelayMessage(t, data, websocket.MessageText, []byte("from-daemon"))
	assertRelayMessage(t, client, websocket.MessageText, []byte("from-daemon"))
}

func TestRelayV2ControlAnswersLegacyJSONPingWithJSONPong(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	control := dialRelay(t, server, "v2-ping", RoleServer, 2, "")
	assertControlMessage(t, control, map[string]any{"type": "sync"})
	beforeFrames := metricValue(t, relayMetrics(t, server), "paseo_relay_frames_forwarded_total")
	writeRelayMessage(t, control, websocket.MessageText, []byte(`{"type":"ping"}`))
	pong := assertControlMessage(t, control, map[string]any{"type": "pong"})
	if _, ok := pong["ts"].(float64); !ok {
		t.Fatalf("pong timestamp = %#v", pong["ts"])
	}
	afterFrames := metricValue(t, relayMetrics(t, server), "paseo_relay_frames_forwarded_total")
	if afterFrames != beforeFrames+1 {
		t.Fatalf("frames forwarded = %v, want %v", afterFrames, beforeFrames+1)
	}
}

func TestRelayV2ControlDiscardsBinaryBeforeObservationAndAdmission(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	server := httptestServerForRelay(t, relay)
	control := dialRelay(t, server, "v2-control-binary", RoleServer, 2, "")
	assertControlMessage(t, control, map[string]any{"type": "sync"})

	beforeCount := relay.frameCount.Load()
	beforeBytes := relay.frameBytes.Load()
	beforeMax := relay.maxFrameBytes.Load()
	beforeReserved := relay.ingressReserved.Load()
	beforeInFlight := relay.ingressInFlight.Load()
	beforeForwarded := relay.framesForwarded.Load()
	ping := []byte(`{"type":"ping"}`)
	writeRelayMessage(t, control, websocket.MessageBinary, []byte("discard me"))
	writeRelayMessage(t, control, websocket.MessageText, ping)
	assertControlMessage(t, control, map[string]any{"type": "pong"})

	if got := relay.frameCount.Load(); got != beforeCount+1 {
		t.Fatalf("observed frames = %d, want %d (only ping)", got, beforeCount+1)
	}
	if got := relay.frameBytes.Load(); got != beforeBytes+int64(len(ping)) {
		t.Fatalf("observed bytes = %d, want %d (only ping)", got, beforeBytes+int64(len(ping)))
	}
	wantMax := max(beforeMax, int64(len(ping)))
	if got := relay.maxFrameBytes.Load(); got != wantMax {
		t.Fatalf("max frame bytes = %d, want %d", got, wantMax)
	}
	if relay.ingressReserved.Load() != beforeReserved || relay.ingressInFlight.Load() != beforeInFlight {
		t.Fatalf("control binary changed ingress accounting: reserved %d/%d in-flight %d/%d", relay.ingressReserved.Load(), beforeReserved, relay.ingressInFlight.Load(), beforeInFlight)
	}
	if got := relay.framesForwarded.Load(); got != beforeForwarded+1 {
		t.Fatalf("forwarded frames = %d, want %d (only pong)", got, beforeForwarded+1)
	}
}

func TestRelayV2ClientBlocksOneMessageUntilDataAttaches(t *testing.T) {
	config := DefaultConfig()
	config.DeliveryTimeoutMS = 500
	config.DataAttachTimeoutMS = 1_000
	relay := mustNewRelay(t, config)
	server := httptestServerForRelay(t, relay)
	client := dialRelay(t, server, "v2-single-flight", RoleClient, 2, "waiting")

	writeRelayMessage(t, client, websocket.MessageText, []byte("first"))
	eventually(t, relayTestTimeout, func() bool {
		relay.mu.Lock()
		defer relay.mu.Unlock()
		session := relay.sessions["v2-single-flight"]
		return session != nil && len(session.waiting["waiting"]) == 1
	})

	writeRelayMessage(t, client, websocket.MessageBinary, []byte("second"))
	time.Sleep(25 * time.Millisecond)
	if got := relay.frameCount.Load(); got != 1 {
		t.Fatalf("source read %d frames while first delivery was waiting, want 1", got)
	}
	relay.mu.Lock()
	waiting := len(relay.sessions["v2-single-flight"].waiting["waiting"])
	relay.mu.Unlock()
	if waiting != 1 {
		t.Fatalf("source waiters = %d, want one in-flight waiter", waiting)
	}

	data := dialRelay(t, server, "v2-single-flight", RoleServer, 2, "waiting")
	assertRelayMessage(t, data, websocket.MessageText, []byte("first"))
	assertRelayMessage(t, data, websocket.MessageBinary, []byte("second"))
}

func TestRelayV2PreAttachWaitUsesTheEarlierDeadline(t *testing.T) {
	tests := []struct {
		name       string
		delivery   int
		attach     int
		wantCode   websocket.StatusCode
		wantReason string
	}{
		{name: "delivery deadline", delivery: 50, attach: 500, wantCode: websocket.StatusTryAgainLater, wantReason: "Delivery unavailable"},
		{name: "data attach deadline", delivery: 500, attach: 50, wantCode: websocket.StatusTryAgainLater, wantReason: "Data route unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := DefaultConfig()
			config.DeliveryTimeoutMS = test.delivery
			config.DataAttachTimeoutMS = test.attach
			relay := mustNewRelay(t, config)
			server := httptestServerForRelay(t, relay)
			client := dialRelay(t, server, "v2-timeout-"+strings.ReplaceAll(test.name, " ", "-"), RoleClient, 2, "waiting")
			writeRelayMessage(t, client, websocket.MessageBinary, []byte("waiting"))
			assertRelayClose(t, client, test.wantCode, test.wantReason)
			eventually(t, relayTestTimeout, func() bool {
				return relay.ingressReserved.Load() == 0 && relay.ingressInFlight.Load() == 0
			})
		})
	}
}

func TestRelayV2PongQueueFailureClosesWithDeliveryUnavailable(t *testing.T) {
	config := DefaultConfig()
	config.DeliveryTimeoutMS = 40
	relay := mustNewRelay(t, config)
	server := httptestServerForRelay(t, relay)
	control := dialRelay(t, server, "v2-pong-failure", RoleServer, 2, "")
	assertControlMessage(t, control, map[string]any{"type": "sync"})
	peer := relay.testPeer("v2-pong-failure", RoleServer, 2, "")
	if peer == nil {
		t.Fatal("control peer missing")
	}
	peer.writeSlot <- struct{}{}
	defer func() { <-peer.writeSlot }()

	writeRelayMessage(t, control, websocket.MessageText, []byte(`{"type":"ping"}`))
	assertRelayClose(t, control, websocket.StatusTryAgainLater, "Delivery unavailable")
}

func TestRelayV2ResetsUnresponsiveControlAfterNudgingDataAttachment(t *testing.T) {
	server := newControlWatchdogTestServer(t, DefaultConfig(), 20*time.Millisecond, 20*time.Millisecond)
	serverID := "v2-control-watchdog"
	control := dialRelay(t, server, serverID, RoleServer, 2, "")
	assertControlMessage(t, control, map[string]any{"type": "sync", "connectionIds": []string{}})
	_ = dialRelay(t, server, serverID, RoleClient, 2, "waiting")
	assertControlMessage(t, control, map[string]any{"type": "connected", "connectionId": "waiting"})
	assertControlMessage(t, control, map[string]any{"type": "sync", "connectionIds": []string{"waiting"}})
	assertRelayClose(t, control, websocket.StatusInternalError, "Control unresponsive")
}

func TestRelayV2ControlWatchdogStopsAfterDataAttachment(t *testing.T) {
	server := newControlWatchdogTestServer(t, DefaultConfig(), 20*time.Millisecond, 20*time.Millisecond)
	serverID := "v2-control-watchdog-attach"
	control := dialRelay(t, server, serverID, RoleServer, 2, "")
	assertControlMessage(t, control, map[string]any{"type": "sync"})
	_ = dialRelay(t, server, serverID, RoleClient, 2, "attached")
	assertControlMessage(t, control, map[string]any{"type": "connected", "connectionId": "attached"})
	assertControlMessage(t, control, map[string]any{"type": "sync", "connectionIds": []string{"attached"}})
	_ = dialRelay(t, server, serverID, RoleServer, 2, "attached")

	// The close stage re-checks attachment, so the control socket survives it.
	time.Sleep(100 * time.Millisecond)
	writeRelayMessage(t, control, websocket.MessageText, []byte(`{"type":"ping"}`))
	assertControlMessage(t, control, map[string]any{"type": "pong"})
}

// TestRelayV2ControlWatchdogSkipsSyncWhenDataAttachesFirst covers the first
// stage's re-check: data attached before the nudge means no nudge at all.
func TestRelayV2ControlWatchdogSkipsSyncWhenDataAttachesFirst(t *testing.T) {
	server := newControlWatchdogTestServer(t, DefaultConfig(), 150*time.Millisecond, 20*time.Millisecond)
	serverID := "v2-control-watchdog-early"
	control := dialRelay(t, server, serverID, RoleServer, 2, "")
	assertControlMessage(t, control, map[string]any{"type": "sync"})
	_ = dialRelay(t, server, serverID, RoleClient, 2, "early")
	assertControlMessage(t, control, map[string]any{"type": "connected", "connectionId": "early"})
	_ = dialRelay(t, server, serverID, RoleServer, 2, "early")

	time.Sleep(250 * time.Millisecond)
	writeRelayMessage(t, control, websocket.MessageText, []byte(`{"type":"ping"}`))
	assertControlMessage(t, control, map[string]any{"type": "pong"})
}

// TestRelayV2ControlSyncListsExistingClientRoutes covers a daemon whose control
// socket attaches after its clients: the roster is not empty for it.
func TestRelayV2ControlSyncListsExistingClientRoutes(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	serverID := "v2-sync-roster"
	_ = dialRelay(t, server, serverID, RoleClient, 2, "route-a")
	_ = dialRelay(t, server, serverID, RoleClient, 2, "route-b")
	control := dialRelay(t, server, serverID, RoleServer, 2, "")
	assertControlMessage(t, control, map[string]any{"type": "sync", "connectionIds": []string{"route-a", "route-b"}})
}

func TestRelayV2FansOutDataToEveryClientOnTheRoute(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	serverID := "v2-fanout"
	control := dialRelay(t, server, serverID, RoleServer, 2, "")
	assertControlMessage(t, control, map[string]any{"type": "sync"})
	first := dialRelay(t, server, serverID, RoleClient, 2, "shared")
	assertControlMessage(t, control, map[string]any{"type": "connected", "connectionId": "shared"})
	second := dialRelay(t, server, serverID, RoleClient, 2, "shared")
	// connected is per attach, so the same ID announces itself twice.
	assertControlMessage(t, control, map[string]any{"type": "connected", "connectionId": "shared"})
	data := dialRelay(t, server, serverID, RoleServer, 2, "shared")

	writeRelayMessage(t, data, websocket.MessageText, []byte("broadcast"))
	assertRelayMessage(t, first, websocket.MessageText, []byte("broadcast"))
	assertRelayMessage(t, second, websocket.MessageText, []byte("broadcast"))

	if err := first.Close(websocket.StatusNormalClosure, ""); err != nil {
		t.Fatal(err)
	}
	writeRelayMessage(t, data, websocket.MessageText, []byte("still-routed"))
	assertRelayMessage(t, second, websocket.MessageText, []byte("still-routed"))

	if err := second.Close(websocket.StatusNormalClosure, ""); err != nil {
		t.Fatal(err)
	}
	assertRelayClose(t, data, websocket.StatusGoingAway, "Client disconnected")
	assertControlMessage(t, control, map[string]any{"type": "disconnected", "connectionId": "shared"})
}

// slowConsumerConfig keeps the delivery deadline short so a blocked
// destination fails within a test timeout.
func slowConsumerConfig() Config {
	config := DefaultConfig()
	config.DeliveryTimeoutMS = 50
	return config
}

func TestRelayV2SlowClientDoesNotBreakFanOutForTheOthers(t *testing.T) {
	relay := mustNewRelay(t, slowConsumerConfig())
	server := httptestServerForRelay(t, relay)
	serverID := "v2-fanout-slow"
	blocked := dialRelay(t, server, serverID, RoleClient, 2, "shared")
	healthy := dialRelay(t, server, serverID, RoleClient, 2, "shared")
	data := dialRelay(t, server, serverID, RoleServer, 2, "shared")
	eventually(t, relayTestTimeout, func() bool { return len(relayClientPeers(relay, serverID, "shared")) == 2 })
	peers := relayClientPeers(relay, serverID, "shared")

	peers[0].writeSlot <- struct{}{}
	writeRelayMessage(t, data, websocket.MessageText, []byte("fan-out"))
	assertRelayMessage(t, healthy, websocket.MessageText, []byte("fan-out"))
	assertRelayClose(t, blocked, websocket.StatusTryAgainLater, "Slow consumer")
	<-peers[0].writeSlot

	// The source stays open because one destination took the frame.
	writeRelayMessage(t, data, websocket.MessageText, []byte("after"))
	assertRelayMessage(t, healthy, websocket.MessageText, []byte("after"))
}

func TestRelayV2ClosesSourceOnlyWhenEveryDestinationFails(t *testing.T) {
	relay := mustNewRelay(t, slowConsumerConfig())
	server := httptestServerForRelay(t, relay)
	serverID := "v2-fanout-stuck"
	_ = dialRelay(t, server, serverID, RoleClient, 2, "shared")
	_ = dialRelay(t, server, serverID, RoleClient, 2, "shared")
	data := dialRelay(t, server, serverID, RoleServer, 2, "shared")
	eventually(t, relayTestTimeout, func() bool { return len(relayClientPeers(relay, serverID, "shared")) == 2 })
	peers := relayClientPeers(relay, serverID, "shared")

	for _, peer := range peers {
		peer.writeSlot <- struct{}{}
	}
	writeRelayMessage(t, data, websocket.MessageText, []byte("nowhere"))
	assertRelayClose(t, data, websocket.StatusTryAgainLater, "Delivery unavailable")
	for _, peer := range peers {
		<-peer.writeSlot
	}
}

func TestRelayV2PayloadDeliveryDoesNotUseNodeWideRegistry(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	leftClient := dialRelay(t, server, "route-left", RoleClient, 2, "same")
	leftData := dialRelay(t, server, "route-left", RoleServer, 2, "same")
	rightData := dialRelay(t, server, "route-right", RoleServer, 2, "same")
	writeRelayMessage(t, leftClient, websocket.MessageText, []byte("owner-local"))
	assertRelayMessage(t, leftData, websocket.MessageText, []byte("owner-local"))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, _, err := rightData.Read(ctx); err == nil {
		t.Fatal("payload crossed server ownership boundary")
	}
}

func TestRelayV2SocketsFailClosedWhenSessionOwnerExits(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	server := httptestServerForRelay(t, relay)
	serverID := "v2-owner-exit"
	control := dialRelay(t, server, serverID, RoleServer, 2, "")
	assertControlMessage(t, control, map[string]any{"type": "sync"})
	if !relay.testKillOwner(serverID) {
		t.Fatal("owner was not found")
	}
	assertRelayClose(t, control, websocket.StatusServiceRestart, "Session owner moved")
}

func TestRelayOwnerCallWatchdogClosesEveryAttachedSocket(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	relay.ownerCallTimeout = 20 * time.Millisecond
	server := httptestServerForRelay(t, relay)
	serverID := "owner-call-watchdog"
	daemon := dialRelay(t, server, serverID, RoleServer, 1, "")
	client := dialRelay(t, server, serverID, RoleClient, 1, "")

	// Hold the session lock after both sockets are attached. The owner-side
	// destination call is now stalled in the same place a slow owner operation
	// would be, while the watchdog must still close the published peer snapshot.
	relay.mu.Lock()
	writeDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		writeDone <- client.Write(ctx, websocket.MessageText, []byte("stalled"))
	}()
	eventually(t, time.Second, func() bool {
		value, ok := relay.ownerSessions.Load(serverID)
		if !ok {
			return false
		}
		session, ok := value.(*relaySession)
		return ok && session.ownerClosed.Load()
	})
	assertRelayClose(t, daemon, websocket.StatusServiceRestart, "Session owner moved")
	assertRelayClose(t, client, websocket.StatusServiceRestart, "Session owner moved")
	relay.mu.Unlock()

	select {
	case <-writeDone:
	case <-time.After(time.Second):
		t.Fatal("stalled owner-call write did not unwind")
	}
	eventually(t, time.Second, func() bool {
		return relay.activeWebSockets.Load() == 0 && !relayHasSession(relay, serverID)
	})
}

func TestRelayV2SocketInitializationFailsClosedWhenOwnerMoves(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	server := httptestServerForRelay(t, relay)
	serverID := "v2-owner-moved"
	control := dialRelay(t, server, serverID, RoleServer, 2, "")
	assertControlMessage(t, control, map[string]any{"type": "sync"})
	if !relay.testMoveOwner(serverID) {
		t.Fatal("owner was not found")
	}
	late := dialRelay(t, server, serverID, RoleClient, 2, "late")
	assertRelayClose(t, late, websocket.StatusServiceRestart, "Session expired")
}

func TestRelayKeepsIdleWebSocketOpenPastDefaultAdapterTimeout(t *testing.T) {
	config := DefaultConfig()
	config.HTTPIdleTimeoutMS = 50
	server := newRelayTestServer(t, config)
	conn := dialRelay(t, server, "idle", RoleServer, 1, "")
	// coder/websocket.Ping waits for a pong to be consumed by a concurrent
	// Reader. CloseRead provides that read pump while keeping this socket idle
	// with respect to application messages.
	_ = conn.CloseRead(context.Background())
	time.Sleep(100 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), relayTestTimeout)
	defer cancel()
	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("idle websocket expired: %v", err)
	}
}

func TestRelayV2ClosesDataWithLastClientAndNotifiesControl(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	serverID := "v2-client-close"
	control := dialRelay(t, server, serverID, RoleServer, 2, "")
	assertControlMessage(t, control, map[string]any{"type": "sync"})
	client := dialRelay(t, server, serverID, RoleClient, 2, "closing")
	assertControlMessage(t, control, map[string]any{"type": "connected", "connectionId": "closing"})
	data := dialRelay(t, server, serverID, RoleServer, 2, "closing")
	if err := client.Close(websocket.StatusNormalClosure, ""); err != nil {
		t.Fatal(err)
	}
	assertRelayClose(t, data, websocket.StatusGoingAway, "Client disconnected")
	assertControlMessage(t, control, map[string]any{"type": "disconnected", "connectionId": "closing"})
}

func TestRelayV2ReplacesDuplicateDaemonDataWithoutDisconnectingClient(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	serverID := "v2-data-replace"
	client := dialRelay(t, server, serverID, RoleClient, 2, "replace")
	original := dialRelay(t, server, serverID, RoleServer, 2, "replace")
	replacement := dialRelay(t, server, serverID, RoleServer, 2, "replace")
	assertRelayClose(t, original, websocket.StatusPolicyViolation, "Replaced by new connection")
	writeRelayMessage(t, replacement, websocket.MessageText, []byte("still-routed"))
	assertRelayMessage(t, client, websocket.MessageText, []byte("still-routed"))
}

func TestRelayV1ValidatesHandshakeRetriesBeforeForwarding(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	serverID := "v1-handshake-retry"
	daemon := dialRelay(t, server, serverID, RoleServer, 1, "")
	client := dialRelay(t, server, serverID, RoleClient, 1, "")
	hello := handshakePayload(t, "e2ee_hello", validPublicKey(t))
	writeRelayMessage(t, client, websocket.MessageText, hello)
	assertRelayMessage(t, daemon, websocket.MessageText, hello)
	writeRelayMessage(t, client, websocket.MessageText, hello)
	assertRelayMessage(t, daemon, websocket.MessageText, hello)
	invalid := handshakePayload(t, "e2ee_hello", make([]byte, 32))
	writeRelayMessage(t, client, websocket.MessageText, invalid)
	assertRelayClose(t, client, websocket.StatusPolicyViolation, "Invalid handshake key")
}

func TestRelayV2ValidatesLegacyBinaryHandshakeWithoutBreakingCiphertext(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	serverID := "v2-binary-handshake"
	control := dialRelay(t, server, serverID, RoleServer, 2, "")
	assertControlMessage(t, control, map[string]any{"type": "sync"})
	client := dialRelay(t, server, serverID, RoleClient, 2, "binary")
	assertControlMessage(t, control, map[string]any{"type": "connected"})
	data := dialRelay(t, server, serverID, RoleServer, 2, "binary")
	hello := handshakePayload(t, "hello", validPublicKey(t))
	ciphertext := make([]byte, 48)
	for i := range ciphertext {
		ciphertext[i] = 0xa5
	}
	writeRelayMessage(t, client, websocket.MessageBinary, hello)
	writeRelayMessage(t, client, websocket.MessageBinary, ciphertext)
	assertRelayMessage(t, data, websocket.MessageBinary, hello)
	assertRelayMessage(t, data, websocket.MessageBinary, ciphertext)
}

func httptestServerForRelay(t *testing.T, relay *Relay) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(relay.Handler())
	t.Cleanup(server.Close)
	return server
}

func validPublicKey(t *testing.T) []byte {
	t.Helper()
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key.PublicKey().Bytes()
}

func handshakePayload(t *testing.T, messageType string, key []byte) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"type":         messageType,
		"key":          base64.StdEncoding.EncodeToString(key),
		"capabilities": map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
