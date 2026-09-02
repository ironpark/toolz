package truenas

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestStorageManifestIsComplete(t *testing.T) {
	if got := len(StorageMethods); got != 108 {
		t.Fatalf("storage methods = %d, want 108", got)
	}
	seen := make(map[string]bool, len(StorageMethods))
	for _, method := range StorageMethods {
		if seen[method.Name] {
			t.Fatalf("duplicate storage method %q", method.Name)
		}
		seen[method.Name] = true
		if method.Service == "" || method.Kind == "" {
			t.Fatalf("incomplete metadata: %+v", method)
		}
	}
	if seen["pool.scrub"] {
		t.Fatal("namespace pool.scrub must not be callable")
	}
	for _, name := range []string{"device.get_info", "disk.wipe", "filesystem.setacl", "pool.create", "pool.dataset.query", "pool.scrub.run", "pool.snapshot.rollback", "pool.snapshottask.run", "zfs.resource.query"} {
		if !seen[name] {
			t.Errorf("missing %s", name)
		}
	}
}

func TestStorageRiskMetadata(t *testing.T) {
	for _, name := range []string{"disk.wipe", "pool.export", "pool.detach", "pool.dataset.change_key", "pool.snapshot.rollback", "filesystem.setacl"} {
		m, ok := StorageMethodByName(name)
		if !ok || !m.Destructive {
			t.Errorf("%s should be destructive: %+v", name, m)
		}
	}
	if m, _ := StorageMethodByName("pool.query"); m.Destructive {
		t.Fatal("pool.query should not be destructive")
	}
}

func TestDecodeStorageResult(t *testing.T) {
	got, err := DecodeStorageResult[PoolEntry](json.RawMessage(`{"id":7,"name":"tank","healthy":true}`))
	if err != nil || got.ID != 7 || got.Name != "tank" || !got.Healthy {
		t.Fatalf("decode = %+v, %v", got, err)
	}
}

func TestUnknownStorageMethodIsRejectedBeforeCall(t *testing.T) {
	s := DiskService{storageCaller{client: &Client{}}}
	_, err := s.Call(context.Background(), "not_real", StorageCall{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestStorageWrapperSendsMethodAndPositionalParameters(t *testing.T) {
	server := newTLSServer(t, func(conn *websocket.Conn) {
		req := readRequest(t, conn)
		if req.Method != "pool.query" {
			t.Errorf("method = %q", req.Method)
		}
		if len(req.Params) != 2 {
			t.Errorf("params = %#v", req.Params)
		}
		writeJSON(t, conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": []any{map[string]any{"id": 1, "name": "tank"}}})
	})
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := Dial(ctx, Config{Endpoint: strings.Replace(server.URL, "https://", "wss://", 1), HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	raw, err := client.Storage().Pools.Query(ctx, StorageCall{Params: []any{[]Filter{}, QueryOptions{}}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeStorageResult[[]PoolEntry](raw)
	if err != nil || len(got) != 1 || got[0].Name != "tank" {
		t.Fatalf("result=%+v err=%v", got, err)
	}
}
