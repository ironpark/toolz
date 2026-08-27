package main

// Regression tests for behavioral gaps between the Go relay and the Elixir
// reference (references/paseo-relay). Each test asserts the reference
// behavior, with the reference source cited; they are written ahead of the
// fixes and fail until the corresponding behavior is ported.
import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/websocket"
)

// dialRelayRawQuery dials /ws with a caller-controlled raw query string, which
// the duplicate-parameter cases need because url.Values cannot express
// parameter order.
func dialRelayRawQuery(t *testing.T, server *httptest.Server, rawQuery string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), relayTestTimeout)
	defer cancel()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?" + rawQuery
	conn, response, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		if response != nil {
			t.Fatalf("dial %s: status=%d err=%v", rawQuery, response.StatusCode, err)
		}
		t.Fatalf("dial %s: %v", rawQuery, err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn
}

func relaySessionExists(relay *Relay, serverID string) bool {
	relay.mu.Lock()
	defer relay.mu.Unlock()
	return relay.sessions[serverID] != nil
}

// exhaustIngressBudget makes every subsequent ingress reservation fail, the
// same observable state as a node whose weighted budget is fully reserved.
func exhaustIngressBudget(relay *Relay) {
	relay.ingressReserved.Store(int64(relay.Config.IngressBudgetBytes))
}

// The reference builds its query map with :cowboy_req.parse_qs |> Map.new, so
// the LAST occurrence of a duplicated parameter wins (socket.ex:296-306;
// docs/PROTOCOL.md documents this as contract). url.Values.Get returns the
// first occurrence, silently routing to the wrong session.
func TestDuplicateQueryParameterLastValueWins(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	server := httptestServerForRelay(t, relay)
	first := strings.ReplaceAll(t.Name(), "/", "-") + "-first"
	second := strings.ReplaceAll(t.Name(), "/", "-") + "-second"
	dialRelayRawQuery(t, server, "serverId="+first+"&serverId="+second+"&role=server")
	if relaySessionExists(relay, first) {
		t.Errorf("session %q exists, want the first serverId value ignored", first)
	}
	if !relaySessionExists(relay, second) {
		t.Errorf("session %q missing, want the last serverId value to win", second)
	}
}

// The reference admits every control-socket text frame against the weighted
// ingress budget before interpreting it (socket.ex handle_control_input →
// Capacity.admit_message), so a ping on an exhausted node closes with
// 1013 Relay ingress capacity instead of answering with a pong.
func TestControlPingConsumesIngressBudget(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	server := httptestServerForRelay(t, relay)
	serverID := strings.ReplaceAll(t.Name(), "/", "-")
	control := dialRelay(t, server, serverID, RoleServer, 2, "")
	assertControlMessage(t, control, map[string]any{"type": "sync"})
	exhaustIngressBudget(relay)
	writeRelayMessage(t, control, websocket.MessageText, []byte(`{"type":"ping"}`))
	assertRelayClose(t, control, websocket.StatusTryAgainLater, "Relay ingress capacity")
}

// The reference reserves the ingress budget before handshake validation runs
// (socket.ex:102-112 admit_message → admit_input → HandshakeValidation.check),
// so an invalid hello on an exhausted node reports 1013 Relay ingress
// capacity, never reaching the 1008 Invalid handshake key path.
func TestIngressExhaustionPrecedesHandshakeRejection(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	server := httptestServerForRelay(t, relay)
	serverID := strings.ReplaceAll(t.Name(), "/", "-")
	_ = dialRelay(t, server, serverID, RoleServer, 1, "")
	client := dialRelay(t, server, serverID, RoleClient, 1, "")
	exhaustIngressBudget(relay)
	writeRelayMessage(t, client, websocket.MessageText, []byte(`{"type":"hello","key":"not-a-key"}`))
	assertRelayClose(t, client, websocket.StatusTryAgainLater, "Relay ingress capacity")
}

// Every control write in the reference goes through Writer.start_control,
// which increments frames_forwarded and bytes_forwarded (delivery/writer.ex),
// so the initial sync and each connected/disconnected roster notification
// count as forwarded frames.
func TestControlNotificationsCountAsForwardedFrames(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	server := httptestServerForRelay(t, relay)
	serverID := strings.ReplaceAll(t.Name(), "/", "-")
	control := dialRelay(t, server, serverID, RoleServer, 2, "")
	assertControlMessage(t, control, map[string]any{"type": "sync"})
	client := dialRelay(t, server, serverID, RoleClient, 2, "conn-metrics")
	assertControlMessage(t, control, map[string]any{"type": "connected", "connectionId": "conn-metrics"})
	_ = client

	wantBytes := int64(len(syncFrame(nil)) + len(connectedFrame("conn-metrics")))
	_ = waitScenario(func() bool {
		return relay.framesForwarded.Load() == 2 && relay.bytesForwarded.Load() == wantBytes
	}, relayTestTimeout)
	if frames, bytes := relay.framesForwarded.Load(), relay.bytesForwarded.Load(); frames != 2 || bytes != wantBytes {
		t.Fatalf("forwarded = (%d frames, %d bytes), want (2, %d): control notifications must count", frames, bytes, wantBytes)
	}
}

// A control text frame that is not a ping has no destination: the reference
// finishes its capacity token while still :reserved, so start_delivery never
// runs and the delivery-wait histogram, inflight bytes, and backpressured
// gauge are untouched (socket.ex handle_control_input, capacity.ex
// finish_message).
func TestNonPingControlFrameDoesNotObserveDeliveryWait(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	server := httptestServerForRelay(t, relay)
	serverID := strings.ReplaceAll(t.Name(), "/", "-")
	control := dialRelay(t, server, serverID, RoleServer, 2, "")
	assertControlMessage(t, control, map[string]any{"type": "sync"})
	writeRelayMessage(t, control, websocket.MessageText, []byte(`{"type":"nudge"}`))
	// The read loop is serial, so a pong proves the nudge frame finished.
	writeRelayMessage(t, control, websocket.MessageText, []byte(`{"type":"ping"}`))
	assertControlMessage(t, control, map[string]any{"type": "pong"})
	if count := relay.deliveryWaitCount.Load(); count != 0 {
		t.Fatalf("delivery_wait_seconds count = %d, want 0: routeless control frames must not observe delivery", count)
	}
}

// The reference owner process outlives its last socket by a 30s idle window
// (ownership.ex @idle_ms), keeping the serverId pinned to this node for quick
// reconnects; ownership must not be released the instant the session empties.
func TestSessionOwnershipOutlivesLastSocketDetach(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	server := httptestServerForRelay(t, relay)
	serverID := strings.ReplaceAll(t.Name(), "/", "-")
	daemon := dialRelay(t, server, serverID, RoleServer, 1, "")
	if err := daemon.Close(websocket.StatusNormalClosure, ""); err != nil {
		t.Fatalf("close daemon: %v", err)
	}
	eventually(t, relayTestTimeout, func() bool { return !relaySessionExists(relay, serverID) })
	_, found, err := relay.ownership.lookup(serverID)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if !found {
		t.Fatal("ownership released immediately on detach, want an idle hold as in the reference")
	}
}

// The reference replies to a bad query with the bare error string and no
// extra headers (socket.ex:56-57). http.Error appends a newline and a
// nosniff header, which byte-diffs against the reference.
func TestBadQueryResponseBodyMatchesReference(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	request, err := http.NewRequest(http.MethodGet, server.URL+"/ws?role=server", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "websocket")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body := make([]byte, 256)
	n, _ := response.Body.Read(body)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.StatusCode)
	}
	if got := string(body[:n]); got != "Missing serverId parameter" {
		t.Errorf("body = %q, want %q with no trailing newline", got, "Missing serverId parameter")
	}
	if nosniff := response.Header.Get("X-Content-Type-Options"); nosniff != "" {
		t.Errorf("X-Content-Type-Options = %q, want the header absent as in the reference", nosniff)
	}
}
