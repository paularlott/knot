package runtime

import (
	"context"
	"os/exec"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
)

// DetectLocalContainerRuntime detects which local container runtime is available
// based on the preference order specified in config
func DetectLocalContainerRuntime(preferences []string) string {
	// Default preference order if none specified
	if len(preferences) == 0 {
		preferences = []string{model.PlatformDocker, model.PlatformPodman, model.PlatformApple}
	}

	for _, runtime := range preferences {
		if isRuntimeAvailable(runtime) {
			log.WithGroup("server").Info("detected local container runtime:", "runtime", runtime)
			return runtime
		}
	}

	log.Warn("No local container runtime detected")
	return ""
}

// DetectAllAvailableRuntimes returns all available container runtimes
func DetectAllAvailableRuntimes() []string {
	runtimes := []string{}
	for _, rt := range []string{model.PlatformDocker, model.PlatformPodman, model.PlatformApple} {
		if isRuntimeAvailable(rt) {
			runtimes = append(runtimes, rt)
		}
	}
	return runtimes
}

// isRuntimeAvailable checks if a specific runtime is available
func isRuntimeAvailable(runtime string) bool {
	var cmd *exec.Cmd

	switch runtime {
	case model.PlatformDocker:
		cmd = exec.Command("docker", "--version")
	case model.PlatformPodman:
		cmd = exec.Command("podman", "--version")
	case model.PlatformApple:
		cmd = exec.Command("container", "--version")
	default:
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd = exec.CommandContext(ctx, cmd.Args[0], cmd.Args[1:]...)
	err := cmd.Run()
	return err == nil
}

// GetDetectedRuntime returns the detected runtime from config
func GetDetectedRuntime() string {
	cfg := config.GetServerConfig()
	if cfg == nil {
		return ""
	}
	return cfg.LocalContainerRuntime
}
