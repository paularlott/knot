package agent_client

import (
	"os"
	"path/filepath"
	"testing"
)

// withProcCgroup sets what /proc/self/cgroup says, and whether the agent
// looks like it runs in a container.
func withProcCgroup(t *testing.T, content string, inContainer bool) {
	t.Helper()
	dir := t.TempDir()
	procSelfCgroup = filepath.Join(dir, "cgroup")
	os.WriteFile(procSelfCgroup, []byte(content), 0o644)
	containerMarkers = nil
	if inContainer {
		marker := filepath.Join(dir, ".dockerenv")
		os.WriteFile(marker, nil, 0o644)
		containerMarkers = []string{marker}
	}
	t.Setenv("container", "")
}

func TestCgroupDirs(t *testing.T) {
	const own = "/docker/abc123"

	// Private cgroup namespace (Docker, Podman on cgroup v2, Apple): the
	// mount is the space's own.
	root := fakeCgroup(t, map[string]string{"cpu.stat": "usage_usec 1\n"})
	withProcCgroup(t, "0::/\n", true)
	if got := cgroupDirs()[""]; got != root {
		t.Errorf("private namespace: dir = %q, want the root %q", got, root)
	}

	// Host cgroup namespace in a container: its own cgroup, below the root.
	root = fakeCgroup(t, map[string]string{"docker/abc123/cpu.stat": "usage_usec 1\n", "cpu.stat": "usage_usec 99\n"})
	withProcCgroup(t, "0::"+own+"\n", true)
	if got, want := cgroupDirs()[""], filepath.Join(root, own); got != want {
		t.Errorf("host namespace in a container: dir = %q, want %q", got, want)
	}
	if usage, _ := readCgroupCPUUsage(); usage != 1000 {
		t.Errorf("host namespace: read %v of CPU, want the container's 1µs, not the host's", usage)
	}

	// An agent run as a service on a host or in a VM: the root, the whole
	// machine, not the service's own cgroup.
	root = fakeCgroup(t, map[string]string{"system.slice/knot-agent.service/cpu.stat": "usage_usec 1\n"})
	withProcCgroup(t, "0::/system.slice/knot-agent.service\n", false)
	if got := cgroupDirs()[""]; got != root {
		t.Errorf("service on a host: dir = %q, want the root %q", got, root)
	}

	// A container whose cgroup isn't visible below the mount (cgroup v1,
	// where the runtime mounts the container's cgroup at the root): the root.
	root = fakeCgroup(t, map[string]string{"cpu,cpuacct/cpuacct.usage": "5\n"})
	withProcCgroup(t, "4:cpu,cpuacct:"+own+"\n", true)
	if got, want := cgroupDirs()["cpuacct"], filepath.Join(root, "cpuacct"); got != want {
		t.Errorf("v1 container: cpuacct dir = %q, want %q", got, want)
	}
	if usage, ok := readCgroupCPUUsage(); !ok || usage != 5 {
		t.Errorf("v1 container: usage = %v, %v; want 5ns", usage, ok)
	}

	// cgroup v1 with the host's hierarchy visible: the container's own.
	root = fakeCgroup(t, map[string]string{"cpu,cpuacct/docker/abc123/cpuacct.usage": "7\n", "cpu,cpuacct/cpuacct.usage": "99\n"})
	withProcCgroup(t, "4:cpu,cpuacct:"+own+"\n6:memory:"+own+"\n", true)
	if usage, ok := readCgroupCPUUsage(); !ok || usage != 7 {
		t.Errorf("v1 host hierarchy: usage = %v, %v; want the container's 7ns", usage, ok)
	}
}

func TestMemoryUsageReadsOwnCgroup(t *testing.T) {
	fakeCgroup(t, map[string]string{
		"docker/abc/memory.current": "1000\n", "docker/abc/memory.max": "max\n",
		"memory.current": "999999\n", "memory.max": "max\n",
	})
	withProcCgroup(t, "0::/docker/abc\n", true)
	used, limit, found := readMemoryUsage()
	if !found || used != 1000 || limit != 0 {
		t.Fatalf("used=%d limit=%d found=%v, want the container's 1000 with no limit", used, limit, found)
	}

	// cgroup v1's "no limit" is a huge number.
	fakeCgroup(t, map[string]string{"memory/memory.usage_in_bytes": "500\n", "memory/memory.limit_in_bytes": "9223372036854771712\n"})
	if used, limit, found := readMemoryUsage(); !found || used != 500 || limit != 0 {
		t.Fatalf("v1 unlimited: used=%d limit=%d found=%v", used, limit, found)
	}

	// No cgroup files: not found, so the caller reads the machine.
	fakeCgroup(t, nil)
	if _, _, found := readMemoryUsage(); found {
		t.Fatal("no cgroup: found")
	}
}
