package agent_server

import (
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

func TestShouldRestartOnAgentLoss(t *testing.T) {
	tests := []struct {
		name     string
		template *model.Template
		want     bool
	}{
		{
			name: "agent auto restart on local container",
			template: &model.Template{
				Platform:               model.PlatformDocker,
				HealthCheckType:        model.HealthCheckAgent,
				HealthCheckAutoRestart: true,
			},
			want: true,
		},
		{
			name: "agent auto restart on nomad",
			template: &model.Template{
				Platform:               model.PlatformNomad,
				HealthCheckType:        model.HealthCheckAgent,
				HealthCheckAutoRestart: true,
			},
			want: true,
		},
		{
			name: "agent without auto restart",
			template: &model.Template{
				Platform:               model.PlatformDocker,
				HealthCheckType:        model.HealthCheckAgent,
				HealthCheckAutoRestart: false,
			},
			want: false,
		},
		{
			name: "tcp auto restart",
			template: &model.Template{
				Platform:               model.PlatformDocker,
				HealthCheckType:        model.HealthCheckTCP,
				HealthCheckAutoRestart: true,
			},
			want: true,
		},
		{
			name: "none health check with auto restart",
			template: &model.Template{
				Platform:               model.PlatformDocker,
				HealthCheckType:        model.HealthCheckNone,
				HealthCheckAutoRestart: true,
			},
			want: false,
		},
		{
			name: "empty health check with auto restart",
			template: &model.Template{
				Platform:               model.PlatformDocker,
				HealthCheckAutoRestart: true,
			},
			want: false,
		},
		{
			name: "manual template",
			template: &model.Template{
				Platform:               model.PlatformManual,
				HealthCheckType:        model.HealthCheckAgent,
				HealthCheckAutoRestart: true,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRestartOnAgentLoss(tt.template); got != tt.want {
				t.Fatalf("shouldRestartOnAgentLoss() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAgentLossDefaults(t *testing.T) {
	if got := agentLossMaxFailures(nil); got != 3 {
		t.Fatalf("agentLossMaxFailures(nil) = %d, want 3", got)
	}

	if got := agentLossMaxFailures(&model.Template{HealthCheckMaxFailures: 5}); got != 5 {
		t.Fatalf("agentLossMaxFailures(template) = %d, want 5", got)
	}

	if got := agentLossCheckInterval(nil); got.String() != "30s" {
		t.Fatalf("agentLossCheckInterval(nil) = %s, want 30s", got)
	}

	if got := agentLossCheckInterval(&model.Template{HealthCheckInterval: 7}); got.String() != "7s" {
		t.Fatalf("agentLossCheckInterval(template) = %s, want 7s", got)
	}
}

func TestAgentLossFailureCounter(t *testing.T) {
	const spaceId = "space-agent-loss-counter"

	clearAgentLossFailures(spaceId)
	if got := recordAgentLossFailure(spaceId); got != 1 {
		t.Fatalf("first failure = %d, want 1", got)
	}
	if got := recordAgentLossFailure(spaceId); got != 2 {
		t.Fatalf("second failure = %d, want 2", got)
	}
	clearAgentLossFailures(spaceId)
	if got := recordAgentLossFailure(spaceId); got != 1 {
		t.Fatalf("failure after clear = %d, want 1", got)
	}
	clearAgentLossFailures(spaceId)
}
