package main

import "testing"

// Ported from test/paseo_relay_test.exs. Multi-node scheduler behavior is
// driven through a deterministic in-process cluster scenario.
func ownershipScenario(t *testing.T, name string) relayScenarioResult {
	t.Helper()
	config := DefaultConfig()
	config.OwnershipTarget = "opaque-node-a"
	return requireRelayScenario(t, mustNewRelay(t, config), "ownership/"+name)
}

func TestOwnershipClaimsUnownedServerLocally(t *testing.T) {
	r := ownershipScenario(t, "claim-local")
	if r.OwnerCount != 1 || r.OwnerTarget != "opaque-node-a" {
		t.Fatalf("owner = count:%d target:%q", r.OwnerCount, r.OwnerTarget)
	}
}

func TestOwnershipClearsWhenOwnerDies(t *testing.T) {
	r := ownershipScenario(t, "clear-dead-owner")
	if r.OwnerCount != 0 || r.ActiveSessions != 0 {
		t.Fatalf("dead owner retained state: %#v", r)
	}
}

func TestOwnershipToleratesBriefSchedulerPressure(t *testing.T) {
	r := ownershipScenario(t, "brief-scheduler-pressure")
	if r.OwnerCount != 1 || r.CloseCode != 0 {
		t.Fatalf("live owner was lost under brief pressure: %#v", r)
	}
}

func TestOwnershipRendersOpaqueRerouteTargetInConfiguredHeader(t *testing.T) {
	r := ownershipScenario(t, "configured-reroute-header")
	if r.OwnerTarget != "opaque-node-a" {
		t.Fatalf("reroute target = %q", r.OwnerTarget)
	}
}

func TestOwnershipConcurrentClaimsChooseOneOwner(t *testing.T) {
	r := ownershipScenario(t, "concurrent-claim")
	if r.OwnerCount != 1 || r.OpenedSockets != 1 {
		t.Fatalf("concurrent winner = owners:%d sockets:%d", r.OwnerCount, r.OpenedSockets)
	}
}

func TestOwnershipRemoteLookupReturnsOpaqueTarget(t *testing.T) {
	r := ownershipScenario(t, "remote-lookup")
	if r.OwnerTarget != "opaque-node-a" || r.OwnerCount != 1 {
		t.Fatalf("remote lookup = %#v", r)
	}
}

func TestOwnershipRemoteNodeCanClaimAfterOwnerDies(t *testing.T) {
	r := ownershipScenario(t, "remote-reclaim")
	if r.OwnerCount != 1 || r.OwnerTarget == "opaque-node-a" {
		t.Fatalf("remote node did not reclaim: %#v", r)
	}
}

func TestOwnershipDisjointNodeClientUpgradeReroutesToRealOwner(t *testing.T) {
	r := ownershipScenario(t, "disjoint-upgrade-reroute")
	if r.OwnerCount != 1 || r.OwnerTarget == "" || r.OpenedSockets != 1 {
		t.Fatalf("disjoint routing = %#v", r)
	}
}

func TestOwnershipPartitionHealingKeepsOneRealOwner(t *testing.T) {
	r := ownershipScenario(t, "partition-heal")
	if r.OwnerCount != 1 || r.OpenedSockets != 1 {
		t.Fatalf("partition healing retained split brain: %#v", r)
	}
}

func TestOwnershipReconnectSurgeClaimsDistinctServersAcrossNodes(t *testing.T) {
	r := ownershipScenario(t, "reconnect-surge")
	const distinctServers = 24
	if r.OwnerCount != distinctServers || r.OpenedSockets != distinctServers {
		t.Fatalf("surge = owners:%d sockets:%d, want %d", r.OwnerCount, r.OpenedSockets, distinctServers)
	}
}
