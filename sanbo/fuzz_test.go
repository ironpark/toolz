package main

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"testing"
)

// FuzzParseConnectionQuery checks that the public /ws query contract never
// panics and that every accepted Connection satisfies the invariants the relay
// relies on when it registers a peer.
func FuzzParseConnectionQuery(f *testing.F) {
	f.Add("local", "server", "", "")
	f.Add("local", "client", "2", "")
	f.Add("local", "client", "2", "conn_1")
	f.Add("local", "server", "2", "conn_1")
	f.Add("", "server", "1", "")
	f.Add("local", "SERVER", " 2 ", "  ")
	f.Add("local", "client", "3", "")

	f.Fuzz(func(t *testing.T, serverID, role, version, connectionID string) {
		connection, err := ParseConnectionQuery(map[string]string{
			"serverId":     serverID,
			"role":         role,
			"v":            version,
			"connectionId": connectionID,
		})
		if err != nil {
			if connection != (Connection{}) {
				t.Fatalf("rejected query returned a non-zero connection: %+v", connection)
			}
			return
		}

		if connection.Role != RoleServer && connection.Role != RoleClient {
			t.Fatalf("accepted unknown role %q", connection.Role)
		}
		if connection.Version != 1 && connection.Version != 2 {
			t.Fatalf("accepted unknown version %d", connection.Version)
		}
		if connection.ServerID == "" || len(connection.ServerID) > maximumRouteIDBytes {
			t.Fatalf("accepted serverId of length %d", len(connection.ServerID))
		}
		if len(connection.ConnectionID) > maximumRouteIDBytes {
			t.Fatalf("accepted connectionId of length %d", len(connection.ConnectionID))
		}
		if connection.Version == 1 && connection.ConnectionID != "" {
			t.Fatalf("v1 connection carries connectionId %q", connection.ConnectionID)
		}
		// A v2 client is always addressable: the relay indexes s.clients by this key.
		if connection.Version == 2 && connection.Role == RoleClient && connection.ConnectionID == "" {
			t.Fatal("v2 client was accepted without a connectionId")
		}

		// Re-parsing an accepted connection must be a fixed point, otherwise a
		// generated connectionId would drift on every reconnect.
		again, err := ParseConnectionQuery(map[string]string{
			"serverId":     connection.ServerID,
			"role":         string(connection.Role),
			"v":            strconv.Itoa(connection.Version),
			"connectionId": connection.ConnectionID,
		})
		if err != nil {
			t.Fatalf("re-parsing an accepted connection failed: %v", err)
		}
		if again != connection {
			t.Fatalf("re-parse drifted: %+v -> %+v", connection, again)
		}
	})
}

// FuzzRelayWebSocketURLRoundTrip checks that identifiers survive the URL
// encoding the test helpers and real clients use to reach ParseConnectionQuery.
func FuzzRelayWebSocketURLRoundTrip(f *testing.F) {
	f.Add("local", "conn_1")
	f.Add("server id/with?chars#", "conn=1&role=server")
	f.Add("서버", "연결")

	f.Fuzz(func(t *testing.T, serverID, connectionID string) {
		if serverID == "" || len(serverID) > maximumRouteIDBytes || len(connectionID) > maximumRouteIDBytes {
			return
		}
		query := url.Values{
			"serverId":     {serverID},
			"role":         {string(RoleClient)},
			"v":            {"2"},
			"connectionId": {connectionID},
		}
		decoded, err := url.ParseQuery(query.Encode())
		if err != nil {
			t.Fatalf("re-parsing an encoded query failed: %v", err)
		}
		connection, err := ParseConnectionQuery(map[string]string{
			"serverId":     decoded.Get("serverId"),
			"role":         decoded.Get("role"),
			"v":            decoded.Get("v"),
			"connectionId": decoded.Get("connectionId"),
		})
		if err != nil {
			t.Fatalf("encoding a valid query made it unparseable: %v", err)
		}
		if connection.ServerID != serverID {
			t.Fatalf("serverId changed across the URL: %q -> %q", serverID, connection.ServerID)
		}
	})
}

// FuzzValidHandshake checks that arbitrary frames never panic the read loop and
// that only real handshakes are ever inspected for a key.
func FuzzValidHandshake(f *testing.F) {
	f.Add([]byte(`{"type":"ping"}`))
	f.Add([]byte(`{"type":"hello","key":"` + base64.StdEncoding.EncodeToString(make([]byte, 32)) + `"}`))
	f.Add([]byte(`{"type":"e2ee_hello"`))
	f.Add([]byte(`{"type":"e2ee_hello","key":null}`))
	f.Add([]byte("\xff\xff\xff\xff"))
	f.Add([]byte(`[]`))

	f.Fuzz(func(t *testing.T, payload []byte) {
		accepted := validHandshake(payload)
		if accepted != validHandshake(payload) {
			t.Fatal("validHandshake is not deterministic")
		}

		var frame struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(payload, &frame) != nil {
			return
		}
		// Well-formed non-handshake frames stay opaque to the relay.
		if frame.Type != "hello" && frame.Type != "e2ee_hello" && !accepted {
			t.Fatalf("rejected an opaque frame of type %q", frame.Type)
		}
	})
}

// FuzzValidHandshakeKey checks the key check itself: only canonical, non
// low-order X25519 public keys are admitted, for both handshake types.
func FuzzValidHandshakeKey(f *testing.F) {
	valid, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(true, valid.PublicKey().Bytes())
	f.Add(false, valid.PublicKey().Bytes())
	f.Add(true, make([]byte, 32))
	f.Add(true, make([]byte, 31))
	f.Add(true, []byte(nil))

	f.Fuzz(func(t *testing.T, e2ee bool, key []byte) {
		kind := "hello"
		if e2ee {
			kind = "e2ee_hello"
		}
		payload := []byte(fmt.Sprintf(`{"type":%q,"key":%q,"capabilities":{}}`, kind, base64.StdEncoding.EncodeToString(key)))
		accepted := validHandshake(payload)

		want := len(key) == 32 && key[31]&0x80 == 0
		if want {
			public, err := ecdh.X25519().NewPublicKey(key)
			if err != nil {
				want = false
			} else if _, err := valid.ECDH(public); err != nil {
				want = false
			}
		}
		if accepted != want {
			t.Fatalf("%s with key %x: accepted=%v want=%v", kind, key, accepted, want)
		}
	})
}

// FuzzLoadConfig checks that no environment can produce a Config that violates
// the bounds LoadConfig advertises.
func FuzzLoadConfig(f *testing.F) {
	f.Add("127.0.0.1", "4000", "false", "30000", "35000", "536870912")
	f.Add("::1", "0", "yes", "35000", "30000", "1")
	f.Add("localhost", "65535", "true", "100", "300000", "8589934592")
	f.Add("", "", "", "", "", "")

	f.Fuzz(func(t *testing.T, host, port, drain, delivery, transport, ingress string) {
		environment := map[string]string{
			"PASEO_RELAY_HOST":                      host,
			"PASEO_RELAY_PORT":                      port,
			"PASEO_RELAY_DRAIN":                     drain,
			"PASEO_RELAY_DELIVERY_TIMEOUT_MS":       delivery,
			"PASEO_RELAY_TRANSPORT_SEND_TIMEOUT_MS": transport,
			"PASEO_RELAY_INGRESS_BUDGET_BYTES":      ingress,
		}
		config, err := LoadConfig(environment)
		if err != nil {
			if config.Port != 0 || config.IP != nil {
				t.Fatal("rejected environment returned a partially populated config")
			}
			return
		}

		if config.IP == nil || config.IP.String() == "" {
			t.Fatalf("accepted host %q without a parsed IP", host)
		}
		if config.Port < 1 || config.Port > 65_535 {
			t.Fatalf("accepted port %d", config.Port)
		}
		if config.DeliveryTimeoutMS >= config.TransportSendTimeoutMS {
			t.Fatalf("accepted delivery %d >= transport %d", config.DeliveryTimeoutMS, config.TransportSendTimeoutMS)
		}
		if config.IngressBudgetBytes < MaximumMessagePayloadBytes*config.IngressWeight {
			t.Fatalf("accepted an ingress budget that cannot admit one maximum message")
		}
		// The relay derives listener and read limits from these, so a zero would
		// disable the corresponding protection outright.
		if config.Acceptors < 1 || config.ConnectionsPerAcceptor < 1 || config.TCPReceiveBufferBytes < 1 {
			t.Fatalf("accepted a config with a disabled capacity bound: %+v", config)
		}
	})
}
