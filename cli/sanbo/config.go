package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
)

// Config is the validated runtime configuration for a relay node.
type Config struct {
	Host                      string
	Port                      int
	Drain                     bool
	Acceptors                 int
	ConnectionsPerAcceptor    int
	HTTPIdleTimeoutMS         int
	CapacityMutationTimeoutMS int
	IngressBudgetBytes        int
	IngressWeight             int
	DeliveryTimeoutMS         int
	TransportSendTimeoutMS    int
	ControlQueueBytes         int
	DataAttachTimeoutMS       int
	TCPReceiveBufferBytes     int
	WebsocketMaxHeapWords     int
	MemoryWatermarkBytes      int
	OwnershipTarget           string
	RerouteHeader             string
	MinimumClusterSize        int
	ClusterQuery              string
	NodeName                  string
	Cookie                    string
}

// EnvironmentVariable describes a supported runtime environment variable.
type EnvironmentVariable struct {
	Name        string
	Default     string
	Description string
}

var environmentVariables = []EnvironmentVariable{
	{Name: "PASEO_RELAY_HOST", Default: "127.0.0.1", Description: "Public listener IP."},
	{Name: "PASEO_RELAY_PORT", Default: "4000", Description: "Public HTTP/WebSocket listener port."},
	{Name: "PASEO_RELAY_DRAIN", Default: "false", Description: "Start unready while existing sessions drain."},
	{Name: "PASEO_RELAY_OWNERSHIP_TARGET", Default: "local", Description: "Opaque target advertised to relay peers."},
	{Name: "PASEO_RELAY_REROUTE_HEADER", Default: "x-reroute-target", Description: "Response header used by the deployment adapter."},
	{Name: "PASEO_RELAY_CLUSTER_QUERY", Default: "", Description: "Namespace component for the optional same-host file lease registry; not DNS discovery."},
	{Name: "PASEO_RELAY_MIN_CLUSTER_SIZE", Default: "1", Description: "Minimum cluster size before accepting unowned sessions."},
	{Name: "PASEO_RELAY_ACCEPTORS", Default: "100", Description: "Listener acceptor processes."},
	{Name: "PASEO_RELAY_CONNECTIONS_PER_ACCEPTOR", Default: "200", Description: "Live connections allowed per acceptor."},
	{Name: "PASEO_RELAY_HTTP_IDLE_TIMEOUT_MS", Default: "15000", Description: "HTTP parsing and unread request-body idle timeout."},
	{Name: "PASEO_RELAY_CAPACITY_MUTATION_TIMEOUT_MS", Default: "5000", Description: "Capacity state mutation timeout."},
	{Name: "PASEO_RELAY_INGRESS_BUDGET_BYTES", Default: "536870912", Description: "Node-wide weighted ingress byte budget."},
	{Name: "PASEO_RELAY_INGRESS_WEIGHT", Default: "4", Description: "Memory weight charged per wire payload byte."},
	{Name: "PASEO_RELAY_DELIVERY_TIMEOUT_MS", Default: "30000", Description: "Writer reservation and send-barrier deadline."},
	{Name: "PASEO_RELAY_TRANSPORT_SEND_TIMEOUT_MS", Default: "35000", Description: "TCP send timeout."},
	{Name: "PASEO_RELAY_CONTROL_QUEUE_BYTES", Default: "1048576", Description: "Per-destination control notification queue bound."},
	{Name: "PASEO_RELAY_DATA_ATTACH_TIMEOUT_MS", Default: "15000", Description: "Maximum wait for a v2 daemon-data socket."},
	{Name: "PASEO_RELAY_TCP_RECEIVE_BUFFER_BYTES", Default: "65536", Description: "Per-socket TCP receive buffer."},
	{Name: "PASEO_RELAY_WEBSOCKET_MAX_HEAP_WORDS", Default: "33554432", Description: "Per-WebSocket heap fuse."},
	{Name: "PASEO_RELAY_MEMORY_WATERMARK_BYTES", Default: "0", Description: "Optional BEAM memory watermark; zero disables it."},
	{Name: "RELEASE_NODE", Default: "", Description: "Member identity for the optional same-host file lease registry."},
	{Name: "RELEASE_COOKIE", Default: "", Description: "Namespace component for the optional same-host file lease registry."},
}

// EnvironmentVariables returns a copy of the supported environment variable list.
func EnvironmentVariables() []EnvironmentVariable {
	variables := make([]EnvironmentVariable, len(environmentVariables))
	copy(variables, environmentVariables)
	return variables
}

// DefaultConfig returns the safe local defaults used by the relay.
func DefaultConfig() Config {
	return Config{
		Host:                      "127.0.0.1",
		Port:                      4000,
		Acceptors:                 100,
		ConnectionsPerAcceptor:    200,
		HTTPIdleTimeoutMS:         15_000,
		CapacityMutationTimeoutMS: 5_000,
		IngressBudgetBytes:        512 * 1024 * 1024,
		IngressWeight:             4,
		DeliveryTimeoutMS:         30_000,
		TransportSendTimeoutMS:    35_000,
		ControlQueueBytes:         1024 * 1024,
		DataAttachTimeoutMS:       15_000,
		TCPReceiveBufferBytes:     64 * 1024,
		WebsocketMaxHeapWords:     32 * 1024 * 1024,
		OwnershipTarget:           "local",
		RerouteHeader:             "x-reroute-target",
		MinimumClusterSize:        1,
	}
}

// LoadConfig parses and validates an environment map. A nil map means no overrides.
func LoadConfig(environment map[string]string) (Config, error) {
	config := DefaultConfig()
	if environment == nil {
		environment = map[string]string{}
	}

	host := valueOr(environment, "PASEO_RELAY_HOST", config.Host)
	if net.ParseIP(host) == nil {
		return Config{}, fmt.Errorf("PASEO_RELAY_HOST must be an IP address")
	}

	var err error
	config.Host = host
	if config.Port, err = integer(environment, "PASEO_RELAY_PORT", config.Port, 1, 65_535); err != nil {
		return Config{}, err
	}
	if config.Drain, err = boolean(environment, "PASEO_RELAY_DRAIN", config.Drain); err != nil {
		return Config{}, err
	}
	if config.Acceptors, err = integer(environment, "PASEO_RELAY_ACCEPTORS", config.Acceptors, 1, 1_000); err != nil {
		return Config{}, err
	}
	if config.ConnectionsPerAcceptor, err = integer(environment, "PASEO_RELAY_CONNECTIONS_PER_ACCEPTOR", config.ConnectionsPerAcceptor, 1, 1_000_000); err != nil {
		return Config{}, err
	}
	if config.HTTPIdleTimeoutMS, err = integer(environment, "PASEO_RELAY_HTTP_IDLE_TIMEOUT_MS", config.HTTPIdleTimeoutMS, 100, 120_000); err != nil {
		return Config{}, err
	}
	if config.CapacityMutationTimeoutMS, err = integer(environment, "PASEO_RELAY_CAPACITY_MUTATION_TIMEOUT_MS", config.CapacityMutationTimeoutMS, 100, 120_000); err != nil {
		return Config{}, err
	}
	if config.IngressBudgetBytes, err = integer(environment, "PASEO_RELAY_INGRESS_BUDGET_BYTES", config.IngressBudgetBytes, 128*1024*1024, 8*1024*1024*1024); err != nil {
		return Config{}, err
	}
	if config.IngressWeight, err = integer(environment, "PASEO_RELAY_INGRESS_WEIGHT", config.IngressWeight, 1, 16); err != nil {
		return Config{}, err
	}
	if config.IngressBudgetBytes < MaximumMessagePayloadBytes*config.IngressWeight {
		return Config{}, fmt.Errorf("PASEO_RELAY_INGRESS_BUDGET_BYTES must admit one maximum assembled message at the configured weight")
	}
	if config.DeliveryTimeoutMS, err = integer(environment, "PASEO_RELAY_DELIVERY_TIMEOUT_MS", config.DeliveryTimeoutMS, 100, 120_000); err != nil {
		return Config{}, err
	}
	if config.TransportSendTimeoutMS, err = integer(environment, "PASEO_RELAY_TRANSPORT_SEND_TIMEOUT_MS", config.TransportSendTimeoutMS, 100, 300_000); err != nil {
		return Config{}, err
	}
	if config.DeliveryTimeoutMS >= config.TransportSendTimeoutMS {
		return Config{}, fmt.Errorf("PASEO_RELAY_DELIVERY_TIMEOUT_MS must be lower than PASEO_RELAY_TRANSPORT_SEND_TIMEOUT_MS")
	}
	if config.ControlQueueBytes, err = integer(environment, "PASEO_RELAY_CONTROL_QUEUE_BYTES", config.ControlQueueBytes, 64, 64*1024*1024); err != nil {
		return Config{}, err
	}
	if config.DataAttachTimeoutMS, err = integer(environment, "PASEO_RELAY_DATA_ATTACH_TIMEOUT_MS", config.DataAttachTimeoutMS, 1_000, 120_000); err != nil {
		return Config{}, err
	}
	if config.TCPReceiveBufferBytes, err = integer(environment, "PASEO_RELAY_TCP_RECEIVE_BUFFER_BYTES", config.TCPReceiveBufferBytes, 4*1024, 1024*1024); err != nil {
		return Config{}, err
	}
	if config.WebsocketMaxHeapWords, err = integer(environment, "PASEO_RELAY_WEBSOCKET_MAX_HEAP_WORDS", config.WebsocketMaxHeapWords, 32*1024*1024, 128*1024*1024); err != nil {
		return Config{}, err
	}
	if config.MemoryWatermarkBytes, err = disabledOrInteger(environment, "PASEO_RELAY_MEMORY_WATERMARK_BYTES", config.MemoryWatermarkBytes, 256*1024*1024, 64*1024*1024*1024); err != nil {
		return Config{}, err
	}
	if config.MinimumClusterSize, err = integer(environment, "PASEO_RELAY_MIN_CLUSTER_SIZE", config.MinimumClusterSize, 1, 1_000); err != nil {
		return Config{}, err
	}

	config.OwnershipTarget = valueOr(environment, "PASEO_RELAY_OWNERSHIP_TARGET", config.OwnershipTarget)
	config.RerouteHeader = valueOr(environment, "PASEO_RELAY_REROUTE_HEADER", config.RerouteHeader)
	config.ClusterQuery = environment["PASEO_RELAY_CLUSTER_QUERY"]
	config.NodeName = environment["RELEASE_NODE"]
	config.Cookie = environment["RELEASE_COOKIE"]
	return config, nil
}

// LoadConfigFromOS loads the runtime configuration from process environment variables.
func LoadConfigFromOS() (Config, error) {
	environment := make(map[string]string)
	for _, variable := range environmentVariables {
		if value, ok := os.LookupEnv(variable.Name); ok {
			environment[variable.Name] = value
		}
	}
	return LoadConfig(environment)
}

func valueOr(environment map[string]string, key, fallback string) string {
	if value, ok := environment[key]; ok {
		return value
	}
	return fallback
}

func integer(environment map[string]string, key string, fallback, minimum, maximum int) (int, error) {
	value, ok := environment[key]
	if !ok {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", key, minimum, maximum)
	}
	return parsed, nil
}

func boolean(environment map[string]string, key string, fallback bool) (bool, error) {
	value, ok := environment[key]
	if !ok {
		return fallback, nil
	}
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be true or false", key)
	}
}

// disabledOrInteger is integer with zero accepted as "disabled".
func disabledOrInteger(environment map[string]string, key string, fallback, minimum, maximum int) (int, error) {
	if environment[key] == "0" {
		return 0, nil
	}
	return integer(environment, key, fallback, minimum, maximum)
}
