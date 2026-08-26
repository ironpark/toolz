package main

import "testing"

// Ported from test/load_client_test.exs. Durations and socket counts are kept
// bounded for unit CI; the protocol roles, cleanup, replacement, and ownership
// invariants are unchanged.
func loadScenario(t *testing.T, name string) relayScenarioResult {
	t.Helper()
	return requireRelayScenario(t, NewRelay(DefaultConfig()), "load/"+name)
}

func TestLoadClientDocumentsGenericV2WebSocketRoles(t *testing.T) {
	r := loadScenario(t, "generic-v2-roles")
	if !r.ControlSocketUsed || r.OpenedSockets < 3 {
		t.Fatalf("v2 roles not exercised: %#v", r)
	}
}

func TestLoadClientHasNoProviderStagingCoordinator(t *testing.T) {
	r := loadScenario(t, "no-provider-coordinator")
	if r.OwnerTarget != "generic" {
		t.Fatalf("load client is provider-specific: target=%q", r.OwnerTarget)
	}
}

func TestLoadClientFailedSetupClosesSiblingSocketThatOpensLater(t *testing.T) {
	r := loadScenario(t, "failed-setup-late-sibling")
	if r.OpenedSockets != 2 || r.ActiveWebSockets != 0 || r.CleanupFailures != 0 {
		t.Fatalf("failed setup leaked sibling: %#v", r)
	}
}

func TestLoadClientRampedSustainedRunRelaysFramesAndCleansUp(t *testing.T) {
	r := loadScenario(t, "ramped-sustained")
	if r.FramesForwarded == 0 || r.BytesForwarded == 0 || r.ActiveWebSockets != 0 || r.CleanupFailures != 0 {
		t.Fatalf("sustained run = %#v", r)
	}
}

func TestLoadClientShardedRunCanOmitSharedControlSocket(t *testing.T) {
	r := loadScenario(t, "sharded-no-control")
	if r.ControlSocketUsed || r.FramesForwarded == 0 || r.CleanupFailures != 0 {
		t.Fatalf("sharded run = %#v", r)
	}
}

func TestLoadClientSustainedTrafficExercisesControlWithValidFrames(t *testing.T) {
	r := loadScenario(t, "sustained-control")
	if !r.ControlSocketUsed || r.FramesForwarded == 0 || r.CloseCode != 0 {
		t.Fatalf("control traffic = %#v", r)
	}
}

func TestLoadClientSignaledRunHoldsSocketsBeforePublishing(t *testing.T) {
	r := loadScenario(t, "signaled-hold")
	if r.OpenedSockets == 0 || r.ActiveWebSockets != 0 || r.CleanupFailures != 0 {
		t.Fatalf("signaled run lifecycle = %#v", r)
	}
}

func TestLoadClientReplacementRunReusesServerIDWithCleanTraffic(t *testing.T) {
	r := loadScenario(t, "replacement-server-id")
	if r.FramesForwarded < 2 || !r.ControlSocketUsed || r.ActiveSessions != 0 || r.CleanupFailures != 0 {
		t.Fatalf("replacement run = %#v", r)
	}
}

func TestLoadClientOwnershipSurgeOpensOneSocketPerDistinctServer(t *testing.T) {
	r := loadScenario(t, "ownership-surge")
	const distinctServers = 16
	if r.OpenedSockets != distinctServers || r.OwnerCount != distinctServers || r.CleanupFailures != 0 {
		t.Fatalf("ownership surge = sockets:%d owners:%d cleanup:%d", r.OpenedSockets, r.OwnerCount, r.CleanupFailures)
	}
}
