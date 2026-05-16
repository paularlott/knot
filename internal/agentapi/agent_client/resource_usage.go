package agent_client

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/paularlott/knot/internal/log"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

func (c *AgentClient) collectResourceUsage() (float64, uint64, uint64, uint64, uint64) {
	cpuPercent := 0.0
	if values, err := cpu.Percent(0, false); err == nil && len(values) > 0 {
		cpuPercent = values[0]
	}

	memoryUsed, memoryLimit := readMemoryUsage()
	if memoryLimit == 0 {
		if vm, err := mem.VirtualMemory(); err == nil && vm != nil {
			memoryUsed = vm.Used
			memoryLimit = vm.Total
		}
	}

	diskUsed, diskLimit := readDiskUsage()

	return cpuPercent, memoryUsed, memoryLimit, diskUsed, diskLimit
}

func readMemoryUsage() (uint64, uint64) {
	if used, limit, ok := readCgroupV2Memory(); ok {
		return used, limit
	}
	if used, limit, ok := readCgroupV1Memory(); ok {
		return used, limit
	}
	return 0, 0
}

func readCgroupV2Memory() (uint64, uint64, bool) {
	usedBytes, errUsed := os.ReadFile("/sys/fs/cgroup/memory.current")
	limitBytes, errLimit := os.ReadFile("/sys/fs/cgroup/memory.max")
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

func readCgroupV1Memory() (uint64, uint64, bool) {
	usedBytes, errUsed := os.ReadFile("/sys/fs/cgroup/memory/memory.usage_in_bytes")
	limitBytes, errLimit := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes")
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
	return used, limit, true
}

func readDiskUsage() (uint64, uint64) {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		homeDir = "/"
	}

	statPath := homeDir
	for statPath != "/" {
		if _, err := os.Stat(statPath); err == nil {
			break
		}
		statPath = filepath.Dir(statPath)
	}

	usage, err := disk.Usage(statPath)
	if err != nil || usage == nil {
		log.WithError(err).Debug("failed to get disk usage for resource telemetry", "path", statPath)
		return 0, 0
	}

	return usage.Used, usage.Total
}
