package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/coder/websocket"
)

func TestHTTPSRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.CloseNow()
		auth := readServiceRequest(t, conn)
		writeServiceResponse(t, conn, auth.ID, map[string]any{"response_type": "SUCCESS"})
		info := readServiceRequest(t, conn)
		writeServiceResponse(t, conn, info.ID, map[string]any{"hostname": "vault"})
		for _, enabled := range []bool{true, false} {
			req := readServiceRequest(t, conn)
			if req.Method != "system.general.update" || len(req.Params) != 1 {
				t.Errorf("unexpected update: %#v", req)
				return
			}
			data := req.Params[0].(map[string]any)
			if len(data) != 2 || data["ui_httpsredirect"] != enabled || data["ui_restart_delay"] != float64(3) {
				t.Errorf("update params = %#v", data)
			}
			writeServiceResponse(t, conn, req.ID, map[string]any{"ui_httpsredirect": enabled})
			req = readServiceRequest(t, conn)
			if req.Method != "system.general.config" {
				t.Errorf("method = %q", req.Method)
			}
			writeServiceResponse(t, conn, req.ID, map[string]any{"ui_httpsredirect": enabled})
		}
		_, _, _ = conn.Read(context.Background())
	}))
	defer server.Close()
	service := &TrueNASService{profilesPath: t.TempDir() + "/servers.json"}
	if _, err := service.Connect(strings.Replace(server.URL, "http://", "ws://", 1), "admin", "password", "password", false, false, false); err != nil {
		t.Fatal(err)
	}
	defer service.Disconnect()
	for _, enabled := range []bool{true, false} {
		if err := service.SetHTTPSRedirect(enabled); err != nil {
			t.Fatal(err)
		}
		if got, err := service.HTTPSRedirect(); err != nil || got != enabled {
			t.Fatalf("HTTPSRedirect() = %v, %v; want %v", got, err, enabled)
		}
	}
}

func TestHTTPSRedirectDisconnected(t *testing.T) {
	service := &TrueNASService{}
	if _, err := service.HTTPSRedirect(); err == nil {
		t.Fatal("expected disconnected read to fail")
	}
	if err := service.SetHTTPSRedirect(true); err == nil {
		t.Fatal("expected disconnected update to fail")
	}
}
