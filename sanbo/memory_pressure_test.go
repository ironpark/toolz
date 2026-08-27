package main

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// pressureConfig returns a config whose watermark is low enough that any
// sample the relay takes crosses it.
func pressureConfig(watermark int) Config {
	config := DefaultConfig()
	config.MemoryWatermarkBytes = watermark
	return config
}

func TestMemoryPressureEngagesAtWatermarkAndClosesAdmission(t *testing.T) {
	relay := mustNewRelay(t, pressureConfig(1_000))
	server := httptestServerForRelay(t, relay)
	if status, _, _ := getResponse(t, server.URL+"/ready"); status != http.StatusOK {
		t.Fatalf("readiness before pressure=%d, want 200", status)
	}

	relay.sampleMemoryPressure(1_000)

	if !relay.memoryPressure.Load() {
		t.Fatal("reaching the watermark did not engage memory pressure")
	}
	if status, _, _ := getResponse(t, server.URL+"/ready"); status != http.StatusServiceUnavailable {
		t.Fatalf("readiness under pressure=%d, want 503", status)
	}
	dialRelayExpectingStatus(t, server, "pressure-admission", RoleServer, 1, "", http.StatusServiceUnavailable)
}

func TestMemoryPressureShedsAttachedPeersAndWaitingDeliveries(t *testing.T) {
	relay := mustNewRelay(t, pressureConfig(1_000))
	server := httptestServerForRelay(t, relay)
	conn := dialRelay(t, server, "pressure-shed", RoleServer, 1, "")
	defer conn.CloseNow()
	eventually(t, relayTestTimeout, func() bool { return relay.activeWebSockets.Load() == 1 })

	relay.sampleMemoryPressure(1_000)

	code, reason, err := scenarioReadClose(conn)
	if err != nil {
		t.Fatalf("read shed close: %v", err)
	}
	if code != websocket.StatusTryAgainLater || reason != "Relay memory pressure" {
		t.Fatalf("shed close=%d %q, want 1013 \"Relay memory pressure\"", code, reason)
	}
	if got := relay.memoryPressureDisconnects.Load(); got != 1 {
		t.Fatalf("memory pressure disconnects=%d, want 1", got)
	}
	if reserved := relay.ingressReserved.Load(); reserved != 0 {
		t.Fatalf("ingress reserved after shed=%d, want 0", reserved)
	}
}

func TestMemoryPressureShedsOnlyOncePerCrossing(t *testing.T) {
	relay := mustNewRelay(t, pressureConfig(1_000))
	server := httptestServerForRelay(t, relay)
	conn := dialRelay(t, server, "pressure-once", RoleServer, 1, "")
	defer conn.CloseNow()
	eventually(t, relayTestTimeout, func() bool { return relay.activeWebSockets.Load() == 1 })

	relay.sampleMemoryPressure(2_000)
	relay.sampleMemoryPressure(3_000)
	relay.sampleMemoryPressure(4_000)

	if got := relay.memoryPressureDisconnects.Load(); got != 1 {
		t.Fatalf("disconnects across three samples above the watermark=%d, want 1", got)
	}
}

func TestMemoryPressureHoldsUntilUsageFallsBelowRecoveryThreshold(t *testing.T) {
	watermark := 2 * MaximumMessagePayloadBytes
	relay := mustNewRelay(t, pressureConfig(watermark))
	relay.sampleMemoryPressure(uint64(watermark))

	// Just under the watermark is not enough; pressure holds until the node has
	// room for one more maximum-size message.
	relay.sampleMemoryPressure(uint64(watermark) - 1)
	if !relay.memoryPressure.Load() {
		t.Fatal("pressure released one byte below the watermark, want it held")
	}

	relay.sampleMemoryPressure(uint64(watermark - MaximumMessagePayloadBytes))
	if relay.memoryPressure.Load() {
		t.Fatal("pressure did not release at the recovery threshold")
	}
}

// TestMemoryPressureShedsInBatchesAcrossSamples covers the gradual shedding
// contract: one crossing does not disconnect every socket on the node.
func TestMemoryPressureShedsInBatchesAcrossSamples(t *testing.T) {
	relay := mustNewRelay(t, pressureConfig(1_000))
	server := httptestServerForRelay(t, relay)
	sockets := initialShedBatch + 4
	for i := 0; i < sockets; i++ {
		conn := dialRelay(t, server, "pressure-batch-"+strconv.Itoa(i), RoleServer, 1, "")
		defer conn.CloseNow()
	}
	eventually(t, relayTestTimeout, func() bool { return relay.activeWebSockets.Load() == int64(sockets) })

	relay.sampleMemoryPressure(1_000)
	if got := relay.memoryPressureDisconnects.Load(); got != int64(initialShedBatch) {
		t.Fatalf("first batch shed %d sockets, want %d", got, initialShedBatch)
	}

	// Reclaiming nothing doubles the batch, so the rest goes on the next tick.
	relay.sampleMemoryPressure(1_000)
	if got := relay.memoryPressureDisconnects.Load(); got != int64(sockets) {
		t.Fatalf("shed %d sockets across two samples, want %d", got, sockets)
	}
}

func TestMemoryPressureReadmitsAfterRelief(t *testing.T) {
	relay := mustNewRelay(t, pressureConfig(1_000))
	server := httptestServerForRelay(t, relay)
	relay.sampleMemoryPressure(1_000)
	relay.sampleMemoryPressure(0)

	conn := dialRelay(t, server, "pressure-relief", RoleServer, 1, "")
	_ = conn.CloseNow()
	if status, _, _ := getResponse(t, server.URL+"/ready"); status != http.StatusOK {
		t.Fatalf("readiness after relief=%d, want 200", status)
	}
}

func TestMemoryPressureDisabledByZeroWatermark(t *testing.T) {
	relay := mustNewRelay(t, pressureConfig(0))
	server := httptestServerForRelay(t, relay)
	conn := dialRelay(t, server, "pressure-disabled", RoleServer, 1, "")
	defer conn.CloseNow()

	relay.sampleMemoryPressure(1 << 62)

	if relay.memoryPressure.Load() {
		t.Fatal("a zero watermark engaged memory pressure")
	}
	if status, _, _ := getResponse(t, server.URL+"/ready"); status != http.StatusOK {
		t.Fatalf("readiness with pressure disabled=%d, want 200", status)
	}
	if stop := relay.watchMemoryPressure(); stop != nil {
		stop()
	}
}

// TestMemoryPressureSamplerRunsFromStart covers the wiring the unit cases skip:
// the sampler goroutine reading real memory use against the watermark.
func TestMemoryPressureSamplerRunsFromStart(t *testing.T) {
	relay := mustNewRelay(t, pressureConfig(1))
	stop := relay.watchMemoryPressure()
	defer stop()

	if !waitScenario(relay.memoryPressure.Load, 2*time.Second) {
		t.Fatal("sampler did not engage pressure against a 1-byte watermark")
	}
}

func TestHeapInUseReportsNonZero(t *testing.T) {
	if heapInUse() == 0 {
		t.Fatal("heapInUse reported no memory held by the runtime")
	}
}

// TestMemoryPressureReleasesWaitingDelivery covers the part of shedding that
// wakes a source blocked while its data route is unattached.
func TestMemoryPressureReleasesWaitingDelivery(t *testing.T) {
	config := pressureConfig(1_000)
	config.DataAttachTimeoutMS = 10_000
	relay := mustNewRelay(t, config)
	server := httptestServerForRelay(t, relay)
	serverID := "pressure-wait"

	control := dialRelay(t, server, serverID, RoleServer, 2, "")
	defer control.CloseNow()
	client := dialRelay(t, server, serverID, RoleClient, 2, "waiting")
	defer client.CloseNow()

	ctx, cancel := context.WithTimeout(context.Background(), relayTestTimeout)
	defer cancel()
	if err := client.Write(ctx, websocket.MessageBinary, []byte("queued-while-detached")); err != nil {
		t.Fatalf("write waiting frame: %v", err)
	}
	if !waitScenario(func() bool { return relayWaitingMessages(relay, serverID, "waiting") == 1 }, relayTestTimeout) {
		t.Fatal("frame did not block awaiting data")
	}

	relay.sampleMemoryPressure(1_000)

	if !waitScenario(func() bool { return relay.ingressReserved.Load() == 0 }, relayTestTimeout) {
		t.Fatalf("waiting delivery bytes not released by shedding: %d", relay.ingressReserved.Load())
	}
	// The close handshake may still be in progress, so an attached session can
	// briefly remain while its waiting delivery has already been released.
	if !waitScenario(func() bool { return !relayHasSession(relay, serverID) }, relayTestTimeout) {
		relay.mu.Lock()
		session := relay.sessions[serverID]
		waiting := 0
		if session != nil {
			waiting = len(session.waiting["waiting"])
		}
		relay.mu.Unlock()
		if waiting != 0 {
			t.Fatalf("waiting delivery remained after shedding: %d", waiting)
		}
	}
}
