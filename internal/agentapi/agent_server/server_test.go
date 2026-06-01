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
			name: "non-agent health check",
			template: &model.Template{
				Platform:               model.PlatformDocker,
				HealthCheckType:        model.HealthCheckTCP,
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
