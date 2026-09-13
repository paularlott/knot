package runtime

import (
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
)

func TestDetectLocalContainerRuntime(t *testing.T) {
	// Preferences come from server config at refresh time; probe each
	// ordering and confirm the snapshot's preferred runtime is a valid
	// platform (or none at all).
	for _, prefs := range [][]string{
		{},
		{model.PlatformDocker, model.PlatformPodman},
		{model.PlatformPodman, model.PlatformDocker},
		{model.PlatformApple},
	} {
		config.SetServerConfig(&config.ServerConfig{LocalContainerRuntimePref: prefs})
		refreshSnapshot()

		result := DetectLocalContainerRuntime()
		if result != "" &&
			result != model.PlatformDocker &&
			result != model.PlatformPodman &&
			result != model.PlatformApple {
			t.Errorf("Unexpected runtime detected: %s", result)
		}

		all := DetectAllAvailableRuntimesWithKVM()
		if result != "" {
			found := false
			for _, rt := range all {
				if rt == result {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("preferred runtime %q missing from all-runtimes %v", result, all)
			}
		}
		// KVM membership matches DetectKVMAvailable exactly — one snapshot.
		hasKVM := false
		for _, rt := range all {
			if rt == model.PlatformKvm {
				hasKVM = true
			}
		}
		if hasKVM != DetectKVMAvailable() {
			t.Error("KVM membership disagrees with DetectKVMAvailable")
		}
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
