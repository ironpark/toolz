package main

import (
	"strings"
	"testing"
)

func TestParseConnectionQueryDefaultsToV1(t *testing.T) {
	got, err := ParseConnectionQuery(map[string]string{"serverId": "srv_v1", "role": "server"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ServerID != "srv_v1" || got.Role != RoleServer || got.Version != 1 || got.ConnectionID != "" {
		t.Fatalf("connection = %#v", got)
	}
}

func TestParseConnectionQueryV2Roles(t *testing.T) {
	control, err := ParseConnectionQuery(map[string]string{"serverId": "srv", "role": "server", "v": "2"})
	if err != nil || control.ConnectionID != "" || control.Version != 2 {
		t.Fatalf("control = %#v, err %v", control, err)
	}

	data, err := ParseConnectionQuery(map[string]string{
		"serverId": "srv", "role": "server", "v": "2", "connectionId": "data-1",
	})
	if err != nil || data.ConnectionID != "data-1" {
		t.Fatalf("data = %#v, err %v", data, err)
	}

	client, err := ParseConnectionQuery(map[string]string{
		"serverId": "srv", "role": "client", "v": "2", "connectionId": "  client-1  ",
	})
	if err != nil || client.ConnectionID != "client-1" {
		t.Fatalf("client = %#v, err %v", client, err)
	}
}

func TestParseConnectionQueryGeneratesV2ClientConnectionID(t *testing.T) {
	got, err := ParseConnectionQuery(map[string]string{"serverId": "srv", "role": "client", "v": "2"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.ConnectionID, "conn_") || len(got.ConnectionID) != len("conn_")+16 {
		t.Fatalf("generated connection id = %q", got.ConnectionID)
	}
}

func TestParseConnectionQueryRejectsInvalidParameters(t *testing.T) {
	tests := []struct {
		name  string
		query map[string]string
		want  string
	}{
		{"role", map[string]string{"serverId": "srv"}, "Missing or invalid role parameter"},
		{"server id", map[string]string{"role": "server"}, "Missing serverId parameter"},
		{"empty server id", map[string]string{"role": "server", "serverId": ""}, "Missing serverId parameter"},
		{"long server id", map[string]string{"role": "server", "serverId": strings.Repeat("x", 257)}, "serverId is too long"},
		{"version", map[string]string{"role": "server", "serverId": "srv", "v": "3"}, "Invalid v parameter (expected 1 or 2)"},
		{"long connection id", map[string]string{"role": "server", "serverId": "srv", "v": "2", "connectionId": strings.Repeat("x", 257)}, "connectionId is too long"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseConnectionQuery(test.query)
			if err == nil || err.Error() != test.want {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}
