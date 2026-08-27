package main

import (
	"bytes"
	"testing"

	"github.com/coder/websocket"
)

// These scenarios mirror test/relay_backpressure_test.exs. Socket-level cases
// use the public relay directly; deterministic scheduler/memory faults use the
// same-package scenario controller and assert the externally visible result.
func backpressureScenario(t *testing.T, name string) relayScenarioResult {
	t.Helper()
	return requireRelayScenario(t, mustNewRelay(t, DefaultConfig()), "backpressure/"+name)
}

func requireClose(t *testing.T, result relayScenarioResult, code websocket.StatusCode, reason string) {
	t.Helper()
	if result.CloseCode != code || result.CloseReason != reason {
		t.Fatalf("close = (%d, %q), want (%d, %q)", result.CloseCode, result.CloseReason, code, reason)
	}
}

func requireCapacityZero(t *testing.T, result relayScenarioResult) {
	t.Helper()
	if result.IngressReservedBytes != 0 || result.InflightDeliveryBytes != 0 || result.BackpressuredSources != 0 {
		t.Fatalf("capacity leaked: ingress=%d inflight=%d blocked=%d", result.IngressReservedBytes, result.InflightDeliveryBytes, result.BackpressuredSources)
	}
}

func TestBackpressureWaitsForDaemonDataWithoutUnboundedClientBuffering(t *testing.T) {
	r := backpressureScenario(t, "wait-for-data")
	if !r.SourceBlocked || len(r.Forwarded) != 1 {
		t.Fatalf("blocked/forwarded = %v/%d, want true/1", r.SourceBlocked, len(r.Forwarded))
	}
}

func TestBackpressurePassiveDestinationStallsSourceTCP(t *testing.T) {
	r := backpressureScenario(t, "passive-destination")
	if !r.SourceBlocked || r.IngressReservedBytes <= 0 {
		t.Fatalf("source was not backpressured: %#v", r)
	}
}

func TestBackpressureSuspendedSourceKeepsOutboundWriterLive(t *testing.T) {
	r := backpressureScenario(t, "suspended-source-outbound-live")
	if len(r.Forwarded) == 0 || r.DestinationClosed {
		t.Fatalf("outbound writer did not stay live: %#v", r)
	}
}

func TestBackpressureBlockedSourceDoesNotAdmitItsNextFrame(t *testing.T) {
	r := backpressureScenario(t, "strict-node-byte-budget")
	if r.CloseCode != 0 {
		t.Fatalf("blocked source closed before data attached: (%d, %q)", r.CloseCode, r.CloseReason)
	}
	if len(r.Forwarded) != 2 || string(r.Forwarded[0]) != "12345678" || string(r.Forwarded[1]) != "x" {
		t.Fatalf("forwarded frames = %q, want both frames in order", r.Forwarded)
	}
	if r.IngressReservedBytes > int64(DefaultConfig().IngressBudgetBytes) {
		t.Fatalf("reserved %d exceeds budget", r.IngressReservedBytes)
	}
}

func TestBackpressurePipelinedFramesRetainOrderWithOneActiveDelivery(t *testing.T) {
	r := backpressureScenario(t, "pipelined-fifo")
	if len(r.Forwarded) != 2 || !bytes.Equal(r.Forwarded[0], []byte("first")) || !bytes.Equal(r.Forwarded[1], []byte("second")) {
		t.Fatalf("forwarded order = %q", r.Forwarded)
	}
}

func TestBackpressureMaximumLegalUnfragmentedPayloadSurvives(t *testing.T) {
	r := backpressureScenario(t, "maximum-unfragmented")
	if len(r.Forwarded) != 1 || len(r.Forwarded[0]) != MaximumMessagePayloadBytes {
		t.Fatalf("forwarded sizes = %v", payloadSizes(r.Forwarded))
	}
}

func TestBackpressureFragmentedMaximumPayloadAllowsInterleavedControl(t *testing.T) {
	r := backpressureScenario(t, "maximum-fragmented-with-control")
	if len(r.Forwarded) != 1 || len(r.Forwarded[0]) != MaximumMessagePayloadBytes || !r.ControlSocketUsed {
		t.Fatalf("fragment/control result = %#v", r)
	}
}

func TestBackpressureIncompleteFragmentsRemainOutsideRelayAdmission(t *testing.T) {
	r := backpressureScenario(t, "incomplete-fragment-unreserved")
	if r.IngressReservedBytes != 0 || r.InflightDeliveryBytes != 0 {
		t.Fatalf("incomplete fragment consumed admission: %#v", r)
	}
}

func TestBackpressureRejectsPayloadOneByteOverWireCeilingWith1009(t *testing.T) {
	requireClose(t, backpressureScenario(t, "wire-ceiling-plus-one"), websocket.StatusMessageTooBig, "Message too big")
}

func TestBackpressureConcurrentProducersRetainPerSourceFIFO(t *testing.T) {
	r := backpressureScenario(t, "concurrent-source-fifo")
	if len(r.Forwarded) != 4 || string(r.Forwarded[0]) != "a1" || string(r.Forwarded[2]) != "a2" {
		t.Fatalf("per-source FIFO not retained: %q", r.Forwarded)
	}
}

func TestBackpressureControlNotificationsUpdateForwardedMetric(t *testing.T) {
	r := backpressureScenario(t, "control-forwarded-metric")
	// The initial sync and the pong both go through the control writer, and the
	// reference counts every control write as a forwarded frame.
	if r.FramesForwarded != 2 || r.BytesForwarded <= 0 {
		t.Fatalf("forward metrics = frames:%d bytes:%d", r.FramesForwarded, r.BytesForwarded)
	}
}

func TestBackpressureAcceptedControlCannotExpireSilentlyInWriterQueue(t *testing.T) {
	r := backpressureScenario(t, "control-queue-deadline")
	if !r.DestinationClosed {
		t.Fatal("accepted control notification expired without closing destination")
	}
}

func TestBackpressureUnreadControlSocketIsShedThroughBoundedWriter(t *testing.T) {
	requireClose(t, backpressureScenario(t, "unread-control"), websocket.StatusTryAgainLater, "Slow consumer")
}

func TestBackpressureWriterCrashClosesRealWebSocket(t *testing.T) {
	requireClose(t, backpressureScenario(t, "writer-crash"), websocket.StatusTryAgainLater, "Delivery unavailable")
}

func TestBackpressureWriterRejectsSuccessorAfterActiveSourceDies(t *testing.T) {
	r := backpressureScenario(t, "dead-source-successor")
	requireClose(t, r, websocket.StatusTryAgainLater, "Delivery unavailable")
	if len(r.Forwarded) != 0 {
		t.Fatalf("successor was accepted: %q", r.Forwarded)
	}
}

func TestBackpressureOversizedV2ControlIsRejectedBeforeJSONParsing(t *testing.T) {
	requireClose(t, backpressureScenario(t, "oversized-control"), websocket.StatusMessageTooBig, "Message too big")
}

func TestBackpressureHeapFuseKillReconcilesEveryCapacityGauge(t *testing.T) {
	r := backpressureScenario(t, "heap-fuse-reconcile")
	requireCapacityZero(t, r)
	if r.ActiveWebSockets != 0 {
		t.Fatalf("active websockets = %d", r.ActiveWebSockets)
	}
}

func TestBackpressureWatermarkClosesOldestBlockedSource(t *testing.T) {
	requireClose(t, backpressureScenario(t, "watermark-oldest"), websocket.StatusTryAgainLater, "Relay memory pressure")
}

func TestBackpressureFailedQueuedWriteClosesDestination(t *testing.T) {
	r := backpressureScenario(t, "queued-write-failure")
	if !r.DestinationClosed || r.CloseCode != websocket.StatusTryAgainLater {
		t.Fatalf("destination remained usable: %#v", r)
	}
}

func TestBackpressureWatermarkClosesIncompleteFragmentSource(t *testing.T) {
	requireClose(t, backpressureScenario(t, "watermark-incomplete-fragment"), websocket.StatusTryAgainLater, "Relay memory pressure")
}

func TestBackpressurePressureEpisodePausesAdmissionUntilMemoryRelief(t *testing.T) {
	r := backpressureScenario(t, "pressure-pause-relief")
	if !r.AdmissionOpen || r.ConnectionRejections == 0 {
		t.Fatalf("admission did not pause then recover: %#v", r)
	}
}

func TestBackpressureTimedOutFrameInvalidatesCapacityEpoch(t *testing.T) {
	r := backpressureScenario(t, "timed-out-frame-epoch")
	if !r.CapacityEpochChanged || r.ActiveWebSockets != 0 {
		t.Fatalf("capacity epoch was not invalidated: %#v", r)
	}
}

func TestBackpressureCapacityRestartDrainsRetainedPayloadsBeforeAdmission(t *testing.T) {
	r := backpressureScenario(t, "restart-drains-retained")
	requireCapacityZero(t, r)
	if !r.CapacityEpochChanged || !r.AdmissionOpen {
		t.Fatalf("capacity restart did not recover cleanly: %#v", r)
	}
}

func TestBackpressureMissingDaemonDataRouteExpiresWithRetryableClose(t *testing.T) {
	requireClose(t, backpressureScenario(t, "missing-data-route"), websocket.StatusTryAgainLater, "Data route unavailable")
}

func TestBackpressureDestinationDeathReleasesBlockedProducersAndReservations(t *testing.T) {
	r := backpressureScenario(t, "destination-death-release")
	requireCapacityZero(t, r)
	if !r.DestinationClosed {
		t.Fatal("destination death was not observed")
	}
}

func TestBackpressureFanoutMetricsCountEveryDestinationDelivery(t *testing.T) {
	r := backpressureScenario(t, "fanout-metrics")
	if r.FramesForwarded != 2 || r.BytesForwarded != 2*int64(len("fanout")) {
		t.Fatalf("fanout metrics = frames:%d bytes:%d", r.FramesForwarded, r.BytesForwarded)
	}
}

func TestBackpressureFanoutCountsOneBlockedSourceAndOneDeliveryWait(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	server := httptestServerForRelay(t, relay)
	serverID := "fanout-metric-semantics"

	control := dialRelay(t, server, serverID, RoleServer, 2, "")
	assertControlMessage(t, control, map[string]any{"type": "sync"})
	first := dialRelay(t, server, serverID, RoleClient, 2, "fanout")
	assertControlMessage(t, control, map[string]any{"type": "connected", "connectionId": "fanout"})
	second := dialRelay(t, server, serverID, RoleClient, 2, "fanout")
	assertControlMessage(t, control, map[string]any{"type": "connected", "connectionId": "fanout"})
	data := dialRelay(t, server, serverID, RoleServer, 2, "fanout")

	peers := relayClientPeers(relay, serverID, "fanout")
	if len(peers) != 2 {
		t.Fatalf("fan-out peers = %d, want 2", len(peers))
	}
	// Hold the first destination's writer. The source still has one delivery in
	// flight even after the second destination accepts the frame.
	peers[0].writeSlot <- struct{}{}
	writeRelayMessage(t, data, websocket.MessageBinary, []byte("fanout"))
	eventually(t, relayTestTimeout, func() bool {
		return relay.backpressuredSources.Load() == 1 && relay.inflightDelivery.Load() == int64(len("fanout"))
	})
	if got := relay.deliveryWaitCount.Load(); got != 0 {
		t.Fatalf("delivery wait count while fan-out is blocked = %d, want 0", got)
	}

	assertRelayMessage(t, second, websocket.MessageBinary, []byte("fanout"))
	if got := relay.backpressuredSources.Load(); got != 1 {
		t.Fatalf("blocked sources after healthy fan-out delivery = %d, want 1", got)
	}
	if got := relay.deliveryWaitCount.Load(); got != 0 {
		t.Fatalf("delivery wait count before blocked destination releases = %d, want 0", got)
	}

	<-peers[0].writeSlot
	assertRelayMessage(t, first, websocket.MessageBinary, []byte("fanout"))
	eventually(t, relayTestTimeout, func() bool {
		return relay.backpressuredSources.Load() == 0 && relay.inflightDelivery.Load() == 0 && relay.deliveryWaitCount.Load() == 1
	})
	if got := relay.deliveryWaitCount.Load(); got != 1 {
		t.Fatalf("delivery wait count = %d, want 1", got)
	}
}

func TestBackpressureUnreadFanoutPeerCannotDelayHealthyOrder(t *testing.T) {
	r := backpressureScenario(t, "unread-fanout-peer")
	if len(r.Forwarded) < 2 || string(r.Forwarded[0]) != "one" || string(r.Forwarded[1]) != "two" {
		t.Fatalf("healthy peer order = %q", r.Forwarded)
	}
}

func payloadSizes(payloads [][]byte) []int {
	sizes := make([]int, len(payloads))
	for i := range payloads {
		sizes[i] = len(payloads[i])
	}
	return sizes
}
