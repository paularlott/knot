//go:build integration

package suites

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paularlott/knot/integration-tests/harness"
)

// TestFilesNoSync runs file storage with files.no_sync: the server says at
// start that writes are not forced to disk, files are written and read as
// ever, and what was written is there after the server is killed and
// started again: without the sync only a crash of the machine or a power
// cut loses writes, not one of the server.
func TestFilesNoSync(t *testing.T) {
	harness.Feature(t, "file-storage")
	store := filepath.Join(t.TempDir(), "store")
	s, admin := bootDedicated(t, "nosync", "--files-path", store, "--files-no-sync")
	ctx, cancel := testCtx(120)
	defer cancel()
	if log, _ := os.ReadFile(s.LogPath); !strings.Contains(string(log), "not forced to disk") {
		t.Errorf("the server did not warn that writes are not forced to disk:\n%s", tailLines(string(log), 20))
	}
	if _, err := admin.Client.CreateFileBucket(ctx, "nosync"); err != nil {
		t.Fatal(err)
	}
	small, large := []byte("small"), bytes.Repeat([]byte("L"), 300<<10)
	for key, data := range map[string][]byte{"small.txt": small, "large.bin": large} {
		if _, err := admin.Client.PutFileObjectMeta(ctx, "nosync", key, bytes.NewReader(data), int64(len(data)), "", nil); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := exec.Command("pkill", "-KILL", "-f", "--", store).CombinedOutput(); err != nil {
		t.Fatalf("kill the server: %v %s", err, out)
	}
	if err := s.Restart(); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if got := fetchFile(t, admin.Client, "nosync", "small.txt"); !bytes.Equal(got, small) {
		t.Errorf("small file after a restart reads %q", got)
	}
	if got := fetchFile(t, admin.Client, "nosync", "large.bin"); !bytes.Equal(got, large) {
		t.Errorf("large file after a restart is %d bytes", len(got))
	}
}
