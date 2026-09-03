package truenas

import (
	"context"
	"testing"
)

func TestNetworkManifestIsComplete(t *testing.T) {
	if got := len(NetworkMethods); got != 35 {
		t.Fatalf("network methods = %d, want 35", got)
	}
	seen := make(map[string]bool, len(NetworkMethods))
	for _, method := range NetworkMethods {
		if seen[method.Name] {
			t.Fatalf("duplicate network method %q", method.Name)
		}
		seen[method.Name] = true
		if method.Service == "" || method.Kind == "" {
			t.Fatalf("incomplete metadata: %+v", method)
		}
	}
	for _, name := range []string{"dns.query", "interface.commit", "interface.update", "network.configuration.update", "network.general.summary", "route.system_routes", "staticroute.delete"} {
		if !seen[name] {
			t.Errorf("missing %s", name)
		}
	}
}

func TestNetworkRiskMetadata(t *testing.T) {
	for _, name := range []string{"interface.cancel_rollback", "interface.checkin", "interface.commit", "interface.create", "interface.delete", "interface.rollback", "interface.save_network_config", "interface.update", "network.configuration.update", "staticroute.create", "staticroute.delete", "staticroute.update"} {
		method, ok := NetworkMethodByName(name)
		if !ok || !method.Destructive {
			t.Errorf("%s should be destructive: %+v", name, method)
		}
	}
	if method, _ := NetworkMethodByName("network.general.summary"); method.Destructive {
		t.Fatal("network.general.summary should not be destructive")
	}
}

func TestUnknownNetworkMethod(t *testing.T) {
	service := InterfaceService{networkCaller{client: &Client{}}}
	if _, err := service.Call(context.Background(), "not_real", NetworkCall{}); err == nil {
		t.Fatal("expected validation error")
	}
}
