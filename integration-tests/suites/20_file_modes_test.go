//go:build integration

package suites

import (
	"bytes"
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
)

// filesRoot is where a file storage test keeps the server's storage and its
// local folders: under KNOT_TEST_FILES_ROOT when set (to test on another
// disk), else in a temporary folder.
func filesRoot(t *testing.T) string {
	t.Helper()
	base := os.Getenv("KNOT_TEST_FILES_ROOT")
	if base == "" {
		return t.TempDir()
	}
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatalf("KNOT_TEST_FILES_ROOT: %v", err)
	}
	dir, err := os.MkdirTemp(base, "it-file-modes-")
	if err != nil {
		t.Fatalf("KNOT_TEST_FILES_ROOT: %v", err)
	}
	if !cfg.Keep {
		t.Cleanup(func() { os.RemoveAll(dir) })
	}
	return dir
}

// TestFileModes checks that the knot command and the NFS server keep file
// modes: copies and syncs both ways and between buckets, chmods synced on
// their own, a write that gives no mode, a restore from a backup, and a
// bucket mounted over NFS.
func TestFileModes(t *testing.T) {
	harness.Feature(t, "file-storage")
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX modes on Windows")
	}

	root := filesRoot(t)
	s, admin := bootDedicated(t, "filemodes", "--files-path", filepath.Join(root, "store"))
	cli := filepath.Join(root, "cli")
	os.MkdirAll(cli, 0755)
	knot := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(bins.Server, append(args, "--server", s.BaseURL, "--token", admin.Token)...)
		cmd.Dir = cli
		cmd.Env = append(os.Environ(), "HOME="+cli)
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		if err := cmd.Run(); err != nil {
			t.Fatalf("knot %s: %v\n%s", strings.Join(args, " "), err, out.String())
		}
		return out.String()
	}
	ctx, cancel := testCtx(300)
	defer cancel()
	if _, err := admin.Client.CreateFileBucket(ctx, "modes"); err != nil {
		t.Fatalf("create bucket: %v", err)
	}

	write := func(path, content string, mode os.FileMode) {
		t.Helper()
		os.MkdirAll(filepath.Dir(path), 0755)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
	localMode := func(path string) os.FileMode {
		t.Helper()
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		return info.Mode().Perm()
	}
	remoteMode := func(key string) os.FileMode {
		t.Helper()
		info, err := admin.Client.HeadFileObject(ctx, "modes", key)
		if err != nil {
			t.Fatalf("head %s: %v", key, err)
		}
		mode, _ := apiclient.ParseFileMode(info.Meta)
		return mode
	}
	expectLocal := func(path string, want os.FileMode) {
		t.Helper()
		if got := localMode(path); got != want {
			t.Errorf("%s: mode %04o, want %04o", strings.TrimPrefix(path, root), got, want)
		}
	}
	expectRemote := func(key string, want os.FileMode) {
		t.Helper()
		if got := remoteMode(key); got != want {
			t.Errorf("modes:%s: mode %04o, want %04o", key, got, want)
		}
	}
	chmodRemote := func(key string, mode os.FileMode) {
		t.Helper()
		info, err := admin.Client.HeadFileObject(ctx, "modes", key)
		if err != nil {
			t.Fatal(err)
		}
		info.Meta[apiclient.FileModeMeta] = apiclient.FormatFileMode(mode)
		if _, err := admin.Client.CopyFileObject(ctx, apiclient.FileCopyRequest{
			SourceBucket: "modes", SourceKey: key, DestBucket: "modes", DestKey: key, Meta: info.Meta,
		}); err != nil {
			t.Fatal(err)
		}
	}

	// knot file cp, both ways.
	write(filepath.Join(root, "cp", "run.sh"), "#!/bin/sh\necho hi\n", 0755)
	knot("file", "cp", filepath.Join(root, "cp", "run.sh"), "modes:cp/run.sh")
	expectRemote("cp/run.sh", 0755)
	knot("file", "cp", "modes:cp/run.sh", filepath.Join(root, "cp-down", "run.sh"))
	expectLocal(filepath.Join(root, "cp-down", "run.sh"), 0755)

	// knot file sync up, a chmod alone, then down into a new folder.
	up := filepath.Join(root, "up")
	write(filepath.Join(up, "run.sh"), "#!/bin/sh\n", 0755)
	write(filepath.Join(up, "data.txt"), "data", 0640)
	write(filepath.Join(up, "sub", "private"), "secret", 0600)
	knot("file", "sync", up, "modes:tree")
	expectRemote("tree/run.sh", 0755)
	expectRemote("tree/data.txt", 0640)
	expectRemote("tree/sub/private", 0600)
	os.Chmod(filepath.Join(up, "data.txt"), 0604)
	if out := knot("file", "sync", up, "modes:tree"); !strings.Contains(out, "set mode") {
		t.Errorf("a chmod alone was not synced:\n%s", out)
	}
	expectRemote("tree/data.txt", 0604)
	down := filepath.Join(root, "down")
	knot("file", "sync", "modes:tree", down)
	expectLocal(filepath.Join(down, "run.sh"), 0755)
	expectLocal(filepath.Join(down, "data.txt"), 0604)
	expectLocal(filepath.Join(down, "sub", "private"), 0600)
	chmodRemote("tree/run.sh", 0700)
	knot("file", "sync", "modes:tree", down)
	expectLocal(filepath.Join(down, "run.sh"), 0700)

	// Between buckets, on the server.
	knot("file", "sync", "modes:tree", "modes:tree-copy")
	expectRemote("tree-copy/run.sh", 0700)
	expectRemote("tree-copy/sub/private", 0600)

	// Two ways: a chmod on either side reaches the other.
	tw := filepath.Join(root, "tw")
	state := filepath.Join(root, "tw-state.json")
	write(filepath.Join(tw, "tool"), "tool", 0750)
	knot("file", "sync", "--two-way", "--state-file", state, tw, "modes:tw")
	expectRemote("tw/tool", 0750)
	os.Chmod(filepath.Join(tw, "tool"), 0700)
	knot("file", "sync", "--two-way", "--state-file", state, tw, "modes:tw")
	expectRemote("tw/tool", 0700)
	chmodRemote("tw/tool", 0755)
	knot("file", "sync", "--two-way", "--state-file", state, tw, "modes:tw")
	expectLocal(filepath.Join(tw, "tool"), 0755)

	// A write that gives no mode, as from the web interface, keeps it.
	if _, err := admin.Client.PutFileObject(ctx, "modes", "tw/tool", strings.NewReader("tool v2"), 7, "", apiclient.FileAttrs{}); err != nil {
		t.Fatal(err)
	}
	expectRemote("tw/tool", 0755)

	// A restore from a backup brings the mode back.
	backup := filepath.Join(root, "backup")
	key := "0123456789abcdef0123456789abcdef"
	knot("admin", "backup", backup, "--encrypt-key", key)
	if err := admin.Client.DeleteFileObject(ctx, "modes", "tree/sub/private"); err != nil {
		t.Fatal(err)
	}
	knot("admin", "file", "restore", backup, "admin--modes:tree/sub/private", "--encrypt-key", key)
	expectRemote("tree/sub/private", 0600)

	t.Run("nfs", func(t *testing.T) {
		mnt := mountNFS(t, s, admin, filepath.Join(root, "mnt"))
		bucket := filepath.Join(mnt, "modes")

		// Files written by the knot command show their modes.
		expectLocal(filepath.Join(bucket, "tree", "data.txt"), 0604)
		expectLocal(filepath.Join(bucket, "tree", "sub", "private"), 0600)

		// A file made over NFS keeps its mode, through a write and a rename.
		made := filepath.Join(bucket, "nfs", "made.sh")
		os.MkdirAll(filepath.Dir(made), 0755)
		if err := os.WriteFile(made, []byte("#!/bin/sh\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(made, 0750); err != nil {
			t.Fatal(err)
		}
		f, err := os.OpenFile(made, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString("echo more\n")
		f.Close()
		moved := filepath.Join(bucket, "nfs", "moved.sh")
		if err := os.Rename(made, moved); err != nil {
			t.Fatal(err)
		}
		expectLocal(moved, 0750)
		if !waitForCond(30, func() bool {
			info, err := admin.Client.HeadFileObject(ctx, "modes", "nfs/moved.sh")
			if err != nil || info.Size != int64(len("#!/bin/sh\necho more\n")) {
				return false
			}
			mode, _ := apiclient.ParseFileMode(info.Meta)
			return mode == 0750
		}) {
			t.Errorf("modes:nfs/moved.sh: mode %04o after it was saved, want 0750", remoteMode("nfs/moved.sh"))
		}

		// A chmod over NFS reaches the bucket and the knot command.
		if err := os.Chmod(filepath.Join(bucket, "tree", "data.txt"), 0611); err != nil {
			t.Fatal(err)
		}
		if !waitForCond(30, func() bool { return remoteMode("tree/data.txt") == 0611 }) {
			t.Errorf("modes:tree/data.txt: mode %04o after a chmod over NFS, want 0611", remoteMode("tree/data.txt"))
		}
		knot("file", "cp", "modes:nfs/moved.sh", filepath.Join(root, "from-nfs", "moved.sh"))
		expectLocal(filepath.Join(root, "from-nfs", "moved.sh"), 0750)
	})
}

// mountNFS serves the server's buckets over NFS, as user, and mounts them at
// dir. It skips the test where this machine cannot mount NFS as this user
// (only macOS lets a user mount without root).
func mountNFS(t *testing.T, s *harness.Server, user *harness.User, dir string) string {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("mounting NFS needs root here")
	}
	port, err := harness.FreePort()
	if err != nil {
		t.Fatal(err)
	}
	srv := exec.Command(bins.Server, "nfs-server", "--server", s.BaseURL, "--token", user.Token,
		"--nfs-listen", "127.0.0.1:"+strconv.Itoa(port))
	srv.Dir = filepath.Dir(dir)
	srv.Env = append(os.Environ(), "HOME="+srv.Dir)
	var log bytes.Buffer
	srv.Stdout, srv.Stderr = &log, &log
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		srv.Process.Kill()
		srv.Wait()
	})
	if !waitForCond(30, func() bool {
		c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), time.Second)
		if err == nil {
			c.Close()
		}
		return err == nil
	}) {
		t.Fatalf("nfs-server did not start:\n%s", log.String())
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "mount_nfs", "-o", "vers=4,tcp,port="+strconv.Itoa(port), "127.0.0.1:/", dir).CombinedOutput()
	if err != nil {
		t.Skipf("cannot mount NFS here: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		if out, err := exec.Command("umount", dir).CombinedOutput(); err != nil {
			exec.Command("diskutil", "unmount", "force", dir).Run()
			t.Logf("umount %s: %v %s", dir, err, out)
		}
	})
	return dir
}
