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

func TestRelayV2ControlFailsClosedWhenOwnerStallsDuringPing(t *testing.T) {
	relay := NewRelay(DefaultConfig())
	server := httptestServerForRelay(t, relay)
	serverID := "v2-stalled-owner"
	control := dialRelay(t, server, serverID, RoleServer, 2, "")
	assertControlMessage(t, control, map[string]any{"type": "sync"})
	resume, ok := requireRelayFaultController(t, relay).testStallOwner(serverID)
	if !ok {
		t.Fatal("owner was not found")
	}
	defer resume()
	writeRelayMessage(t, control, websocket.MessageText, []byte(`{"type":"ping"}`))
	assertRelayClose(t, control, websocket.StatusTryAgainLater, "Delivery unavailable")
}

func TestRelayV2ResetsUnresponsiveControlAfterNudgingDataAttachment(t *testing.T) {
	config := DefaultConfig()
	config.DataAttachTimeoutMS = 50
	server := newRelayTestServer(t, config)
	serverID := "v2-control-watchdog"
	control := dialRelay(t, server, serverID, RoleServer, 2, "")
	assertControlMessage(t, control, map[string]any{"type": "sync"})
	_ = dialRelay(t, server, serverID, RoleClient, 2, "waiting")
	assertControlMessage(t, control, map[string]any{"type": "connected", "connectionId": "waiting"})
	assertControlMessage(t, control, map[string]any{"type": "sync", "connectionIds": []string{"waiting"}})
	assertRelayClose(t, control, websocket.StatusInternalError, "Control unresponsive")
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
	if !requireRelayFaultController(t, relay).testKillOwner(serverID) {
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
	if !requireRelayFaultController(t, relay).testMoveOwner(serverID) {
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
