package truenas

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestDialAuthenticatesAndCalls(t *testing.T) {
	server := newTLSServer(t, func(conn *websocket.Conn) {
		auth := readRequest(t, conn)
		if auth.Method != "auth.login_ex" {
			t.Errorf("authentication method = %q", auth.Method)
		}
		assertAPIKeyLogin(t, auth, "admin", "1-secret")
		writeJSON(t, conn, map[string]any{
			"jsonrpc": "2.0",
			"id":      auth.ID,
			"result":  map[string]any{"response_type": "SUCCESS", "user_info": nil},
		})
		writeJSON(t, conn, map[string]any{
			"jsonrpc": "2.0",
			"method":  "collection_update",
			"params": map[string]any{
				"collection": "core.get_jobs",
				"msg":        "changed",
			},
		})

		call := readRequest(t, conn)
		if call.Method != "system.info" {
			t.Errorf("call method = %q", call.Method)
		}
		writeJSON(t, conn, map[string]any{
			"jsonrpc": "2.0",
			"id":      call.ID,
			"result":  map[string]any{"version": "TrueNAS-25.10.5"},
		})
	})
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := Dial(ctx, Config{
		Endpoint:   strings.Replace(server.URL, "https://", "wss://", 1),
		Username:   "admin",
		APIKey:     "1-secret",
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer client.Close()

	var info struct {
		Version string `json:"version"`
	}
	if err := client.Call(ctx, "system.info", nil, &info); err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if info.Version != "TrueNAS-25.10.5" {
		t.Fatalf("version = %q", info.Version)
	}

	select {
	case notification := <-client.Notifications():
		if notification.Method != "collection_update" {
			t.Fatalf("notification method = %q", notification.Method)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for notification")
	}
}

func TestCallAcceptsResponseLargerThanWebSocketDefault(t *testing.T) {
	const largeValueSize = 64 << 10
	server := newTLSServer(t, func(conn *websocket.Conn) {
		call := readRequest(t, conn)
		writeJSON(t, conn, map[string]any{
			"jsonrpc": "2.0",
			"id":      call.ID,
			"result":  strings.Repeat("x", largeValueSize),
		})
	})
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := Dial(ctx, Config{
		Endpoint:         strings.Replace(server.URL, "https://", "wss://", 1),
		HTTPClient:       server.Client(),
		DisableReconnect: true,
	})
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer client.Close()

	var result string
	if err := client.Call(ctx, "user.query", nil, &result); err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if len(result) != largeValueSize {
		t.Fatalf("result length = %d, want %d", len(result), largeValueSize)
	}
}

func TestDialAllowsPrivateTLSCertificate(t *testing.T) {
	server := newTLSServer(t, func(conn *websocket.Conn) {})
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := Dial(ctx, Config{
		Endpoint:           strings.Replace(server.URL, "https://", "wss://", 1),
		InsecureSkipVerify: true,
	})
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer client.Close()
}

func TestDialAuthenticatesWithPassword(t *testing.T) {
	server := newTLSServer(t, func(conn *websocket.Conn) {
		auth := readRequest(t, conn)
		if auth.Method != "auth.login_ex" {
			t.Errorf("authentication method = %q", auth.Method)
		}
		assertPasswordLogin(t, auth, "admin", "password with spaces ")
		writeJSON(t, conn, map[string]any{
			"jsonrpc": "2.0",
			"id":      auth.ID,
			"result":  map[string]any{"response_type": "SUCCESS", "user_info": nil},
		})
	})
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := Dial(ctx, Config{
		Endpoint:   strings.Replace(server.URL, "https://", "wss://", 1),
		Username:   "admin",
		Password:   "password with spaces ",
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer client.Close()
}

func TestDialContinuesPasswordAuthenticationWithOTP(t *testing.T) {
	server := newTLSServer(t, func(conn *websocket.Conn) {
		auth := readRequest(t, conn)
		assertPasswordLogin(t, auth, "admin", "password")
		writeJSON(t, conn, map[string]any{
			"jsonrpc": "2.0", "id": auth.ID,
			"result": map[string]any{"response_type": "OTP_REQUIRED"},
		})

		continuation := readRequest(t, conn)
		if continuation.Method != "auth.login_ex_continue" || len(continuation.Params) != 1 {
			t.Fatalf("continuation = %#v", continuation)
		}
		data, ok := continuation.Params[0].(map[string]any)
		if !ok || data["mechanism"] != "OTP_TOKEN" || data["otp_token"] != "123456" {
			t.Fatalf("continuation params = %#v", continuation.Params)
		}
		writeJSON(t, conn, map[string]any{
			"jsonrpc": "2.0", "id": continuation.ID,
			"result": map[string]any{"response_type": "SUCCESS", "user_info": nil},
		})
	})
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := Dial(ctx, Config{
		Endpoint: strings.Replace(server.URL, "https://", "wss://", 1),
		Username: "admin", Password: "password", OTP: "123456",
		HTTPClient: server.Client(), DisableReconnect: true,
	})
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer client.Close()
}

func TestCallRetriesOverloadError(t *testing.T) {
	server := newTLSServer(t, func(conn *websocket.Conn) {
		first := readRequest(t, conn)
		writeJSON(t, conn, map[string]any{
			"jsonrpc": "2.0", "id": first.ID,
			"error": map[string]any{"code": -32000, "message": "too many concurrent calls"},
		})
		second := readRequest(t, conn)
		writeJSON(t, conn, map[string]any{
			"jsonrpc": "2.0", "id": second.ID, "result": "pong",
		})
	})
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := Dial(ctx, Config{
		Endpoint:   strings.Replace(server.URL, "https://", "wss://", 1),
		HTTPClient: server.Client(), BusyRetryDelay: time.Millisecond,
		DisableReconnect: true,
	})
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer client.Close()
	var pong string
	if err := client.Call(ctx, "core.ping", nil, &pong); err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if pong != "pong" {
		t.Fatalf("result = %q", pong)
	}
}

func TestSubscribeAndUnsubscribe(t *testing.T) {
	server := newTLSServer(t, func(conn *websocket.Conn) {
		subscribe := readRequest(t, conn)
		if subscribe.Method != "core.subscribe" || len(subscribe.Params) != 1 || subscribe.Params[0] != "pool.query" {
			t.Fatalf("subscribe = %#v", subscribe)
		}
		writeJSON(t, conn, map[string]any{
			"jsonrpc": "2.0", "id": subscribe.ID, "result": "subscription-1",
		})
		unsubscribe := readRequest(t, conn)
		if unsubscribe.Method != "core.unsubscribe" || len(unsubscribe.Params) != 1 || unsubscribe.Params[0] != "subscription-1" {
			t.Fatalf("unsubscribe = %#v", unsubscribe)
		}
		writeJSON(t, conn, map[string]any{
			"jsonrpc": "2.0", "id": unsubscribe.ID, "result": nil,
		})
	})
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := Dial(ctx, Config{
		Endpoint:   strings.Replace(server.URL, "https://", "wss://", 1),
		HTTPClient: server.Client(), DisableReconnect: true,
	})
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer client.Close()
	subscription, err := client.Subscribe(ctx, "pool.query")
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	if subscription.ID() != "subscription-1" {
		t.Fatalf("subscription ID = %q", subscription.ID())
	}
	if err := client.Unsubscribe(ctx, subscription); err != nil {
		t.Fatalf("Unsubscribe() error = %v", err)
	}
}

func TestReconnectRestoresSubscription(t *testing.T) {
	var connections atomic.Int32
	restored := make(chan struct{})
	server := newTLSServer(t, func(conn *websocket.Conn) {
		connectionNumber := connections.Add(1)
		auth := readRequest(t, conn)
		assertAPIKeyLogin(t, auth, "admin", "1-secret")
		writeJSON(t, conn, map[string]any{
			"jsonrpc": "2.0", "id": auth.ID,
			"result": map[string]any{"response_type": "SUCCESS", "user_info": nil},
		})
		switch connectionNumber {
		case 1:
			subscribe := readRequest(t, conn)
			writeJSON(t, conn, map[string]any{
				"jsonrpc": "2.0", "id": subscribe.ID, "result": "subscription-1",
			})
			time.Sleep(10 * time.Millisecond)
			conn.CloseNow()
		case 2:
			subscribe := readRequest(t, conn)
			if subscribe.Method != "core.subscribe" || subscribe.Params[0] != "pool.query" {
				t.Errorf("restored subscription = %#v", subscribe)
			}
			writeJSON(t, conn, map[string]any{
				"jsonrpc": "2.0", "id": subscribe.ID, "result": "subscription-2",
			})
			close(restored)
			ping := readRequest(t, conn)
			writeJSON(t, conn, map[string]any{
				"jsonrpc": "2.0", "id": ping.ID, "result": "pong",
			})
		}
	})
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := Dial(ctx, Config{
		Endpoint: strings.Replace(server.URL, "https://", "wss://", 1),
		Username: "admin", APIKey: "1-secret",
		HTTPClient: server.Client(), ReconnectDelay: 25 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer client.Close()
	subscription, err := client.Subscribe(ctx, "pool.query")
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	select {
	case <-restored:
	case <-ctx.Done():
		t.Fatal("timed out waiting for subscription restoration")
	}
	for subscription.ID() != "subscription-2" {
		select {
		case <-time.After(time.Millisecond):
		case <-ctx.Done():
			t.Fatalf("restored subscription ID = %q", subscription.ID())
		}
	}
	var pong string
	if err := client.Call(ctx, "core.ping", nil, &pong); err != nil || pong != "pong" {
		t.Fatalf("Call() after reconnect = %q, %v", pong, err)
	}
}

func TestCallReturnsRPCError(t *testing.T) {
	server := newTLSServer(t, func(conn *websocket.Conn) {
		call := readRequest(t, conn)
		writeJSON(t, conn, map[string]any{
			"jsonrpc": "2.0",
			"id":      call.ID,
			"error": map[string]any{
				"code":    -32001,
				"message": "method call error",
				"data": map[string]any{
					"errname": "EACCES",
					"reason":  "permission denied",
				},
			},
		})
	})
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := Dial(ctx, Config{
		Endpoint:   strings.Replace(server.URL, "https://", "wss://", 1),
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer client.Close()

	err = client.Call(ctx, "pool.query", nil, nil)
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("Call() error = %T %v, want *RPCError", err, err)
	}
	if rpcErr.Code != -32001 || rpcErr.Data == nil || rpcErr.Data.ErrName != "EACCES" {
		t.Fatalf("RPC error = %#v", rpcErr)
	}
}

func newTLSServer(t *testing.T, serve func(*websocket.Conn)) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("websocket.Accept() error = %v", err)
			return
		}
		defer conn.CloseNow()
		serve(conn)
	}))
	t.Cleanup(server.Close)
	return server
}

func readRequest(t *testing.T, conn *websocket.Conn) request {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("conn.Read() error = %v", err)
	}
	var req request
	if err := json.Unmarshal(payload, &req); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return req
}

func writeJSON(t *testing.T, conn *websocket.Conn, value any) {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
		t.Fatalf("conn.Write() error = %v", err)
	}
}

func assertAPIKeyLogin(t *testing.T, req request, username, apiKey string) {
	t.Helper()
	if len(req.Params) != 1 {
		t.Fatalf("login params = %#v", req.Params)
	}
	login, ok := req.Params[0].(map[string]any)
	if !ok || login["mechanism"] != "API_KEY_PLAIN" || login["username"] != username || login["api_key"] != apiKey {
		t.Fatalf("login params = %#v", req.Params)
	}
}

func assertPasswordLogin(t *testing.T, req request, username, password string) {
	t.Helper()
	if len(req.Params) != 1 {
		t.Fatalf("login params = %#v", req.Params)
	}
	login, ok := req.Params[0].(map[string]any)
	if !ok || login["mechanism"] != "PASSWORD_PLAIN" || login["username"] != username || login["password"] != password {
		t.Fatalf("login params = %#v", req.Params)
	}
}
