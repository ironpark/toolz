package main

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
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

func TestRelayV2ControlPairsClientsAndFlushesBufferedFramesInOrder(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	serverID := "v2-buffered"
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
	relay := NewRelay(slowConsumerConfig())
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
	relay := NewRelay(slowConsumerConfig())
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
	relay := NewRelay(DefaultConfig())
	server := httptestServerForRelay(t, relay)
	serverID := "v2-owner-exit"
	control := dialRelay(t, server, serverID, RoleServer, 2, "")
	assertControlMessage(t, control, map[string]any{"type": "sync"})
	if !relay.testKillOwner(serverID) {
		t.Fatal("owner was not found")
	}
	assertRelayClose(t, control, websocket.StatusServiceRestart, "Session owner moved")
}

func TestRelayV2SocketInitializationFailsClosedWhenOwnerMoves(t *testing.T) {
	relay := NewRelay(DefaultConfig())
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
