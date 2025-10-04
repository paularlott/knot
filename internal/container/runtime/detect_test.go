package runtime

import (
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

func TestDetectLocalContainerRuntime(t *testing.T) {
	tests := []struct {
		name        string
		preferences []string
	}{
		{
			name:        "default preferences",
			preferences: []string{},
		},
		{
			name:        "docker first",
			preferences: []string{model.PlatformDocker, model.PlatformPodman},
		},
		{
			name:        "podman first",
			preferences: []string{model.PlatformPodman, model.PlatformDocker},
		},
		{
			name:        "apple only",
			preferences: []string{model.PlatformApple},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectLocalContainerRuntime(tt.preferences)
			// Result can be empty or one of the valid platforms
			if result != "" && 
				result != model.PlatformDocker && 
				result != model.PlatformPodman && 
				result != model.PlatformApple {
				t.Errorf("Unexpected runtime detected: %s", result)
			}
		})
	}
}

func TestIsRuntimeAvailable(t *testing.T) {
	tests := []struct {
		name    string
		runtime string
	}{
		{
			name:    "docker",
			runtime: model.PlatformDocker,
		},
		{
			name:    "podman",
			runtime: model.PlatformPodman,
		},
		{
			name:    "apple",
			runtime: model.PlatformApple,
		},
		{
			name:    "invalid runtime",
			runtime: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRuntimeAvailable(tt.runtime)
			// Just verify it returns a boolean without error
			_ = result
		})
	}
}
