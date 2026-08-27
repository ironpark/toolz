package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

const relayTestTimeout = 300 * time.Millisecond

func newRelayTestServer(t *testing.T, config Config) *httptest.Server {
	t.Helper()
	return httptestServerForRelay(t, NewRelay(config))
}

// newControlWatchdogTestServer serves a relay whose control watchdog runs on
// millisecond stages instead of the production 10s/5s, so the always-on
// behavior is observable within a test timeout.
func newControlWatchdogTestServer(t *testing.T, config Config, sync, close time.Duration) *httptest.Server {
	t.Helper()
	relay := NewRelay(config)
	relay.controlSyncDelay = sync
	relay.controlCloseDelay = close
	return httptestServerForRelay(t, relay)
}

func getResponse(t *testing.T, url string) (int, http.Header, string) {
	t.Helper()
	response, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	return response.StatusCode, response.Header, string(body)
}

func relayWebSocketQuery(serverID string, role Role, version int, connectionID string) url.Values {
	query := url.Values{"serverId": {serverID}, "role": {string(role)}}
	if version == 2 {
		query.Set("v", "2")
		if connectionID != "" {
			query.Set("connectionId", connectionID)
		}
	}
	return query
}

func relayWebSocketURL(server *httptest.Server, serverID string, role Role, version int, connectionID string) string {
	return "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?" +
		relayWebSocketQuery(serverID, role, version, connectionID).Encode()
}

func dialRelay(t *testing.T, server *httptest.Server, serverID string, role Role, version int, connectionID string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), relayTestTimeout)
	defer cancel()
	conn, response, err := websocket.Dial(ctx, relayWebSocketURL(server, serverID, role, version, connectionID), nil)
	if err != nil {
		if response != nil {
			t.Fatalf("dial relay: status=%d err=%v", response.StatusCode, err)
		}
		t.Fatalf("dial relay: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn
}

// dialRelayExpectingStatus asserts that a relay upgrade is refused with the
// given HTTP status.
func dialRelayExpectingStatus(t *testing.T, server *httptest.Server, serverID string, role Role, version int, connectionID string, want int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), relayTestTimeout)
	defer cancel()
	conn, response, err := websocket.Dial(ctx, relayWebSocketURL(server, serverID, role, version, connectionID), nil)
	if conn != nil {
		_ = conn.CloseNow()
		t.Fatalf("dial %s: connection upgraded, want status %d", serverID, want)
	}
	if err == nil || response == nil || response.StatusCode != want {
		t.Fatalf("dial %s: response=%#v err=%v, want status %d", serverID, response, err, want)
	}
	if response.Body != nil {
		_ = response.Body.Close()
	}
}

func writeRelayMessage(t *testing.T, conn *websocket.Conn, messageType websocket.MessageType, payload []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), relayTestTimeout)
	defer cancel()
	if err := conn.Write(ctx, messageType, payload); err != nil {
		t.Fatalf("write relay message: %v", err)
	}
}

func readRelayMessage(t *testing.T, conn *websocket.Conn) (websocket.MessageType, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), relayTestTimeout)
	defer cancel()
	messageType, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read relay message: %v", err)
	}
	return messageType, payload
}

func assertRelayMessage(t *testing.T, conn *websocket.Conn, wantType websocket.MessageType, wantPayload []byte) {
	t.Helper()
	messageType, payload := readRelayMessage(t, conn)
	if messageType != wantType || string(payload) != string(wantPayload) {
		t.Fatalf("message = (%v, %q), want (%v, %q)", messageType, payload, wantType, wantPayload)
	}
}

func assertControlMessage(t *testing.T, conn *websocket.Conn, want map[string]any) map[string]any {
	t.Helper()
	messageType, payload := readRelayMessage(t, conn)
	if messageType != websocket.MessageText {
		t.Fatalf("control message type = %v, want text", messageType)
	}
	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode control message %q: %v", payload, err)
	}
	for key, value := range want {
		if !equalJSONValue(got[key], value) {
			t.Fatalf("control[%q] = %#v, want %#v; full=%#v", key, got[key], value, got)
		}
	}
	return got
}

func equalJSONValue(got, want any) bool {
	gotJSON, gotErr := json.Marshal(got)
	wantJSON, wantErr := json.Marshal(want)
	return gotErr == nil && wantErr == nil && string(gotJSON) == string(wantJSON)
}

func assertRelayClose(t *testing.T, conn *websocket.Conn, wantCode websocket.StatusCode, wantReason string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), relayTestTimeout)
	defer cancel()
	_, _, err := conn.Read(ctx)
	if err == nil {
		t.Fatal("connection remained open")
	}
	if websocket.CloseStatus(err) != wantCode {
		t.Fatalf("close status = %d, want %d: %v", websocket.CloseStatus(err), wantCode, err)
	}
	var closeError websocket.CloseError
	if !errors.As(err, &closeError) || closeError.Reason != wantReason {
		t.Fatalf("close reason = %q, want %q: %v", closeError.Reason, wantReason, err)
	}
}

func relayMetrics(t *testing.T, server *httptest.Server) string {
	t.Helper()
	status, _, body := getResponse(t, server.URL+"/metrics")
	if status != http.StatusOK {
		t.Fatalf("metrics status = %d", status)
	}
	return body
}

func metricValue(t *testing.T, metrics, name string) float64 {
	t.Helper()
	for _, line := range strings.Split(metrics, "\n") {
		if strings.HasPrefix(line, name+" ") {
			value, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(line, name)), 64)
			if err != nil {
				t.Fatalf("parse metric %q: %v", line, err)
			}
			return value
		}
	}
	t.Fatalf("metric %q not found", name)
	return 0
}

func eventually(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	if !waitScenario(condition, timeout) {
		t.Fatal("condition did not become true before timeout")
	}
}

// relayScenarioResult is the observable result of an internal failure scenario.
// Production remains free of test hooks; a compatibility implementation exposes
// these methods only to same-package tests when the behavior cannot be induced
// through the public socket boundary deterministically.
type relayScenarioResult struct {
	CloseCode               websocket.StatusCode
	CloseReason             string
	Forwarded               [][]byte
	ActiveWebSockets        int64
	ActiveSessions          int64
	IngressReservedBytes    int64
	InflightDeliveryBytes   int64
	BackpressuredSources    int64
	ConnectionRejections    int64
	FramesForwarded         int64
	BytesForwarded          int64
	OwnerTarget             string
	OwnerCount              int
	OpenedSockets           int
	CapacityEpochChanged    bool
	ListenerEpochChanged    bool
	AdmissionOpen           bool
	SourceBlocked           bool
	DestinationClosed       bool
	ConnectionStillAttached bool
	ControlSocketUsed       bool
	CleanupFailures         int
}

func requireRelayScenario(t *testing.T, relay *Relay, name string) relayScenarioResult {
	t.Helper()
	result, err := relay.testRunScenario(name)
	if err != nil {
		t.Fatalf("run relay scenario %q: %v", name, err)
	}
	return result
}

// relayClientPeers returns every client peer attached to a v2 route, which the
// fan-out cases need in order to block one destination at a time.
func relayClientPeers(relay *Relay, serverID, connectionID string) []*relayPeer {
	relay.mu.Lock()
	defer relay.mu.Unlock()
	s := relay.sessions[serverID]
	if s == nil {
		return nil
	}
	return append([]*relayPeer(nil), s.clients[connectionID]...)
}
