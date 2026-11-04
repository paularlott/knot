//go:build !windows

package command_files

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
)

// expectLink checks that path is a symbolic link to target.
func expectLink(t *testing.T, path, target string) {
	t.Helper()
	got, err := os.Readlink(path)
	if err != nil {
		t.Errorf("%s: not a link: %v", filepath.Base(path), err)
		return
	}
	if got != target {
		t.Errorf("%s: link to %q, want %q", filepath.Base(path), got, target)
	}
}

// expectRemoteLink checks that a bucket file is a link to target, stored as
// the NFS server stores one.
func expectRemoteLink(t *testing.T, client *apiclient.ApiClient, key, target string) {
	t.Helper()
	info, err := client.HeadFileObject(context.Background(), "sync-test", key)
	if err != nil {
		t.Errorf("%s: %v", key, err)
		return
	}
	if info.Meta[apiclient.FileModeMeta] != "120777" {
		t.Errorf("%s: mode %q, want a link's 120777", key, info.Meta[apiclient.FileModeMeta])
	}
	if c, _ := remoteContent(t, client, "sync-test", key); c != target {
		t.Errorf("%s: link to %q, want %q", key, c, target)
	}
}

// putRemoteLink makes a link in the bucket, as one made over NFS.
func putRemoteLink(t *testing.T, client *apiclient.ApiClient, key, target string) {
	t.Helper()
	if _, err := client.PutFileObject(context.Background(), "sync-test", key, strings.NewReader(target), int64(len(target)), "", apiclient.FileAttrs{Mode: linkMode}); err != nil {
		t.Fatal(err)
	}
}

// A tree like composer's: a file, a link to it from bin/, and a link to a
// folder, which is never followed.
func linkTree(t *testing.T) (dir, outside string) {
	t.Helper()
	dir, outside = t.TempDir(), t.TempDir()
	writeMode(t, filepath.Join(dir, "vendor", "pkg", "tool"), "#!/bin/sh\n", 0755)
	os.MkdirAll(filepath.Join(dir, "vendor", "bin"), 0755)
	if err := os.Symlink("../pkg/tool", filepath.Join(dir, "vendor", "bin", "tool")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(outside, "secret.txt"), "secret")
	if err := os.Symlink(outside, filepath.Join(dir, "ext")); err != nil {
		t.Fatal(err)
	}
	return dir, outside
}

func TestCopyLinks(t *testing.T) {
	client, _ := newFilesServer(t)
	dir, outside := linkTree(t)

	mustCopy(t, client, true, dir+"/", "sync-test:cp/")
	expectRemoteLink(t, client, "cp/vendor/bin/tool", "../pkg/tool")
	expectRemoteLink(t, client, "cp/ext", outside)
	if _, ok := remoteContent(t, client, "sync-test", "cp/ext/secret.txt"); ok {
		t.Error("a link to a folder was followed")
	}

	down := t.TempDir()
	mustCopy(t, client, true, "sync-test:cp/", down+"/")
	expectLink(t, filepath.Join(down, "vendor", "bin", "tool"), "../pkg/tool")
	expectLink(t, filepath.Join(down, "ext"), outside)
	if c := readLocal(t, filepath.Join(down, "vendor", "bin", "tool")); c != "#!/bin/sh\n" {
		t.Errorf("link does not reach the file: %q", c)
	}
}

func TestSyncLinks(t *testing.T) {
	client, _ := newFilesServer(t)
	ctx := context.Background()
	dir, outside := linkTree(t)

	if err := syncUp(ctx, client, dir, "sync-test", "up/", false, false); err != nil {
		t.Fatal(err)
	}
	expectRemoteLink(t, client, "up/vendor/bin/tool", "../pkg/tool")
	expectRemoteLink(t, client, "up/ext", outside)
	if _, ok := remoteContent(t, client, "sync-test", "up/ext/secret.txt"); ok {
		t.Error("a link to a folder was followed")
	}

	// A link pointed elsewhere is synced; one left alone is not sent again.
	os.Remove(filepath.Join(dir, "vendor", "bin", "tool"))
	os.Symlink("../pkg/other", filepath.Join(dir, "vendor", "bin", "tool"))
	if err := syncUp(ctx, client, dir, "sync-test", "up/", false, false); err != nil {
		t.Fatal(err)
	}
	expectRemoteLink(t, client, "up/vendor/bin/tool", "../pkg/other")

	// Down into a new folder, then a link changed in the bucket.
	down := t.TempDir()
	if err := syncDown(ctx, client, "sync-test", "up/", down, false, false); err != nil {
		t.Fatal(err)
	}
	expectLink(t, filepath.Join(down, "vendor", "bin", "tool"), "../pkg/other")
	expectLink(t, filepath.Join(down, "ext"), outside)
	putRemoteLink(t, client, "up/vendor/bin/tool", "../pkg/tool")
	if err := syncDown(ctx, client, "sync-test", "up/", down, false, false); err != nil {
		t.Fatal(err)
	}
	expectLink(t, filepath.Join(down, "vendor", "bin", "tool"), "../pkg/tool")

	// A link replaces a file of the same name, and the other way round.
	writeFile(t, filepath.Join(dir, "swap"), "a file")
	putRemoteLink(t, client, "up/swap", "vendor")
	if err := syncDown(ctx, client, "sync-test", "up/", dir, false, false); err != nil {
		t.Fatal(err)
	}
	expectLink(t, filepath.Join(dir, "swap"), "vendor")

	// Deleting a link deletes the link, not what it points to.
	if err := syncDown(ctx, client, "sync-test", "up/", down, false, false); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteFileObject(ctx, "sync-test", "up/ext"); err != nil {
		t.Fatal(err)
	}
	if err := syncDown(ctx, client, "sync-test", "up/", down, true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(down, "ext")); !os.IsNotExist(err) {
		t.Errorf("deleted link still there: %v", err)
	}
	if c := readLocal(t, filepath.Join(outside, "secret.txt")); c != "secret" {
		t.Errorf("deleting a link reached its target: %q", c)
	}
}

func TestSyncTwoWayLinks(t *testing.T) {
	client, _ := newFilesServer(t)
	dir := t.TempDir()
	state := filepath.Join(t.TempDir(), "state.json")
	writeFile(t, filepath.Join(dir, "real.txt"), "real")
	os.Symlink("real.txt", filepath.Join(dir, "here"))
	putRemoteLink(t, client, "tw/there", "real.txt")

	twoWay(t, client, dir, "sync-test:tw", state, false)
	expectRemoteLink(t, client, "tw/here", "real.txt")
	expectLink(t, filepath.Join(dir, "there"), "real.txt")

	// Settled: nothing more to do, though a link's own time cannot be set.
	out := twoWay(t, client, dir, "sync-test:tw", state, false)
	if !logged(out, "synced", `"uploaded":0,"downloaded":0,"modes":0`) {
		t.Errorf("settled sync did something:\n%s", out)
	}
}

// A bucket can hold a link and files below its name; written locally, those
// would land where the link points. Nothing is ever written through a link.
func TestNoWritesThroughLinks(t *testing.T) {
	client, _ := newFilesServer(t)
	ctx := context.Background()
	outside := t.TempDir()

	// A link the bucket planted, and a file below it.
	putRemoteLink(t, client, "evil/esc", outside)
	putRemote(t, client, "sync-test", "evil/esc/pwned", "pwned")
	down := t.TempDir()
	syncDown(ctx, client, "sync-test", "evil/", down, false, false)
	mustCopyMayFail(t, client, "sync-test:evil/", t.TempDir()+"/")
	if _, err := os.Stat(filepath.Join(outside, "pwned")); err == nil {
		t.Fatal("a download was written through a link")
	}

	// A local link, and a bucket file below its name: neither written nor
	// deleted through it, nor read through it for an upload.
	local := t.TempDir()
	os.Symlink(outside, filepath.Join(local, "ext"))
	writeFile(t, filepath.Join(outside, "keep.txt"), "keep")
	putRemote(t, client, "sync-test", "mixed/ext/keep.txt", "replaced")
	putRemote(t, client, "sync-test", "mixed/ext/new.txt", "new")
	syncDown(ctx, client, "sync-test", "mixed/", local, true, false)
	if c := readLocal(t, filepath.Join(outside, "keep.txt")); c != "keep" {
		t.Errorf("a file outside was replaced through a link: %q", c)
	}
	if _, err := os.Stat(filepath.Join(outside, "new.txt")); err == nil {
		t.Error("a file was written outside through a link")
	}
	if err := syncUp(ctx, client, local, "sync-test", "upmixed/", false, false); err != nil {
		t.Fatal(err)
	}
	if _, ok := remoteContent(t, client, "sync-test", "upmixed/ext/keep.txt"); ok {
		t.Error("an upload read a file through a link")
	}
}

// A link whose time in the bucket differs is left as it is: setting a link's
// time would set its target's.
func TestSyncLinkTimesLeftAlone(t *testing.T) {
	client, _ := newFilesServer(t)
	ctx := context.Background()
	down := t.TempDir()
	writeFile(t, filepath.Join(down, "target.txt"), "target")
	os.Symlink("target.txt", filepath.Join(down, "link"))
	before, _ := os.Stat(filepath.Join(down, "target.txt"))

	old := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if _, err := client.PutFileObject(ctx, "sync-test", "times/link", strings.NewReader("target.txt"), int64(len("target.txt")), "", apiclient.FileAttrs{Mode: linkMode, Mtime: old}); err != nil {
		t.Fatal(err)
	}
	if err := syncDown(ctx, client, "sync-test", "times/", down, false, false); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(filepath.Join(down, "target.txt"))
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("syncing a link set its target's time: %v, was %v", after.ModTime(), before.ModTime())
	}
}

// mustCopyMayFail runs a copy that may refuse, as long as it does not panic.
func mustCopyMayFail(t *testing.T, client *apiclient.ApiClient, args ...string) {
	t.Helper()
	runCopy(context.Background(), client, args, true)
}
