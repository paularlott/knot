package agent_client

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/paularlott/knot/internal/log"
	"github.com/shirou/gopsutil/v3/disk"
)

// procSelfMountinfo is read to find the filesystem holding the home
// directory; tests point it at a fake.
var procSelfMountinfo = "/proc/self/mountinfo"

// How often the home directory's size is measured on a shared filesystem:
// every minute, or ten times as long as the last measurement took, up to
// fifteen minutes, so a home holding a great many files isn't walked
// constantly. One measurement may take at most homeSizeBudget.
const (
	homeSizeMinInterval = time.Minute
	homeSizeMaxInterval = 15 * time.Minute
	homeSizeBudget      = 60 * time.Second
)

// sharedFSTypes are filesystems that hold a container's files on the host's
// disk, or share a directory from it: their size is the host's, not the
// space's.
var sharedFSTypes = map[string]bool{
	"overlay": true, "aufs": true, "virtiofs": true, "9p": true,
	"nfs": true, "nfs4": true, "cifs": true, "smb3": true,
}

type mountInfo struct {
	point  string // where it's mounted
	root   string // the directory of its filesystem that's mounted there
	fsType string
}

// readDiskUsage returns the disk the space uses and, when it has a disk of
// its own, that disk's size. A VM, an Apple container, a manual host or a
// dedicated volume has a filesystem of its own: its use and size are the
// space's. A container's files on the host's disk — its overlay root, or a
// host directory or local volume mounted as home — share the host's
// filesystem, whose figures say nothing about the space: there the home
// directory is measured instead, and there's no limit to report.
func readDiskUsage() (uint64, uint64) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "/"
	}
	for home != "/" {
		if _, err := os.Stat(home); err == nil {
			break
		}
		home = filepath.Dir(home)
	}
	if resolved, err := filepath.EvalSymlinks(home); err == nil {
		home = resolved
	}

	if m, ok := mountFor(home); ok && m.shared() {
		return homeSize.get(home), 0
	}

	usage, err := disk.Usage(home)
	if err != nil || usage == nil {
		log.WithError(err).Debug("failed to get disk usage for resource telemetry", "path", home)
		return 0, 0
	}
	return usage.Used, usage.Total
}

// shared reports whether the mount is part of a bigger filesystem — a
// subdirectory of it, such as a bind-mounted host directory or a Docker
// local volume — or a filesystem type that lives on, or shares from, the
// host.
func (m mountInfo) shared() bool {
	return m.root != "/" || sharedFSTypes[m.fsType] || strings.HasPrefix(m.fsType, "fuse")
}

// mountFor returns the mount holding path: the one with the longest mount
// point that contains it.
func mountFor(path string) (mountInfo, bool) {
	data, err := os.ReadFile(procSelfMountinfo)
	if err != nil {
		return mountInfo{}, false
	}
	var best mountInfo
	found := false
	for _, line := range strings.Split(string(data), "\n") {
		m, ok := parseMountInfo(line)
		if !ok || !pathWithin(path, m.point) {
			continue
		}
		if !found || len(m.point) >= len(best.point) {
			best, found = m, true
		}
	}
	return best, found
}

// parseMountInfo reads a /proc/self/mountinfo line:
// id parent major:minor root mountpoint options [optional...] - fstype source superoptions
func parseMountInfo(line string) (mountInfo, bool) {
	before, after, ok := strings.Cut(line, " - ")
	if !ok {
		return mountInfo{}, false
	}
	fields := strings.Fields(before)
	tail := strings.Fields(after)
	if len(fields) < 5 || len(tail) < 1 {
		return mountInfo{}, false
	}
	return mountInfo{root: unescapeMount(fields[3]), point: unescapeMount(fields[4]), fsType: tail[0]}, true
}

// unescapeMount undoes the octal escapes (\040 for a space) in mountinfo
// paths.
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func pathWithin(path, dir string) bool {
	return dir == "/" || path == dir || strings.HasPrefix(path, strings.TrimSuffix(dir, "/")+"/")
}

// dirSizer measures a directory's size in the background, at most every
// homeSizeInterval, and returns the last measurement straight away.
type dirSizer struct {
	mu         sync.Mutex
	path       string
	size       uint64
	measuredAt time.Time
	interval   time.Duration // until the next measurement
	running    bool
}

var homeSize dirSizer

func (d *dirSizer) get(path string) uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.path != path {
		d.path, d.size, d.measuredAt = path, 0, time.Time{}
	}
	if !d.running && time.Since(d.measuredAt) >= d.interval {
		d.running = true
		go d.measure(path)
	}
	return d.size
}

func (d *dirSizer) measure(path string) {
	start := time.Now()
	size := measureDir(path, start.Add(homeSizeBudget))
	took := time.Since(start)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.running = false
	if d.path == path {
		d.size, d.measuredAt = size, time.Now()
		d.interval = min(max(10*took, homeSizeMinInterval), homeSizeMaxInterval)
	}
}

// measureDir adds up the disk space the files below path take, staying on
// path's filesystem and counting hard-linked files once. It stops at the
// deadline with what it has counted so far.
func measureDir(path string, deadline time.Time) uint64 {
	rootDev, ok := fileDevice(path)
	if !ok {
		return 0
	}
	var total uint64
	seen := map[fileID]bool{}
	filepath.WalkDir(path, func(p string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable: skip it
		}
		if time.Now().After(deadline) {
			return filepath.SkipAll
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		id, blocks, ok := fileUsage(info)
		if !ok {
			total += uint64(info.Size())
			return nil
		}
		if id.dev != rootDev {
			if entry.IsDir() {
				return filepath.SkipDir // another filesystem mounted below
			}
			return nil
		}
		if id.links > 1 {
			key := fileID{dev: id.dev, ino: id.ino}
			if seen[key] {
				return nil
			}
			seen[key] = true
		}
		total += blocks
		return nil
	})
	return total
}

// fileID identifies a file on disk; links is how many names it has.
type fileID struct {
	dev, ino uint64
	links    uint64
}

func fileDevice(path string) (uint64, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	id, _, ok := fileUsage(info)
	return id.dev, ok
}
