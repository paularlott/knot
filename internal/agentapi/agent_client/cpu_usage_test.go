package agent_client

import (
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// fakeCgroup points cgroupRoot at a temporary tree holding files.
func fakeCgroup(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	prev, prevProc, prevMarkers := cgroupRoot, procSelfCgroup, containerMarkers
	cgroupRoot = root
	// Hermetic: no /proc/self/cgroup or container markers from the machine
	// running the tests.
	procSelfCgroup = filepath.Join(root, "no-proc-self-cgroup")
	containerMarkers = nil
	t.Cleanup(func() { cgroupRoot, procSelfCgroup, containerMarkers = prev, prevProc, prevMarkers })
	return root
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.001 }

func TestCountCPUList(t *testing.T) {
	for list, want := range map[string]int{"0": 1, "0-14": 15, "0-3,8,10-11": 7, "": 0, "3-1": 0, "x": 0} {
		if got := countCPUList(list); got != want {
			t.Errorf("countCPUList(%q) = %d, want %d", list, got, want)
		}
	}
}

func TestCPUAllowance(t *testing.T) {
	// cgroup v2 with a quota of 1.5 CPUs
	fakeCgroup(t, map[string]string{"cpu.max": "150000 100000\n", "cpuset.cpus.effective": "0-7\n"})
	if got := cpuAllowance(); !near(got, 1.5) {
		t.Errorf("v2 quota: allowance = %v, want 1.5", got)
	}

	// cgroup v2, no quota: the CPUs it may run on
	fakeCgroup(t, map[string]string{"cpu.max": "max 100000\n", "cpuset.cpus.effective": "0-3,6\n"})
	if got := cpuAllowance(); got != 5 {
		t.Errorf("v2 cpuset: allowance = %v, want 5", got)
	}

	// cgroup v1 quota
	fakeCgroup(t, map[string]string{"cpu/cpu.cfs_quota_us": "200000\n", "cpu/cpu.cfs_period_us": "100000\n"})
	if got := cpuAllowance(); got != 2 {
		t.Errorf("v1 quota: allowance = %v, want 2", got)
	}

	// cgroup v1, unlimited (-1), no cpuset: every CPU
	fakeCgroup(t, map[string]string{"cpu/cpu.cfs_quota_us": "-1\n", "cpu/cpu.cfs_period_us": "100000\n"})
	if got := cpuAllowance(); got != float64(runtime.NumCPU()) {
		t.Errorf("v1 unlimited: allowance = %v, want %d", got, runtime.NumCPU())
	}
}

func TestCPUSamplerV2(t *testing.T) {
	root := fakeCgroup(t, map[string]string{
		"cpu.stat":              "usage_usec 1000000\nuser_usec 800000\nsystem_usec 200000\n",
		"cpu.max":               "max 100000\n",
		"cpuset.cpus.effective": "0-3\n",
	})
	var s cpuSampler
	start := time.Unix(1000, 0)
	if _, _, ok := s.sample(start); ok {
		t.Fatal("the first sample has nothing to compare with")
	}

	// One core busy for two seconds: 2s of CPU time over 2s.
	os.WriteFile(filepath.Join(root, "cpu.stat"), []byte("usage_usec 3000000\n"), 0o644)
	cores, percent, ok := s.sample(start.Add(2 * time.Second))
	if !ok || !near(cores, 1) || !near(percent, 25) {
		t.Fatalf("one busy core of 4: cores=%v percent=%v ok=%v, want 1 and 25%%", cores, percent, ok)
	}

	// Idle: no more CPU time.
	cores, percent, ok = s.sample(start.Add(4 * time.Second))
	if !ok || cores != 0 || percent != 0 {
		t.Fatalf("idle: cores=%v percent=%v ok=%v", cores, percent, ok)
	}

	// A reset counter (the cgroup was recreated) skips a sample.
	os.WriteFile(filepath.Join(root, "cpu.stat"), []byte("usage_usec 10\n"), 0o644)
	if _, _, ok := s.sample(start.Add(6 * time.Second)); ok {
		t.Fatal("a counter that went backwards must not produce a rate")
	}
}

func TestCPUSamplerV1AndCap(t *testing.T) {
	root := fakeCgroup(t, map[string]string{
		"cpuacct/cpuacct.usage": "0\n",
		"cpu/cpu.cfs_quota_us":  "50000\n",
		"cpu/cpu.cfs_period_us": "100000\n",
	})
	var s cpuSampler
	start := time.Unix(1000, 0)
	s.sample(start)

	// Half a core used against a half-core quota: 100%.
	os.WriteFile(filepath.Join(root, "cpuacct/cpuacct.usage"), []byte("1000000000\n"), 0o644) // 1s in ns
	cores, percent, ok := s.sample(start.Add(2 * time.Second))
	if !ok || !near(cores, 0.5) || !near(percent, 100) {
		t.Fatalf("v1: cores=%v percent=%v ok=%v, want 0.5 and 100%%", cores, percent, ok)
	}

	// Bursting past the quota is shown as 100%, not more.
	os.WriteFile(filepath.Join(root, "cpuacct/cpuacct.usage"), []byte("4000000000\n"), 0o644)
	if _, percent, _ := s.sample(start.Add(4 * time.Second)); percent != 100 {
		t.Fatalf("over quota: percent=%v, want 100", percent)
	}
}

func TestNoCgroup(t *testing.T) {
	fakeCgroup(t, nil)
	if _, found := readCgroupCPUUsage(); found {
		t.Fatal("an empty tree has no CPU usage")
	}
}
