package main

import (
	"context"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestCapacityReconcileReleasesOrphanedReservation(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	epoch := relay.capacityEpoch.Load()
	// A reservation no buffer and no live route accounts for: the shape left
	// behind when a teardown path loses its release.
	relay.ingressReserved.Add(4_096)

	relay.reconcileCapacity()

	if got := relay.ingressReserved.Load(); got != 0 {
		t.Fatalf("ingress reserved after reconcile=%d, want 0", got)
	}
	if relay.capacityEpoch.Load() == epoch {
		t.Fatal("reconciling a leak did not advance the capacity epoch")
	}
	if relay.capacityUnavailable.Load() {
		t.Fatal("capacity left unavailable after the mutation completed")
	}
}

func TestCapacityReconcileLeavesInFlightReservationsAlone(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	epoch := relay.capacityEpoch.Load()
	// Mid-route: reserved and published as in-flight, not yet buffered.
	relay.ingressInFlight.Add(4_096)
	relay.ingressReserved.Add(4_096)

	relay.reconcileCapacity()

	if got := relay.ingressReserved.Load(); got != 4_096 {
		t.Fatalf("reconcile reclaimed a live route's reservation: %d, want 4096", got)
	}
	if relay.capacityEpoch.Load() != epoch {
		t.Fatal("reconcile advanced the capacity epoch with nothing to correct")
	}
}

func TestCapacityReconcileLeavesBufferedFramesAlone(t *testing.T) {
	config := DefaultConfig()
	config.DataAttachTimeoutMS = 10_000
	relay := mustNewRelay(t, config)
	server := httptestServerForRelay(t, relay)
	serverID := "reconcile-buffered"

	control := dialRelay(t, server, serverID, RoleServer, 2, "")
	defer control.CloseNow()
	client := dialRelay(t, server, serverID, RoleClient, 2, "buffered")
	defer client.CloseNow()

	ctx, cancel := context.WithTimeout(context.Background(), relayTestTimeout)
	defer cancel()
	if err := client.Write(ctx, websocket.MessageBinary, []byte("still-waiting-for-a-route")); err != nil {
		t.Fatalf("write buffered frame: %v", err)
	}
	if !waitScenario(func() bool { return relay.ingressReserved.Load() > 0 }, relayTestTimeout) {
		t.Fatal("frame was not buffered")
	}
	reserved := relay.ingressReserved.Load()

	relay.reconcileCapacity()

	if got := relay.ingressReserved.Load(); got != reserved {
		t.Fatalf("reconcile reclaimed buffered bytes: %d, want %d", got, reserved)
	}
}

// TestCapacityLedgersBalanceAfterRouting is the invariant the reconciler rests
// on: ordinary forwarding leaves neither ledger holding anything.
func TestCapacityLedgersBalanceAfterRouting(t *testing.T) {
	relay := mustNewRelay(t, DefaultConfig())
	server := httptestServerForRelay(t, relay)
	serverID := "reconcile-balance"

	daemon := dialRelay(t, server, serverID, RoleServer, 1, "")
	defer daemon.CloseNow()
	client := dialRelay(t, server, serverID, RoleClient, 1, "")
	defer client.CloseNow()

	ctx, cancel := context.WithTimeout(context.Background(), relayTestTimeout)
	defer cancel()
	if err := client.Write(ctx, websocket.MessageText, []byte("routed")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, _, err := daemon.Read(ctx); err != nil {
		t.Fatalf("read forwarded frame: %v", err)
	}

	if !waitScenario(func() bool {
		return relay.ingressReserved.Load() == 0 && relay.ingressInFlight.Load() == 0
	}, relayTestTimeout) {
		t.Fatalf("ledgers unbalanced: reserved=%d in-flight=%d",
			relay.ingressReserved.Load(), relay.ingressInFlight.Load())
	}
}

func TestReserveIngressDoesNotLeakInFlightOnRejection(t *testing.T) {
	config := DefaultConfig()
	config.IngressBudgetBytes = 1_024
	relay := mustNewRelay(t, config)

	if relay.reserveIngress(2_048) {
		t.Fatal("reserved beyond the ingress budget")
	}
	if got := relay.ingressInFlight.Load(); got != 0 {
		t.Fatalf("in-flight after a rejected reservation=%d, want 0", got)
	}
	if got := relay.ingressReserved.Load(); got != 0 {
		t.Fatalf("reserved after a rejected reservation=%d, want 0", got)
	}
}

// TestCapacityReconcilerRunsOnItsInterval covers the wiring: the sampler
// goroutine, not just the reconcile step it calls.
func TestCapacityReconcilerRunsOnItsInterval(t *testing.T) {
	config := DefaultConfig()
	config.CapacityMutationTimeoutMS = 20
	relay := mustNewRelay(t, config)
	relay.ingressReserved.Add(8_192)

	stop := relay.watchCapacity()
	defer stop()

	if !waitScenario(func() bool { return relay.ingressReserved.Load() == 0 }, 2*time.Second) {
		t.Fatalf("reconciler did not run: reserved=%d", relay.ingressReserved.Load())
	}
}
