package agent_client

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/agentapi/msg"
)

func TestCopyToSpaceMkdir(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "a", "b")
	r := handleCopyToSpace(msg.CopyFileMessage{Direction: "to_space", DestPath: dir, Mode: "mkdir", FilePerm: 0700})
	if !r.Success {
		t.Fatal(r.Error)
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("not a folder: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0700 {
		t.Errorf("mode %o, want 0700", info.Mode().Perm())
	}
	// Again, on a folder that is there: its mode is set.
	if r := handleCopyToSpace(msg.CopyFileMessage{Direction: "to_space", DestPath: dir, Mode: "mkdir", FilePerm: 0750}); !r.Success {
		t.Fatal(r.Error)
	}
	if info, _ := os.Stat(dir); runtime.GOOS != "windows" && info.Mode().Perm() != 0750 {
		t.Errorf("mode %o, want 0750", info.Mode().Perm())
	}
	// Where a file is, it fails.
	f := filepath.Join(root, "file")
	os.WriteFile(f, []byte("x"), 0644)
	if r := handleCopyToSpace(msg.CopyFileMessage{Direction: "to_space", DestPath: f, Mode: "mkdir"}); r.Success {
		t.Error("mkdir over a file succeeded")
	}
}

func TestCopyToSpaceRename(t *testing.T) {
	root := t.TempDir()
	tmp := filepath.Join(root, ".big.knot-upload")
	dest := filepath.Join(root, "sub", "big")
	os.WriteFile(tmp, []byte("new"), 0644)
	// The destination is a symlink: replaced, not written through.
	target := filepath.Join(root, "target")
	os.WriteFile(target, []byte("keep"), 0644)
	os.MkdirAll(filepath.Dir(dest), 0755)
	if err := os.Symlink(target, dest); err != nil {
		t.Skip("symlinks not available:", err)
	}
	mtime := time.Now().Add(-time.Hour).Truncate(time.Second)
	r := handleCopyToSpace(msg.CopyFileMessage{
		Direction: "to_space", DestPath: dest, Mode: "rename", RenameFrom: tmp,
		MtimeNs: mtime.UnixNano(), FilePerm: 0600,
	})
	if !r.Success {
		t.Fatal(r.Error)
	}
	info, err := os.Lstat(dest)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("destination not a file: %v %v", err, info.Mode())
	}
	if got, _ := os.ReadFile(dest); string(got) != "new" {
		t.Errorf("content %q", got)
	}
	if got, _ := os.ReadFile(target); string(got) != "keep" {
		t.Errorf("written through the symlink: %q", got)
	}
	if !info.ModTime().Equal(mtime) || (runtime.GOOS != "windows" && info.Mode().Perm() != 0600) {
		t.Errorf("mtime %v mode %o", info.ModTime(), info.Mode().Perm())
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Error("temp file still there")
	}
	if r := handleCopyToSpace(msg.CopyFileMessage{Direction: "to_space", DestPath: dest, Mode: "rename"}); r.Success {
		t.Error("rename without rename_from succeeded")
	}
}

// A folder a mirror made read-only can still be added to, renamed into and
// removed from by its owner, and keeps its mode.
func TestCopyToSpaceReadOnlyFolder(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("folder modes don't stop the writes")
	}
	root := t.TempDir()
	ro := filepath.Join(root, "ro")
	t.Cleanup(func() { os.Chmod(ro, 0755) })
	if r := handleCopyToSpace(msg.CopyFileMessage{Direction: "to_space", DestPath: ro, Mode: "mkdir", FilePerm: 0555}); !r.Success {
		t.Fatal(r.Error)
	}
	for _, m := range []msg.CopyFileMessage{
		{Direction: "to_space", DestPath: filepath.Join(ro, "new.txt"), Content: []byte("n")},
		{Direction: "to_space", DestPath: filepath.Join(ro, ".big.tmp"), Content: []byte("b"), Mode: "append"},
		{Direction: "to_space", DestPath: filepath.Join(ro, "big"), Mode: "rename", RenameFrom: filepath.Join(ro, ".big.tmp")},
		{Direction: "to_space", DestPath: filepath.Join(ro, "link"), SymlinkTarget: "new.txt"},
		{Direction: "to_space", DestPath: filepath.Join(ro, "inner"), Mode: "mkdir"},
	} {
		if r := handleCopyToSpace(m); !r.Success {
			t.Errorf("%s %s: %s", m.Mode, m.DestPath, r.Error)
		}
	}
	if info, _ := os.Stat(ro); info.Mode().Perm() != 0555 {
		t.Errorf("folder mode %o, want 0555", info.Mode().Perm())
	}
	for _, p := range []string{filepath.Join(ro, "new.txt"), ro} {
		a, b := net.Pipe()
		go func() { handleDeleteFileExecution(a, msg.DeleteFileMessage{Path: p, Recursive: true}); a.Close() }()
		var resp msg.DeleteFileResponse
		if err := msg.ReadMessage(b, &resp); err != nil || !resp.Success {
			t.Errorf("delete %s: %v %s", p, err, resp.Error)
		}
	}
	if _, err := os.Lstat(ro); !os.IsNotExist(err) {
		t.Errorf("read-only tree still there: %v", err)
	}
}

func TestCopyToSpaceChmod(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "run.sh")
	os.WriteFile(f, []byte("x"), 0644)
	if r := handleCopyToSpace(msg.CopyFileMessage{Direction: "to_space", DestPath: f, Mode: "chmod", FilePerm: 0755}); !r.Success {
		t.Fatal(r.Error)
	}
	if info, _ := os.Stat(f); runtime.GOOS != "windows" && info.Mode().Perm() != 0755 {
		t.Errorf("mode %o, want 0755", info.Mode().Perm())
	}
	if b, _ := os.ReadFile(f); string(b) != "x" {
		t.Errorf("content changed: %q", b)
	}
	for name, m := range map[string]msg.CopyFileMessage{
		"missing": {Direction: "to_space", DestPath: filepath.Join(root, "nope"), Mode: "chmod", FilePerm: 0755},
		"no mode": {Direction: "to_space", DestPath: f, Mode: "chmod"},
	} {
		if r := handleCopyToSpace(m); r.Success {
			t.Errorf("%s: succeeded", name)
		}
	}
	link := filepath.Join(root, "link")
	if os.Symlink(f, link) == nil {
		if r := handleCopyToSpace(msg.CopyFileMessage{Direction: "to_space", DestPath: link, Mode: "chmod", FilePerm: 0700}); r.Success {
			t.Error("chmod through a symlink succeeded")
		}
		if info, _ := os.Stat(f); runtime.GOOS != "windows" && info.Mode().Perm() != 0755 {
			t.Errorf("the symlink's target changed: %o", info.Mode().Perm())
		}
	}
}

// A tree holding a folder its owner can't read is still removed.
func TestDeleteTreeWithUnreadableFolder(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("folder modes don't stop reads")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "tree")
	os.MkdirAll(filepath.Join(dir, "locked"), 0755)
	os.WriteFile(filepath.Join(dir, "locked", "f"), []byte("f"), 0644)
	os.Chmod(filepath.Join(dir, "locked"), 0200)
	t.Cleanup(func() { os.Chmod(filepath.Join(dir, "locked"), 0755) })
	a, b := net.Pipe()
	go func() { handleDeleteFileExecution(a, msg.DeleteFileMessage{Path: dir, Recursive: true}); a.Close() }()
	var resp msg.DeleteFileResponse
	if err := msg.ReadMessage(b, &resp); err != nil || !resp.Success {
		t.Fatalf("delete: %v %s", err, resp.Error)
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Errorf("still there: %v", err)
	}
}
