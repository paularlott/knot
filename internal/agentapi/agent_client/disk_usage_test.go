package agent_client

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Lines taken from the runtimes: a Docker container (overlay root, a local
// volume and a bind-mounted host directory), and an Apple container.
const dockerMountinfo = `4266 1041 0:57 / / rw,relatime - overlay overlay rw,lowerdir=/x
4268 4266 0:68 / /proc rw,nosuid - proc proc rw
4279 4275 0:38 /docker/volumes/home/_data /home/knot rw,noatime master:69 - btrfs /dev/vdb1 rw
4280 4275 0:38 /Users/paul/src /home/knot/src rw - virtiofs mount0 rw
4281 4275 254:32 / /data rw - ext4 /dev/vdc rw
4282 4275 0:38 / /home/with\040space rw - ext4 /dev/vdd rw
`

const appleMountinfo = `44 32 254:16 / / rw,relatime - ext4 /dev/vdb rw
45 44 0:28 / /proc rw,relatime - proc proc rw
`

func withMountinfo(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mountinfo")
	os.WriteFile(path, []byte(content), 0o644)
	prev := procSelfMountinfo
	procSelfMountinfo = path
	t.Cleanup(func() { procSelfMountinfo = prev })
}

func TestMountFor(t *testing.T) {
	withMountinfo(t, dockerMountinfo)
	cases := []struct {
		path, point string
		shared      bool
	}{
		{"/root", "/", true},                           // the container's overlay root
		{"/home/knot", "/home/knot", true},             // a Docker local volume: part of the host's disk
		{"/home/knot/src/app", "/home/knot/src", true}, // a host directory shared in
		{"/data/x", "/data", false},                    // a disk of its own
		{"/home/with space/a", "/home/with space", false},
	}
	for _, c := range cases {
		m, ok := mountFor(c.path)
		if !ok || m.point != c.point || m.shared() != c.shared {
			t.Errorf("%s: mount %+v ok=%v shared=%v, want %s shared=%v", c.path, m, ok, m.shared(), c.point, c.shared)
		}
	}

	withMountinfo(t, appleMountinfo)
	if m, ok := mountFor("/home/knot"); !ok || m.shared() {
		t.Errorf("apple: %+v, want its own disk", m)
	}
}

func TestMeasureDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a"), make([]byte, 64*1024), 0o644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "b"), make([]byte, 64*1024), 0o644)
	one := measureDir(dir, time.Now().Add(time.Minute))
	if one < 128*1024 {
		t.Fatalf("measured %d bytes, want at least the 128 KiB written", one)
	}

	// A hard link adds a name, not space.
	if err := os.Link(filepath.Join(dir, "a"), filepath.Join(dir, "a-link")); err == nil {
		if two := measureDir(dir, time.Now().Add(time.Minute)); two != one {
			t.Errorf("with a hard link: %d bytes, want %d", two, one)
		}
	}

	// Past the deadline it stops.
	if got := measureDir(dir, time.Now().Add(-time.Second)); got >= one {
		t.Errorf("past the deadline: %d bytes, want less than %d", got, one)
	}
}
