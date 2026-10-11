package agent_client

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/paularlott/knot/internal/database/model"
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

// The built-in checks pass the template's skip-SSL and timeout settings by
// keyword, so they must reach the check rather than fall back to defaults.
func TestBuiltinHealthChecksHonourSettings(t *testing.T) {
	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer tlsSrv.Close()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	for _, tc := range []struct {
		name    string
		hcType  string
		config  string
		skipSSL bool
		timeout uint32
		healthy bool
	}{
		{"https with a self-signed cert, verify", model.HealthCheckHTTP, tlsSrv.URL, false, 5, false},
		{"https with a self-signed cert, skip verify", model.HealthCheckHTTP, tlsSrv.URL, true, 5, true},
		{"open tcp port", model.HealthCheckTCP, strconv.Itoa(port), false, 5, true},
		{"program that succeeds", model.HealthCheckProgram, "true", false, 5, true},
		{"program slower than the timeout", model.HealthCheckProgram, "sleep 5", false, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := runHealthCheckScript(buildHealthCheckScript(tc.hcType, tc.config, tc.skipSSL, tc.timeout))
			if result == nil || result.Healthy != tc.healthy {
				t.Errorf("healthy = %v, want %v", result, tc.healthy)
			}
		})
	}
}
