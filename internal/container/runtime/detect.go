package runtime

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
)

// Detection shells out to the runtime CLI and logs its result, and callers
// hit it per operation (boot cleanup, reconcile, client creation) — without
// caching a boot with N local-container spaces probes and logs N times over.
// Results are cached briefly so a runtime that appears or disappears mid-run
// (someone starts Docker later) is still picked up.
const detectCacheTTL = 30 * time.Second

var (
	detectMu       sync.Mutex
	detectCache    = map[string]detectCacheEntry{}
	detectAllCache = map[string]detectAllCacheEntry{}
)

type detectCacheEntry struct {
	value   string
	expires time.Time
}

type detectAllCacheEntry struct {
	value   []string
	expires time.Time
}

func normalizePreferences(preferences []string) []string {
	if len(preferences) == 0 {
		return []string{model.PlatformDocker, model.PlatformPodman, model.PlatformApple}
	}
	return preferences
}

func cacheKey(preferences []string) string {
	return strings.Join(normalizePreferences(preferences), ",")
}

// DetectLocalContainerRuntime detects which local container runtime is available
// based on the preference order specified in config and checks if the daemon is running
func DetectLocalContainerRuntime(preferences []string) string {
	key := cacheKey(preferences)

	detectMu.Lock()
	defer detectMu.Unlock()

	if entry, ok := detectCache[key]; ok && time.Now().Before(entry.expires) {
		return entry.value
	}

	runtime := detectLocalContainerRuntime(normalizePreferences(preferences))
	detectCache[key] = detectCacheEntry{value: runtime, expires: time.Now().Add(detectCacheTTL)}
	return runtime
}

func detectLocalContainerRuntime(preferences []string) string {
	for _, runtime := range preferences {
		if isRuntimeAvailable(runtime) {
			log.WithGroup("server").Info("detected local container runtime:", "runtime", runtime)
			return runtime
		}
	}

	log.Warn("No local container runtime detected")
	return ""
}

// DetectAllAvailableRuntimes returns all available container runtimes that are running
// Only runtimes listed in preferences are checked
func DetectAllAvailableRuntimes(preferences []string) []string {
	key := cacheKey(preferences)

	detectMu.Lock()
	defer detectMu.Unlock()

	if entry, ok := detectAllCache[key]; ok && time.Now().Before(entry.expires) {
		// Copy so callers can't mutate the cached slice.
		out := make([]string, len(entry.value))
		copy(out, entry.value)
		return out
	}

	runtimes := []string{}
	for _, rt := range normalizePreferences(preferences) {
		if isRuntimeAvailable(rt) {
			runtimes = append(runtimes, rt)
		}
	}
	detectAllCache[key] = detectAllCacheEntry{value: runtimes, expires: time.Now().Add(detectCacheTTL)}

	out := make([]string, len(runtimes))
	copy(out, runtimes)
	return out
}

// isRuntimeAvailable checks if a specific runtime is available and running
func isRuntimeAvailable(runtime string) bool {
	var cmd *exec.Cmd

	switch runtime {
	case model.PlatformDocker:
		cmd = exec.Command("docker", "info")
	case model.PlatformPodman:
		cmd = exec.Command("podman", "info")
	case model.PlatformApple:
		cmd = exec.Command("container", "system", "status")
	default:
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd = exec.CommandContext(ctx, cmd.Args[0], cmd.Args[1:]...)
	err := cmd.Run()
	return err == nil
}
