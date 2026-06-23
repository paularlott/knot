package agent_client

import (
	"testing"

	knotscriptling "github.com/paularlott/knot/internal/scriptling"
)

func TestUpdateHealthCheckStateCountsFailures(t *testing.T) {
	state := &spaceHealthState{}

	unhealthy, failures, wasUnhealthy := updateHealthCheckState(state, &knotscriptling.HealthCheckResult{Healthy: false})
	if !unhealthy || failures != 1 || wasUnhealthy {
		t.Fatalf("first failure = unhealthy %v failures %d wasUnhealthy %v, want true 1 false", unhealthy, failures, wasUnhealthy)
	}

	unhealthy, failures, wasUnhealthy = updateHealthCheckState(state, &knotscriptling.HealthCheckResult{Healthy: false})
	if !unhealthy || failures != 2 || !wasUnhealthy {
		t.Fatalf("second failure = unhealthy %v failures %d wasUnhealthy %v, want true 2 true", unhealthy, failures, wasUnhealthy)
	}
}

func TestUpdateHealthCheckStateResetsOnRecovery(t *testing.T) {
	state := &spaceHealthState{}

	updateHealthCheckState(state, &knotscriptling.HealthCheckResult{Healthy: false})
	unhealthy, failures, wasUnhealthy := updateHealthCheckState(state, &knotscriptling.HealthCheckResult{Healthy: true})
	if unhealthy || failures != 1 || !wasUnhealthy {
		t.Fatalf("recovery = unhealthy %v failures %d wasUnhealthy %v, want false 1 true", unhealthy, failures, wasUnhealthy)
	}
	if state.consecutiveFailures != 0 || state.wasUnhealthy {
		t.Fatalf("state after recovery = failures %d wasUnhealthy %v, want 0 false", state.consecutiveFailures, state.wasUnhealthy)
	}
}

func TestUpdateHealthCheckStateHealthyNilDoesNotIncrement(t *testing.T) {
	state := &spaceHealthState{}

	unhealthy, failures, wasUnhealthy := updateHealthCheckState(state, nil)
	if unhealthy || failures != 0 || wasUnhealthy {
		t.Fatalf("nil result = unhealthy %v failures %d wasUnhealthy %v, want false 0 false", unhealthy, failures, wasUnhealthy)
	}

	unhealthy, failures, wasUnhealthy = updateHealthCheckState(state, &knotscriptling.HealthCheckResult{Healthy: true})
	if unhealthy || failures != 0 || wasUnhealthy {
		t.Fatalf("healthy result = unhealthy %v failures %d wasUnhealthy %v, want false 0 false", unhealthy, failures, wasUnhealthy)
	}
}
