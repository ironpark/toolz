package main

import (
	"strconv"
	"testing"
)

func TestLoadConfigUsesSafeLocalDefaults(t *testing.T) {
	got, err := LoadConfig(nil)
	if err != nil {
		t.Fatalf("LoadConfig(nil): %v", err)
	}

	want := DefaultConfig()
	if got.Host != want.Host || got.Port != want.Port || got.Drain != want.Drain ||
		got.Acceptors != want.Acceptors || got.ConnectionsPerAcceptor != want.ConnectionsPerAcceptor ||
		got.IngressBudgetBytes != want.IngressBudgetBytes || got.IngressWeight != want.IngressWeight ||
		got.DeliveryTimeoutMS != want.DeliveryTimeoutMS ||
		got.TransportSendTimeoutMS != want.TransportSendTimeoutMS ||
		got.MemoryWatermarkBytes != want.MemoryWatermarkBytes {
		t.Fatalf("defaults differ: got %#v, want %#v", got, want)
	}
	if got.IP.String() != "127.0.0.1" {
		t.Fatalf("default IP = %q, want 127.0.0.1", got.IP)
	}
}

func TestEnvironmentVariableInventory(t *testing.T) {
	vars := EnvironmentVariables()
	if len(vars) != 22 {
		t.Fatalf("environment variable count = %d, want 22", len(vars))
	}

	seen := make(map[string]bool, len(vars))
	for _, variable := range vars {
		if variable.Name == "" || seen[variable.Name] {
			t.Fatalf("invalid or duplicate environment variable: %#v", variable)
		}
		seen[variable.Name] = true
	}
	for _, name := range []string{
		"PASEO_RELAY_HOST", "PASEO_RELAY_PORT", "PASEO_RELAY_DRAIN",
		"PASEO_RELAY_OWNERSHIP_TARGET", "PASEO_RELAY_REROUTE_HEADER",
		"PASEO_RELAY_CLUSTER_QUERY", "PASEO_RELAY_MIN_CLUSTER_SIZE",
		"PASEO_RELAY_ACCEPTORS", "PASEO_RELAY_CONNECTIONS_PER_ACCEPTOR",
		"PASEO_RELAY_HTTP_IDLE_TIMEOUT_MS", "PASEO_RELAY_CAPACITY_MUTATION_TIMEOUT_MS",
		"PASEO_RELAY_INGRESS_BUDGET_BYTES", "PASEO_RELAY_INGRESS_WEIGHT",
		"PASEO_RELAY_DELIVERY_TIMEOUT_MS", "PASEO_RELAY_TRANSPORT_SEND_TIMEOUT_MS",
		"PASEO_RELAY_CONTROL_QUEUE_BYTES", "PASEO_RELAY_DATA_ATTACH_TIMEOUT_MS",
		"PASEO_RELAY_TCP_RECEIVE_BUFFER_BYTES", "PASEO_RELAY_WEBSOCKET_MAX_HEAP_WORDS",
		"PASEO_RELAY_MEMORY_WATERMARK_BYTES", "RELEASE_NODE", "RELEASE_COOKIE",
	} {
		if !seen[name] {
			t.Errorf("missing environment variable %q", name)
		}
	}
}

func TestLoadConfigParsesReleaseSettings(t *testing.T) {
	got, err := LoadConfig(map[string]string{
		"PASEO_RELAY_HOST":                         "127.0.0.2",
		"PASEO_RELAY_PORT":                         "4400",
		"PASEO_RELAY_DRAIN":                        "true",
		"PASEO_RELAY_OWNERSHIP_TARGET":             "node-a",
		"PASEO_RELAY_REROUTE_HEADER":               "x-owner",
		"PASEO_RELAY_CLUSTER_QUERY":                "_paseo._tcp.example",
		"PASEO_RELAY_MIN_CLUSTER_SIZE":             "3",
		"PASEO_RELAY_ACCEPTORS":                    "20",
		"PASEO_RELAY_CONNECTIONS_PER_ACCEPTOR":     "750",
		"PASEO_RELAY_HTTP_IDLE_TIMEOUT_MS":         "10000",
		"PASEO_RELAY_CAPACITY_MUTATION_TIMEOUT_MS": "7500",
		"PASEO_RELAY_INGRESS_BUDGET_BYTES":         "256000000",
		"PASEO_RELAY_INGRESS_WEIGHT":               "2",
		"PASEO_RELAY_DELIVERY_TIMEOUT_MS":          "5000",
		"PASEO_RELAY_TRANSPORT_SEND_TIMEOUT_MS":    "6000",
		"PASEO_RELAY_CONTROL_QUEUE_BYTES":          "1024",
		"PASEO_RELAY_DATA_ATTACH_TIMEOUT_MS":       "2000",
		"PASEO_RELAY_TCP_RECEIVE_BUFFER_BYTES":     "32768",
		"PASEO_RELAY_WEBSOCKET_MAX_HEAP_WORDS":     "33554432",
		"PASEO_RELAY_MEMORY_WATERMARK_BYTES":       "1500000000",
		"RELEASE_NODE":                             "relay@127.0.0.2",
		"RELEASE_COOKIE":                           "secret",
	})
	if err != nil {
		t.Fatalf("LoadConfig(): %v", err)
	}

	if got.Port != 4400 || !got.Drain || got.Acceptors != 20 || got.ConnectionsPerAcceptor != 750 ||
		got.HTTPIdleTimeoutMS != 10000 || got.CapacityMutationTimeoutMS != 7500 ||
		got.IngressBudgetBytes != 256000000 || got.IngressWeight != 2 ||
		got.DeliveryTimeoutMS != 5000 || got.TransportSendTimeoutMS != 6000 ||
		got.ControlQueueBytes != 1024 || got.DataAttachTimeoutMS != 2000 ||
		got.TCPReceiveBufferBytes != 32768 || got.WebsocketMaxHeapWords != 33554432 ||
		got.MemoryWatermarkBytes != 1500000000 || got.MinimumClusterSize != 3 ||
		got.OwnershipTarget != "node-a" || got.RerouteHeader != "x-owner" ||
		got.ClusterQuery != "_paseo._tcp.example" || got.NodeName != "relay@127.0.0.2" ||
		got.Cookie != "secret" {
		t.Fatalf("parsed config differs: %#v", got)
	}
}

func TestLoadConfigRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"host", map[string]string{"PASEO_RELAY_HOST": "not-an-ip"}, "PASEO_RELAY_HOST must be an IP address"},
		{"port", map[string]string{"PASEO_RELAY_PORT": "not-a-port"}, "PASEO_RELAY_PORT must be an integer between 1 and 65535"},
		{"drain", map[string]string{"PASEO_RELAY_DRAIN": "yes"}, "PASEO_RELAY_DRAIN must be true or false"},
		{"acceptors", map[string]string{"PASEO_RELAY_ACCEPTORS": "0"}, "PASEO_RELAY_ACCEPTORS must be an integer between 1 and 1000"},
		{"connections", map[string]string{"PASEO_RELAY_CONNECTIONS_PER_ACCEPTOR": "0"}, "PASEO_RELAY_CONNECTIONS_PER_ACCEPTOR must be an integer between 1 and 1000000"},
		{"http idle", map[string]string{"PASEO_RELAY_HTTP_IDLE_TIMEOUT_MS": "0"}, "PASEO_RELAY_HTTP_IDLE_TIMEOUT_MS must be an integer between 100 and 120000"},
		{"capacity mutation", map[string]string{"PASEO_RELAY_CAPACITY_MUTATION_TIMEOUT_MS": "99"}, "PASEO_RELAY_CAPACITY_MUTATION_TIMEOUT_MS must be an integer between 100 and 120000"},
		{"ingress weight", map[string]string{"PASEO_RELAY_INGRESS_WEIGHT": "17"}, "PASEO_RELAY_INGRESS_WEIGHT must be an integer between 1 and 16"},
		{"control queue", map[string]string{"PASEO_RELAY_CONTROL_QUEUE_BYTES": "63"}, "PASEO_RELAY_CONTROL_QUEUE_BYTES must be an integer between 64 and 67108864"},
		{"data attach", map[string]string{"PASEO_RELAY_DATA_ATTACH_TIMEOUT_MS": "999"}, "PASEO_RELAY_DATA_ATTACH_TIMEOUT_MS must be an integer between 1000 and 120000"},
		{"receive buffer", map[string]string{"PASEO_RELAY_TCP_RECEIVE_BUFFER_BYTES": "4095"}, "PASEO_RELAY_TCP_RECEIVE_BUFFER_BYTES must be an integer between 4096 and 1048576"},
		{"heap fuse", map[string]string{"PASEO_RELAY_WEBSOCKET_MAX_HEAP_WORDS": "33554431"}, "PASEO_RELAY_WEBSOCKET_MAX_HEAP_WORDS must be an integer between 33554432 and 134217728"},
		{"watermark", map[string]string{"PASEO_RELAY_MEMORY_WATERMARK_BYTES": "1"}, "PASEO_RELAY_MEMORY_WATERMARK_BYTES must be an integer between 268435456 and 68719476736"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadConfig(test.env)
			if err == nil || err.Error() != test.want {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLoadConfigValidatesCrossFieldLimits(t *testing.T) {
	_, err := LoadConfig(map[string]string{
		"PASEO_RELAY_DELIVERY_TIMEOUT_MS":       "5000",
		"PASEO_RELAY_TRANSPORT_SEND_TIMEOUT_MS": "5000",
	})
	if want := "PASEO_RELAY_DELIVERY_TIMEOUT_MS must be lower than PASEO_RELAY_TRANSPORT_SEND_TIMEOUT_MS"; err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}

	_, err = LoadConfig(map[string]string{
		"PASEO_RELAY_INGRESS_BUDGET_BYTES": strconv.Itoa(128 * 1024 * 1024),
		"PASEO_RELAY_INGRESS_WEIGHT":       "8",
	})
	if want := "PASEO_RELAY_INGRESS_BUDGET_BYTES must admit one maximum assembled message at the configured weight"; err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}

	weight := 5
	exactBudget := MaximumMessagePayloadBytes * weight
	got, err := LoadConfig(map[string]string{
		"PASEO_RELAY_INGRESS_BUDGET_BYTES": strconv.Itoa(exactBudget),
		"PASEO_RELAY_INGRESS_WEIGHT":       strconv.Itoa(weight),
	})
	if err != nil || got.IngressBudgetBytes != exactBudget || got.IngressWeight != weight {
		t.Fatalf("exact budget: got %#v, err %v", got, err)
	}
}

func TestLoadConfigAllowsDisabledMemoryWatermark(t *testing.T) {
	got, err := LoadConfig(map[string]string{"PASEO_RELAY_MEMORY_WATERMARK_BYTES": "0"})
	if err != nil || got.MemoryWatermarkBytes != 0 {
		t.Fatalf("disabled watermark: got %#v, err %v", got, err)
	}
}

// One-to-one ports of the focused upstream Config cases. The broader table
// tests above additionally guard the complete Go environment inventory.
func TestConfigLoadsAndValidatesCapacityMutationTimeout(t *testing.T) {
	got, err := LoadConfig(map[string]string{"PASEO_RELAY_CAPACITY_MUTATION_TIMEOUT_MS": "7500"})
	if err != nil || got.CapacityMutationTimeoutMS != 7500 {
		t.Fatalf("config = %#v, err=%v", got, err)
	}
	assertConfigError(t, map[string]string{"PASEO_RELAY_CAPACITY_MUTATION_TIMEOUT_MS": "99"}, "PASEO_RELAY_CAPACITY_MUTATION_TIMEOUT_MS must be an integer between 100 and 120000")
}

func TestConfigRejectsUnbindableListenerHostname(t *testing.T) {
	assertConfigError(t, map[string]string{"PASEO_RELAY_HOST": "not-an-ip"}, "PASEO_RELAY_HOST must be an IP address")
}

func TestConfigRejectsInvalidPort(t *testing.T) {
	assertConfigError(t, map[string]string{"PASEO_RELAY_PORT": "not-a-port"}, "PASEO_RELAY_PORT must be an integer between 1 and 65535")
}

func TestConfigRecognizesDrainMode(t *testing.T) {
	got, err := LoadConfig(map[string]string{"PASEO_RELAY_DRAIN": "true", "PASEO_RELAY_PORT": "4400"})
	if err != nil || !got.Drain || got.Port != 4400 {
		t.Fatalf("config = %#v, err=%v", got, err)
	}
}

func TestConfigRequiresHeapFuseToAdmitMaximumFrame(t *testing.T) {
	assertConfigError(t, map[string]string{"PASEO_RELAY_WEBSOCKET_MAX_HEAP_WORDS": "33554431"}, "PASEO_RELAY_WEBSOCKET_MAX_HEAP_WORDS must be an integer between 33554432 and 134217728")
	got, err := LoadConfig(map[string]string{"PASEO_RELAY_WEBSOCKET_MAX_HEAP_WORDS": "33554432"})
	if err != nil || got.WebsocketMaxHeapWords != 33_554_432 {
		t.Fatalf("config = %#v, err=%v", got, err)
	}
}

func TestConfigLoadsListenerCeilingAsConnectionsPerAcceptor(t *testing.T) {
	got, err := LoadConfig(map[string]string{"PASEO_RELAY_ACCEPTORS": "20", "PASEO_RELAY_CONNECTIONS_PER_ACCEPTOR": "750", "PASEO_RELAY_HTTP_IDLE_TIMEOUT_MS": "10000"})
	if err != nil || got.Acceptors != 20 || got.ConnectionsPerAcceptor != 750 || got.HTTPIdleTimeoutMS != 10_000 {
		t.Fatalf("config = %#v, err=%v", got, err)
	}
}

func TestConfigValidatesWeightedIngressAndDeliveryLimits(t *testing.T) {
	got, err := LoadConfig(map[string]string{
		"PASEO_RELAY_INGRESS_BUDGET_BYTES": "256000000", "PASEO_RELAY_INGRESS_WEIGHT": "2",
		"PASEO_RELAY_DELIVERY_TIMEOUT_MS": "5000", "PASEO_RELAY_TRANSPORT_SEND_TIMEOUT_MS": "6000",
		"PASEO_RELAY_CONTROL_QUEUE_BYTES": "1024", "PASEO_RELAY_TCP_RECEIVE_BUFFER_BYTES": "32768",
	})
	if err != nil || got.IngressBudgetBytes != 256_000_000 || got.IngressWeight != 2 || got.DeliveryTimeoutMS != 5_000 || got.TransportSendTimeoutMS != 6_000 {
		t.Fatalf("config = %#v, err=%v", got, err)
	}
}

func TestConfigRequiresCapacityForOneMaximumCompleteMessage(t *testing.T) {
	weight := 5
	exact := MaximumMessagePayloadBytes * weight
	got, err := LoadConfig(map[string]string{"PASEO_RELAY_INGRESS_BUDGET_BYTES": strconv.Itoa(exact), "PASEO_RELAY_INGRESS_WEIGHT": strconv.Itoa(weight)})
	if err != nil || got.IngressBudgetBytes != exact {
		t.Fatalf("exact capacity = %#v, err=%v", got, err)
	}
	assertConfigError(t, map[string]string{"PASEO_RELAY_INGRESS_BUDGET_BYTES": strconv.Itoa(exact - 1), "PASEO_RELAY_INGRESS_WEIGHT": strconv.Itoa(weight)}, "PASEO_RELAY_INGRESS_BUDGET_BYTES must admit one maximum assembled message at the configured weight")
}

func assertConfigError(t *testing.T, env map[string]string, want string) {
	t.Helper()
	_, err := LoadConfig(env)
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}
