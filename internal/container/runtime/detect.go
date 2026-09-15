package runtime

import (
	"context"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
)

// Runtime availability is maintained by a background refresher: probing
// shells out (docker info, podman info, virsh …) with multi-second timeouts
// when a runtime is absent, so request paths read a snapshot the refresher
// holds rather than probing themselves. A runtime that appears or disappears
// mid-run (someone starts Docker later) is picked up on the next refresh.
const refreshInterval = 30 * time.Second

// snapshot is one refresh's outcome. Preferred is the first available
// container runtime in preference order ("" when none); AllWithKVM is the
// full set including KVM — what nodes gossip and placement filters on.
// Container runtimes stay in preference order so "first available" and the
// ordered list derive from the same probe.
type snapshot struct {
	Preferred  string
	AllWithKVM []string
	kvm        bool
}

var (
	currentSnapshot atomic.Pointer[snapshot]
	refreshOnce     sync.Once
	fallbackOnce    sync.Once
)

// StartBackgroundRefresh probes once synchronously — so boot-time consumers
// such as CleanupOnBoot start with data — then re-probes on an interval for
// the life of the process. Safe to call multiple times; only the first
// starts the refresher. Without it (unit tests, embedded use), reads fall
// back to one synchronous probe on first use.
func StartBackgroundRefresh() {
	refreshOnce.Do(func() {
		refreshSnapshot()
		go func() {
			for {
				time.Sleep(refreshInterval)
				refreshSnapshot()
			}
		}()
	})
}

func refreshSnapshot() {
	defer func() { recover() }() // a probe panic must never take the server down

	// Detection order comes from the enabled-backends allowlist: container
	// backends are probed in the order listed (podman before docker makes
	// podman the auto-detected runtime), with the default order when the
	// list is empty or names no container backend. nomad/kvm entries are
	// not detection candidates and are ignored here.
	prefs := defaultPreferences()
	if cfg := config.GetServerConfig(); cfg != nil {
		if listed := containerBackendsInOrder(cfg.EnabledBackends); len(listed) > 0 {
			prefs = listed
		}
	}

	s := &snapshot{}
	for _, rt := range prefs {
		if isRuntimeAvailable(rt) {
			if s.Preferred == "" {
				log.WithGroup("server").Info("detected local container runtime:", "runtime", rt)
				s.Preferred = rt
			}
			s.AllWithKVM = append(s.AllWithKVM, rt)
		}
	}
	if s.Preferred == "" {
		log.Warn("No local container runtime detected")
	}
	if s.kvm = isKvmAvailable(); s.kvm {
		s.AllWithKVM = append(s.AllWithKVM, model.PlatformKvm)
	}

	currentSnapshot.Store(s)
}

// getSnapshot returns the current refresh result, probing synchronously once
// if the refresher never ran (unit tests, embedded use).
func getSnapshot() *snapshot {
	if s := currentSnapshot.Load(); s != nil {
		return s
	}
	fallbackOnce.Do(func() { currentSnapshot.Store(&snapshot{}) })
	refreshSnapshot()
	return currentSnapshot.Load()
}

func normalizePreferences(preferences []string) []string {
	if len(preferences) == 0 {
		return defaultPreferences()
	}
	return preferences
}

// containerBackendsInOrder filters an enabled-backends list down to the
// container runtimes, preserving the listed order.
func containerBackendsInOrder(enabled []string) []string {
	var out []string
	for _, backend := range enabled {
		switch backend {
		case model.PlatformDocker, model.PlatformPodman, model.PlatformApple:
			out = append(out, backend)
		}
	}
	return out
}

func defaultPreferences() []string {
	return []string{model.PlatformDocker, model.PlatformPodman, model.PlatformApple}
}

// DetectLocalContainerRuntime returns the local container runtime to use —
// the first available in the configured preference order, "" when none is
// running.
func DetectLocalContainerRuntime() string {
	return getSnapshot().Preferred
}

// DetectAllAvailableRuntimes returns the available container runtimes, in
// preference order. KVM is deliberately excluded so container
// auto-detection never picks it — use DetectAllAvailableRuntimesWithKVM.
func DetectAllAvailableRuntimes() []string {
	all := getSnapshot().AllWithKVM
	out := make([]string, 0, len(all))
	for _, rt := range all {
		if rt != model.PlatformKvm {
			out = append(out, rt)
		}
	}
	return out
}

// DetectAllAvailableRuntimesWithKVM returns the available container runtimes
// plus "kvm" when the node can run KVM virtual machines. This is the list
// nodes gossip as their runtimes metadata and that placement filters on.
func DetectAllAvailableRuntimesWithKVM() []string {
	all := getSnapshot().AllWithKVM
	out := make([]string, len(all))
	copy(out, all)
	return out
}

// DetectKVMAvailable reports whether this node can run KVM virtual machines.
func DetectKVMAvailable() bool {
	return getSnapshot().kvm
}

func isKvmAvailable() bool {
	if _, err := os.Stat("/dev/kvm"); err != nil {
		return false
	}
	if _, err := exec.LookPath("virsh"); err != nil {
		return false
	}
	if _, err := exec.LookPath("virt-install"); err != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Reach the daemon, not just the binary — an unreachable libvirtd means
	// no VMs can actually be created here.
	return exec.CommandContext(ctx, "virsh", "--connect", "qemu:///system", "list").Run() == nil
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
