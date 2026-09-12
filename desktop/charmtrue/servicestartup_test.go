package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type startupCaller struct {
	calls  int
	method string
	params []any
	err    error
}

func (c *startupCaller) Call(_ context.Context, method string, params []any, result any) error {
	c.calls++
	c.method, c.params = method, params
	return c.err
}

func TestServiceStartupOnlyUpdatesBootPolicy(t *testing.T) {
	for _, automatic := range []bool{true, false} {
		client := &startupCaller{}
		if err := setServiceStartup(context.Background(), client, " cifs ", automatic); err != nil {
			t.Fatal(err)
		}
		if client.calls != 1 || client.method != "service.update" || !reflect.DeepEqual(client.params, []any{"cifs", map[string]any{"enable": automatic}}) {
			t.Fatalf("unexpected service call: %+v", client)
		}
	}
}

func TestServiceStartupValidationAndFailure(t *testing.T) {
	client := &startupCaller{}
	if err := setServiceStartup(context.Background(), client, " ", true); err == nil || client.calls != 0 {
		t.Fatal("blank service must not send a request")
	}
	want := errors.New("permission denied")
	client.err = want
	if err := setServiceStartup(context.Background(), client, "ssh", false); !errors.Is(err, want) {
		t.Fatalf("error not propagated: %v", err)
	}
	service := &TrueNASService{}
	if err := service.SetServiceStartup("ssh", true); err == nil {
		t.Fatal("disconnected call must fail")
	}
}
