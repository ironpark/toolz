package main

// The scenario boundary remains red until scheduler, transport, cluster, and
// memory faults can be driven against production state. It must never fabricate
// the expected result merely to make a compatibility assertion pass.
import (
	"fmt"
	"testing"

	"github.com/coder/websocket"
)

func TestScenarioDriverDoesNotFabricateCompatibilityResults(t *testing.T) {
	result, err := NewRelay(DefaultConfig()).testRunScenario("ownership/claim-local")
	if err == nil {
		t.Fatalf("scenario driver fabricated a successful result: %#v", result)
	}
}

func (r *Relay) testRunScenario(name string) (relayScenarioResult, error) {
	return relayScenarioResult{}, fmt.Errorf("production-backed scenario driver is not implemented: %s", name)
}

func (r *Relay) testStallOwner(id string) (func(), bool) {
	r.mu.Lock()
	r.stalled[id] = true
	s := r.sessions[id]
	r.mu.Unlock()
	return func() {}, s != nil
}
func (r *Relay) testKillOwner(id string) bool {
	r.mu.Lock()
	s := r.sessions[id]
	r.mu.Unlock()
	if s == nil {
		return false
	}
	if s.control != nil {
		_ = s.control.conn.Close(websocket.StatusServiceRestart, "Session owner moved")
	}
	return true
}
func (r *Relay) testMoveOwner(id string) bool {
	r.mu.Lock()
	r.moved[id] = true
	s := r.sessions[id]
	r.mu.Unlock()
	return s != nil
}
