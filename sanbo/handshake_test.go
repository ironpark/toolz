package main

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/coder/websocket"
)

// Ported from test/paseo_relay/handshake_validation_test.exs.
func TestHandshakeAcceptsCanonicalKeysForHelloAndE2EEHello(t *testing.T) {
	for _, kind := range []string{"hello", "e2ee_hello"} {
		for _, opcode := range []websocket.MessageType{websocket.MessageText, websocket.MessageBinary} {
			t.Run(fmt.Sprintf("%s/%d", kind, opcode), func(t *testing.T) {
				assertHandshakeAccepted(t, opcode, handshakePayload(t, kind, validPublicKey(t)))
			})
		}
	}
}

func TestHandshakeRejectsUnsupportedPublicKey(t *testing.T) {
	assertHandshakeRejected(t, websocket.MessageText, handshakePayload(t, "e2ee_hello", make([]byte, 32)))
}

func TestHandshakeRejectsOtherUnsupportedPublicKeys(t *testing.T) {
	encodings := []string{
		"0100000000000000000000000000000000000000000000000000000000000000",
		"E0EB7A7C3B41B8AE1656E3FAF19FC46ADA098DEB9C32B1FD866205165F49B800",
		"5F9C95BCA3508C24B1D0B1559C83EF5B04445CC4581C8E86D8224EDDD09F1157",
		"ECFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF7F",
		"EDFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF7F",
		"EEFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF7F",
		// Coordinates at or above 2^255-19, which X25519 would silently reduce
		// onto a key the relay already accepts.
		"EFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF7F",
		"FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF7F",
	}
	for _, encoded := range encodings {
		key, err := hex.DecodeString(encoded)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(encoded[:4], func(t *testing.T) {
			assertHandshakeRejected(t, websocket.MessageBinary, handshakePayload(t, "hello", key))
		})
	}
}

func TestHandshakeRejectsMalformedOrWrongSizedKeyEncoding(t *testing.T) {
	canonical := base64.StdEncoding.EncodeToString(validPublicKey(t))
	invalidPadBits := base64.StdEncoding.EncodeToString(append([]byte{9}, make([]byte, 31)...))
	invalidPadBits = strings.TrimSuffix(invalidPadBits, "A=") + "B="
	invalid := []any{nil, 42, "not base64!", base64.StdEncoding.EncodeToString(make([]byte, 31)), base64.StdEncoding.EncodeToString(make([]byte, 33)), strings.TrimRight(canonical, "="), invalidPadBits}
	for i, key := range invalid {
		payload := []byte(fmt.Sprintf(`{"type":"e2ee_hello","key":%s,"capabilities":{}}`, mustJSON(t, key)))
		t.Run(fmt.Sprintf("invalid-%d", i), func(t *testing.T) {
			assertHandshakeRejected(t, websocket.MessageText, payload)
		})
	}
	assertHandshakeRejected(t, websocket.MessageText, []byte(`{"type":"e2ee_hello"}`))
}

func TestHandshakeRejectsNonCanonicalKeyEncoding(t *testing.T) {
	key := make([]byte, 32)
	key[0], key[31] = 9, 0x80
	assertHandshakeRejected(t, websocket.MessageBinary, handshakePayload(t, "hello", key))
}

func TestHandshakeLeavesNonHandshakeFramesOpaque(t *testing.T) {
	for i, test := range []struct {
		opcode  websocket.MessageType
		payload []byte
	}{
		{websocket.MessageBinary, []byte(strings.Repeat("\xff", 64))},
		{websocket.MessageText, []byte(`{"type":"ping"}`)},
		{websocket.MessageText, []byte(`{"type":"hello"`)},
	} {
		t.Run(fmt.Sprintf("opaque-%d", i), func(t *testing.T) { assertHandshakeAccepted(t, test.opcode, test.payload) })
	}
}

func TestHandshakeCountsAcceptedAndRejectedFramesWithoutRouteIdentifiers(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	metrics := relayMetrics(t, server)
	for _, outcome := range []string{"accepted", "rejected"} {
		for _, version := range []int{1, 2} {
			for _, kind := range []string{"hello", "e2ee_hello"} {
				name := fmt.Sprintf(`paseo_relay_handshake_%s_total{routing_version="v%d",type="%s"}`, outcome, version, kind)
				if !strings.Contains(metrics, name) {
					t.Errorf("metrics missing %s", name)
				}
			}
		}
	}
	if strings.Contains(metrics, "serverId") {
		t.Fatal("handshake metrics leak route identifiers")
	}
}

func assertHandshakeAccepted(t *testing.T, opcode websocket.MessageType, payload []byte) {
	t.Helper()
	server := newRelayTestServer(t, DefaultConfig())
	serverID := strings.ReplaceAll(t.Name(), "/", "-")
	daemon := dialRelay(t, server, serverID, RoleServer, 1, "")
	client := dialRelay(t, server, serverID, RoleClient, 1, "")
	writeRelayMessage(t, client, opcode, payload)
	assertRelayMessage(t, daemon, opcode, payload)
}

func assertHandshakeRejected(t *testing.T, opcode websocket.MessageType, payload []byte) {
	t.Helper()
	server := newRelayTestServer(t, DefaultConfig())
	serverID := strings.ReplaceAll(t.Name(), "/", "-")
	_ = dialRelay(t, server, serverID, RoleServer, 1, "")
	client := dialRelay(t, server, serverID, RoleClient, 1, "")
	writeRelayMessage(t, client, opcode, payload)
	assertRelayClose(t, client, websocket.StatusPolicyViolation, "Invalid handshake key")
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
