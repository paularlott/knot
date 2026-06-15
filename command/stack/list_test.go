package command_stack

import (
	"testing"

	"github.com/paularlott/knot/apiclient"
)

func TestStackSpaceStatus(t *testing.T) {
	tests := []struct {
		name  string
		space apiclient.SpaceInfo
		want  string
	}{
		{name: "running", space: apiclient.SpaceInfo{IsDeployed: true}, want: "Running"},
		{name: "stopping", space: apiclient.SpaceInfo{IsDeployed: true, IsPending: true}, want: "Stopping"},
		{name: "deleting", space: apiclient.SpaceInfo{IsDeleting: true}, want: "Deleting"},
		{name: "starting", space: apiclient.SpaceInfo{IsPending: true}, want: "Starting"},
		{name: "stopped", space: apiclient.SpaceInfo{}, want: "Stopped"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stackSpaceStatus(tt.space); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestStackSpaceHealth(t *testing.T) {
	tests := []struct {
		name  string
		space apiclient.SpaceInfo
		want  string
	}{
		{name: "healthy running", space: apiclient.SpaceInfo{IsDeployed: true, Healthy: true}, want: "Healthy"},
		{name: "unhealthy running", space: apiclient.SpaceInfo{IsDeployed: true, Healthy: false}, want: "Unhealthy"},
		{name: "stopping", space: apiclient.SpaceInfo{IsDeployed: true, IsPending: true, Healthy: true}, want: "-"},
		{name: "starting", space: apiclient.SpaceInfo{IsPending: true, Healthy: true}, want: "-"},
		{name: "stopped", space: apiclient.SpaceInfo{Healthy: true}, want: "-"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stackSpaceHealth(tt.space); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
