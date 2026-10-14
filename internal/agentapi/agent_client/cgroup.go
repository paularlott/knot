package agent_client

import (
	"os"
	"path/filepath"
	"strings"
)

// Files the cgroup and container checks read; tests point them at fakes.
var (
	procSelfCgroup   = "/proc/self/cgroup"
	containerMarkers = []string{"/.dockerenv", "/run/.containerenv"}
)

// containerCgroupHints are path segments container runtimes put in the
// cgroup of the containers they run.
var containerCgroupHints = []string{"docker", "libpod", "containerd", "crio", "kubepods", "nomad", "lxc", "podman"}

// cgroupDirs returns, for each cgroup hierarchy, the directory holding the
// space's own cgroup: on cgroup v2 the key is "", on cgroup v1 it's the
// controller directory, such as "cpuacct" or "cpu,cpuacct".
//
// In a container with its own cgroup namespace — the default for Docker
// and Podman on cgroup v2, and Apple containers, which are a VM each — the
// cgroup mount is already the space's own, and /proc/self/cgroup says "/".
// A container that shares the host's cgroup namespace sees every cgroup,
// and /proc/self/cgroup names its own, such as /docker/<id>; that's the one
// to read. Outside a container — a manual space, or an agent run as a
// service in a VM — the root cgroup is the whole machine, which is the
// space, rather than the agent's own service cgroup.
func cgroupDirs() map[string]string {
	dirs := map[string]string{}
	data, err := os.ReadFile(procSelfCgroup)
	if err != nil {
		return dirs
	}
	inContainer := runningInContainer(string(data))
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		controllers, path := parts[1], parts[2]
		for _, dir := range hierarchyDirs(controllers) {
			base := filepath.Join(cgroupRoot, dir)
			own := filepath.Join(base, path)
			if inContainer && path != "/" && own != base && dirExists(own) {
				dirs[dir] = own
			} else {
				dirs[dir] = base
			}
		}
	}
	return dirs
}

// hierarchyDirs returns the directories a /proc/self/cgroup line's
// controllers are mounted at under the cgroup root: "" for cgroup v2, else
// the controller list itself and, for the CPU controllers, the names they
// are commonly mounted under.
func hierarchyDirs(controllers string) []string {
	if controllers == "" {
		return []string{""}
	}
	dirs := []string{controllers}
	switch controllers {
	case "cpu,cpuacct", "cpuacct,cpu":
		dirs = append(dirs, "cpu", "cpuacct", "cpu,cpuacct", "cpuacct,cpu")
	}
	return dirs
}

// runningInContainer reports whether the agent runs in a container, from
// the runtimes' marker files, the container environment variable Podman and
// systemd-nspawn set, or the shape of its cgroup.
func runningInContainer(procCgroup string) bool {
	for _, marker := range containerMarkers {
		if _, err := os.Stat(marker); err == nil {
			return true
		}
	}
	if os.Getenv("container") != "" {
		return true
	}
	for _, hint := range containerCgroupHints {
		if strings.Contains(procCgroup, hint) {
			return true
		}
	}
	return false
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// cgroupFile returns the path of a file in the space's cgroup for the
// hierarchy dir ("" for cgroup v2).
func cgroupFile(dirs map[string]string, dir, name string) string {
	if d, ok := dirs[dir]; ok {
		return filepath.Join(d, name)
	}
	return filepath.Join(cgroupRoot, dir, name)
}
