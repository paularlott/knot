package agent_client

import (
	"os"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/mem"
)

// collectResourceUsage returns the space's CPU use, as cores and as a
// percentage of the CPU it may use, and its memory and disk use and limits.
func (c *AgentClient) collectResourceUsage() (float64, float64, uint64, uint64, uint64, uint64) {
	cpuCores, cpuPercent := spaceCPUUsage()

	// The space's own memory use, from its cgroup; with no memory limit,
	// what the machine has is the limit. Where there's no cgroup to read,
	// such as the root of a VM or a manual host, the machine is the space.
	memoryUsed, memoryLimit, found := readMemoryUsage()
	if !found || memoryLimit == 0 {
		if vm, err := mem.VirtualMemory(); err == nil && vm != nil {
			if !found {
				memoryUsed = vm.Used
			}
			memoryLimit = vm.Total
		}
	}

	diskUsed, diskLimit := readDiskUsage()

	return cpuCores, cpuPercent, memoryUsed, memoryLimit, diskUsed, diskLimit
}

// readMemoryUsage reads the space's memory use and limit from its own
// cgroup (see cgroupDirs).
func readMemoryUsage() (used, limit uint64, found bool) {
	dirs := cgroupDirs()
	if used, limit, ok := readCgroupV2Memory(dirs); ok {
		return used, limit, true
	}
	if used, limit, ok := readCgroupV1Memory(dirs); ok {
		return used, limit, true
	}
	return 0, 0, false
}

func readCgroupV2Memory(dirs map[string]string) (uint64, uint64, bool) {
	usedBytes, errUsed := os.ReadFile(cgroupFile(dirs, "", "memory.current"))
	limitBytes, errLimit := os.ReadFile(cgroupFile(dirs, "", "memory.max"))
	if errUsed != nil || errLimit != nil {
		return 0, 0, false
	}

	used, err := strconv.ParseUint(strings.TrimSpace(string(usedBytes)), 10, 64)
	if err != nil {
		return 0, 0, false
	}

	limitText := strings.TrimSpace(string(limitBytes))
	if limitText == "max" {
		return used, 0, true
	}

	limit, err := strconv.ParseUint(limitText, 10, 64)
	if err != nil {
		return 0, 0, false
	}
	return used, limit, true
}

func readCgroupV1Memory(dirs map[string]string) (uint64, uint64, bool) {
	usedBytes, errUsed := os.ReadFile(cgroupFile(dirs, "memory", "memory.usage_in_bytes"))
	limitBytes, errLimit := os.ReadFile(cgroupFile(dirs, "memory", "memory.limit_in_bytes"))
	if errUsed != nil || errLimit != nil {
		return 0, 0, false
	}

	used, err := strconv.ParseUint(strings.TrimSpace(string(usedBytes)), 10, 64)
	if err != nil {
		return 0, 0, false
	}

	limit, err := strconv.ParseUint(strings.TrimSpace(string(limitBytes)), 10, 64)
	if err != nil {
		return 0, 0, false
	}
	// No limit reads as the largest page-aligned value, about 8 EiB.
	if limit >= 1<<62 {
		limit = 0
	}
	return used, limit, true
}
