package agent_client

import (
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
)

// cgroupRoot is where the space's cgroup is mounted; tests point it at a
// fake tree.
var cgroupRoot = "/sys/fs/cgroup"

// cpuSampler turns the space's cumulative CPU time into a usage rate between
// samples. Inside a container the cgroup holds the space's own CPU time —
// /proc/stat there is the whole host's — and in a VM or on a manual host the
// root cgroup is the whole machine, which is what the space is.
type cpuSampler struct {
	mu        sync.Mutex
	lastUsage time.Duration // cumulative CPU time at the last sample
	lastAt    time.Time
}

var spaceCPU cpuSampler

// sample returns the CPU the space used since the last sample, in cores, and
// as a percentage of the CPU the space may use: its quota, else the CPUs it
// may run on. ok is false on the first sample and where there's no cgroup to
// read, such as an agent on macOS.
func (s *cpuSampler) sample(now time.Time) (cores, percent float64, ok bool) {
	usage, found := readCgroupCPUUsage()
	if !found {
		return 0, 0, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	prevUsage, prevAt := s.lastUsage, s.lastAt
	s.lastUsage, s.lastAt = usage, now
	if prevAt.IsZero() || usage < prevUsage {
		return 0, 0, false // first sample, or the counter was reset
	}
	elapsed := now.Sub(prevAt)
	if elapsed <= 0 {
		return 0, 0, false
	}

	cores = float64(usage-prevUsage) / float64(elapsed)
	percent = cores / cpuAllowance() * 100
	return cores, min(percent, 100), true
}

// readCgroupCPUUsage returns the cgroup's cumulative CPU time: usage_usec in
// cpu.stat on cgroup v2, cpuacct.usage (nanoseconds) on cgroup v1.
func readCgroupCPUUsage() (time.Duration, bool) {
	dirs := cgroupDirs()
	if data, err := os.ReadFile(cgroupFile(dirs, "", "cpu.stat")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if value, ok := strings.CutPrefix(line, "usage_usec "); ok {
				if usec, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && usec >= 0 && usec <= math.MaxInt64/int64(time.Microsecond) {
					return time.Duration(usec) * time.Microsecond, true
				}
			}
		}
	}
	for _, dir := range []string{"cpuacct", "cpu,cpuacct", "cpuacct,cpu"} {
		if data, err := os.ReadFile(cgroupFile(dirs, dir, "cpuacct.usage")); err == nil {
			if ns, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64); err == nil && ns >= 0 {
				return time.Duration(ns), true
			}
		}
	}
	return 0, false
}

// cpuAllowance is how many CPUs the space may use: its CPU quota, else the
// number of CPUs it may run on, else every CPU.
func cpuAllowance() float64 {
	dirs := cgroupDirs()
	if quota := cgroupCPUQuota(dirs); quota > 0 {
		return quota
	}
	if n := cgroupCPUSetSize(dirs); n > 0 {
		return float64(n)
	}
	return float64(runtime.NumCPU())
}

// cgroupCPUQuota returns the CPU quota in CPUs, 0 for none: cpu.max
// ("<quota> <period>" or "max <period>") on cgroup v2, cpu.cfs_quota_us and
// cpu.cfs_period_us on cgroup v1.
func cgroupCPUQuota(dirs map[string]string) float64 {
	if data, err := os.ReadFile(cgroupFile(dirs, "", "cpu.max")); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) == 2 && fields[0] != "max" {
			quota, errQ := strconv.ParseFloat(fields[0], 64)
			period, errP := strconv.ParseFloat(fields[1], 64)
			if errQ == nil && errP == nil && quota > 0 && period > 0 {
				return quota / period
			}
		}
		return 0
	}
	for _, dir := range []string{"cpu", "cpu,cpuacct", "cpuacct,cpu"} {
		quotaData, errQ := os.ReadFile(cgroupFile(dirs, dir, "cpu.cfs_quota_us"))
		periodData, errP := os.ReadFile(cgroupFile(dirs, dir, "cpu.cfs_period_us"))
		if errQ != nil || errP != nil {
			continue
		}
		quota, errQ := strconv.ParseFloat(strings.TrimSpace(string(quotaData)), 64)
		period, errP := strconv.ParseFloat(strings.TrimSpace(string(periodData)), 64)
		if errQ == nil && errP == nil && quota > 0 && period > 0 {
			return quota / period
		}
		return 0
	}
	return 0
}

// cgroupCPUSetSize returns how many CPUs the cgroup may run on, 0 if unknown.
func cgroupCPUSetSize(dirs map[string]string) int {
	for _, f := range [][2]string{{"", "cpuset.cpus.effective"}, {"cpuset", "cpuset.effective_cpus"}, {"cpuset", "cpuset.cpus"}} {
		if data, err := os.ReadFile(cgroupFile(dirs, f[0], f[1])); err == nil {
			if n := countCPUList(strings.TrimSpace(string(data))); n > 0 {
				return n
			}
		}
	}
	return 0
}

// countCPUList counts the CPUs in a list such as "0-3,8,10-11".
func countCPUList(list string) int {
	n := 0
	for _, part := range strings.Split(list, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lo, hi, isRange := strings.Cut(part, "-")
		first, err := strconv.Atoi(lo)
		if err != nil {
			return 0
		}
		last := first
		if isRange {
			if last, err = strconv.Atoi(hi); err != nil || last < first {
				return 0
			}
		}
		n += last - first + 1
	}
	return n
}

// spaceCPUUsage returns the space's CPU use as cores and as a percentage of
// what it may use. Without a cgroup it falls back to the whole machine's use.
func spaceCPUUsage() (cores, percent float64) {
	if cores, percent, ok := spaceCPU.sample(time.Now()); ok {
		return cores, percent
	}
	if _, found := readCgroupCPUUsage(); found {
		return 0, 0 // the first sample: the next one has a rate
	}
	if values, err := cpu.Percent(0, false); err == nil && len(values) > 0 {
		return values[0] / 100 * float64(runtime.NumCPU()), values[0]
	}
	return 0, 0
}
