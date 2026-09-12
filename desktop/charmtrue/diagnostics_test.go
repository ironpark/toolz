package main

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

type diagnosticCaller struct {
	data        string
	queryError  error
	updateError error
	queries     int
	updates     int
	params      []any
}

func (c *diagnosticCaller) Call(_ context.Context, method string, params []any, result any) error {
	if method == "pool.dataset.query" {
		c.queries++
		if c.queryError != nil {
			return c.queryError
		}
		return json.Unmarshal([]byte(c.data), result)
	}
	if method != "pool.dataset.update" {
		return errors.New("unexpected method")
	}
	c.updates++
	c.params = params
	return c.updateError
}

func TestDiagnosticTuning(t *testing.T) {
	for _, tc := range []struct{ action, property, value string }{{"compression_lz4", "compression", "LZ4"}, {"sync_standard", "sync", "STANDARD"}} {
		t.Run(tc.action, func(t *testing.T) {
			client := &diagnosticCaller{data: `{"id":"tank/data","type":"FILESYSTEM","compression":{"value":"off"},"sync":{"value":"disabled"}}`}
			if err := applyDiagnosticTuning(context.Background(), client, "tank/data", tc.action); err != nil {
				t.Fatal(err)
			}
			want := []any{"tank/data", map[string]any{tc.property: tc.value}}
			if !reflect.DeepEqual(client.params, want) || client.queries != 1 || client.updates != 1 {
				t.Fatalf("unexpected calls: %+v", client)
			}
		})
	}
}

func TestDiagnosticTuningRejectsUnsafeOrStaleTargets(t *testing.T) {
	for _, tc := range []struct{ target, action, data string }{
		{"tank", "compression_lz4", `{}`},
		{"tank/.system/data", "compression_lz4", `{}`},
		{"tank/ix-apps/data", "compression_lz4", `{}`},
		{"tank/data", "arbitrary", `{}`},
		{"tank/data", "compression_lz4", `{"id":"tank/other","type":"FILESYSTEM","compression":{"value":"off"}}`},
		{"tank/data", "compression_lz4", `{"id":"tank/data","type":"VOLUME","compression":{"value":"off"}}`},
		{"tank/data", "compression_lz4", `{"id":"tank/data","type":"FILESYSTEM","locked":true,"compression":{"value":"off"}}`},
		{"tank/data", "compression_lz4", `{"id":"tank/data","type":"FILESYSTEM","compression":{"value":"lz4"}}`},
		{"tank/data", "sync_standard", `{"id":"tank/data","type":"FILESYSTEM","sync":{"value":"always"}}`},
	} {
		client := &diagnosticCaller{data: tc.data}
		if err := applyDiagnosticTuning(context.Background(), client, tc.target, tc.action); err == nil {
			t.Errorf("expected rejection: %+v", tc)
		}
		if client.updates != 0 {
			t.Errorf("rejected target was updated: %+v", tc)
		}
	}
}

func TestDiagnosticTuningAPIFailures(t *testing.T) {
	failure := errors.New("permission denied")
	for _, client := range []*diagnosticCaller{
		{queryError: failure},
		{data: `{"id":"tank/data","type":"FILESYSTEM","compression":{"value":"off"}}`, updateError: failure},
	} {
		if err := applyDiagnosticTuning(context.Background(), client, "tank/data", "compression_lz4"); !errors.Is(err, failure) {
			t.Fatalf("error = %v", err)
		}
		if client.queryError != nil && client.updates != 0 {
			t.Fatal("updated after failed query")
		}
	}
}
