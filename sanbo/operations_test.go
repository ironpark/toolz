package main

import (
	"net/http"
	"strings"
	"testing"
)

// Ported from references/paseo-relay/test/paseo_relay/operations_test.exs.
func TestOperationsHealthIsAlwaysLive(t *testing.T) {
	config := DefaultConfig()
	config.Drain = true
	server := newRelayTestServer(t, config)
	status, header, body := getResponse(t, server.URL+"/health")
	if status != http.StatusOK || header.Get("content-type") != "application/json" || body != `{"status":"ok"}` {
		t.Fatalf("health = (%d, %q, %q)", status, header.Get("content-type"), body)
	}
}

func TestOperationsReadyRefusesNewWorkDuringDrain(t *testing.T) {
	config := DefaultConfig()
	config.Drain = true
	server := newRelayTestServer(t, config)
	status, _, body := getResponse(t, server.URL+"/ready")
	if status != http.StatusServiceUnavailable || body != `{"status":"unready"}` {
		t.Fatalf("ready while draining = (%d, %q)", status, body)
	}
}

func TestOperationsMetricsExposeStablePrometheusSurface(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	status, header, body := getResponse(t, server.URL+"/metrics")
	if status != http.StatusOK || header.Get("content-type") != "text/plain; version=0.0.4" {
		t.Fatalf("metrics response = (%d, %q)", status, header.Get("content-type"))
	}
	requireMetricLines(t, body,
		"# HELP paseo_relay_ready Whether this node admits new relay work.\n",
		"# TYPE paseo_relay_ready gauge\n",
		"# HELP paseo_relay_draining Whether this node is draining.\n",
		"# TYPE paseo_relay_draining gauge\n",
		"# HELP paseo_relay_active_websockets Open WebSocket connections on this node.\n",
		"# TYPE paseo_relay_active_websockets gauge\n",
		"# HELP paseo_relay_active_sessions Relay sessions owned by this node.\n",
		"# TYPE paseo_relay_active_sessions gauge\n",
		"# HELP paseo_relay_reroute_responses_total WebSocket upgrades rerouted to another owner.\n",
		"# TYPE paseo_relay_reroute_responses_total counter\n",
		"# HELP paseo_relay_connection_rejections_total WebSocket upgrades rejected at configured capacity or during memory pressure.\n",
		"# TYPE paseo_relay_connection_rejections_total counter\n",
		"# HELP paseo_relay_frames_forwarded_total WebSocket frames forwarded by this node.\n",
		"# TYPE paseo_relay_frames_forwarded_total counter\n",
		"# HELP paseo_relay_bytes_forwarded_total WebSocket payload bytes forwarded by this node.\n",
		"# TYPE paseo_relay_bytes_forwarded_total counter\n",
		"# HELP paseo_relay_ingress_reserved_bytes Weighted ingress bytes admitted on this node.\n",
		"# TYPE paseo_relay_ingress_reserved_bytes gauge\n",
		"# HELP paseo_relay_inflight_delivery_bytes Payload bytes currently held by synchronous downstream delivery.\n",
		"# TYPE paseo_relay_inflight_delivery_bytes gauge\n",
		"# HELP paseo_relay_backpressured_sources Source WebSockets currently waiting for downstream delivery.\n",
		"# TYPE paseo_relay_backpressured_sources gauge\n",
		"# HELP paseo_relay_slow_consumer_disconnects_total Destinations disconnected after exceeding a delivery deadline.\n",
		"# TYPE paseo_relay_slow_consumer_disconnects_total counter\n",
		"# HELP paseo_relay_delivery_timeouts_total Synchronous downstream deliveries that exceeded their deadline.\n",
		"# TYPE paseo_relay_delivery_timeouts_total counter\n",
		"# HELP paseo_relay_memory_pressure_disconnects_total WebSockets closed by node memory-pressure recovery.\n",
		"# TYPE paseo_relay_memory_pressure_disconnects_total counter\n",
		"# HELP paseo_relay_max_frame_bytes Largest WebSocket frame payload observed since node start.\n",
		"# TYPE paseo_relay_max_frame_bytes gauge\n",
		"# HELP paseo_relay_beam_total_memory_bytes Total memory allocated by BEAM.\n",
		"# TYPE paseo_relay_beam_total_memory_bytes gauge\n",
		"# HELP paseo_relay_beam_process_memory_bytes Memory allocated by BEAM processes.\n",
		"# TYPE paseo_relay_beam_process_memory_bytes gauge\n",
		"# HELP paseo_relay_beam_binary_memory_bytes Memory allocated for BEAM binaries.\n",
		"# TYPE paseo_relay_beam_binary_memory_bytes gauge\n",
		"# HELP paseo_relay_beam_ets_memory_bytes Memory allocated for BEAM ETS tables.\n",
		"# TYPE paseo_relay_beam_ets_memory_bytes gauge\n",
		"# HELP paseo_relay_handshake_accepted_total Client E2EE handshake frames accepted by the handshake input validator.\n",
		"# TYPE paseo_relay_handshake_accepted_total counter\n",
		"# HELP paseo_relay_handshake_rejected_total Client E2EE handshake frames rejected by the handshake input validator.\n",
		"# TYPE paseo_relay_handshake_rejected_total counter\n",
		"# HELP paseo_relay_delivery_wait_seconds Time a source waits for synchronous downstream delivery.\n",
		"# TYPE paseo_relay_delivery_wait_seconds histogram\n",
		"# HELP paseo_relay_frame_size_bytes WebSocket payload-size distribution.\n",
		"# TYPE paseo_relay_frame_size_bytes histogram\n",
		"paseo_relay_ready 1\n",
		"paseo_relay_draining 0\n",
		"paseo_relay_active_websockets 0\n",
	)
}

func TestOperationsReadyWaitsForMinimumClusterSize(t *testing.T) {
	config := DefaultConfig()
	config.MinimumClusterSize = 2
	server := newRelayTestServer(t, config)
	status, _, body := getResponse(t, server.URL+"/ready")
	if status != http.StatusServiceUnavailable || body != `{"status":"unready"}` {
		t.Fatalf("ready below cluster floor = (%d, %q)", status, body)
	}
}

func TestOperationsMetricsRecoverAfterCapacityFailure(t *testing.T) {
	r := requireRelayScenario(t, mustNewRelay(t, DefaultConfig()), "operations/metrics-process-restart")
	if r.ConnectionRejections == 0 || !r.AdmissionOpen {
		t.Fatalf("metrics state was not retained across restart: %#v", r)
	}
}

func TestOperationsReadinessIsBoundedWhileCapacityIsStalled(t *testing.T) {
	r := requireRelayScenario(t, mustNewRelay(t, DefaultConfig()), "operations/stalled-capacity-ready")
	if r.AdmissionOpen || r.CloseCode != 0 {
		t.Fatalf("stalled Capacity readiness was not bounded/unavailable: %#v", r)
	}
}

func TestOperationsMetricsOmitUnavailableCapacityGauges(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	relay.connectionRejections.Add(1)
	relay.capacityUnavailable.Store(true)
	server := httptestServerForRelay(t, relay)
	metrics := relayMetrics(t, server)
	for _, name := range []string{
		"paseo_relay_active_websockets",
		"paseo_relay_ingress_reserved_bytes",
		"paseo_relay_inflight_delivery_bytes",
		"paseo_relay_backpressured_sources",
	} {
		if strings.Contains(metrics, name) {
			t.Errorf("unavailable capacity gauge %q was rendered", name)
		}
	}
	if !strings.Contains(metrics, "paseo_relay_connection_rejections_total 1\n") {
		t.Fatal("independent telemetry was lost while capacity was unavailable")
	}
}

func TestOperationsUnknownPathReturnsNotFound(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	status, header, body := getResponse(t, server.URL+"/missing")
	if status != http.StatusNotFound || header.Get("content-type") != "text/plain" || body != "not found\n" {
		t.Fatalf("not found = (%d, %q, %q)", status, header.Get("content-type"), body)
	}
}
