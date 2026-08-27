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
	for _, line := range []string{
		"paseo_relay_ready 1\n",
		"paseo_relay_draining 0\n",
		"paseo_relay_active_websockets 0\n",
	} {
		if !strings.Contains(body, line) {
			t.Errorf("metrics missing %q", line)
		}
	}
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
	r := requireRelayScenario(t, mustNewRelay(t, DefaultConfig()), "operations/stalled-capacity-metrics")
	if r.IngressReservedBytes != -1 || r.InflightDeliveryBytes != -1 || r.BackpressuredSources != -1 || r.ConnectionRejections == 0 {
		t.Fatalf("unavailable gauges were rendered or independent telemetry was lost: %#v", r)
	}
}

func TestOperationsUnknownPathReturnsNotFound(t *testing.T) {
	server := newRelayTestServer(t, DefaultConfig())
	status, header, body := getResponse(t, server.URL+"/missing")
	if status != http.StatusNotFound || header.Get("content-type") != "text/plain" || body != "not found\n" {
		t.Fatalf("not found = (%d, %q, %q)", status, header.Get("content-type"), body)
	}
}
