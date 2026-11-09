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
	"github.com/paularlott/knot/internal/filestore"
)

// objectMode is the mode kept with a bucket file, ok false when it has none.
func objectMode(t *testing.T, store *filestore.Store, bucket, key string) (os.FileMode, bool) {
	t.Helper()
	o, err := store.HeadObject(&filestore.Principal{IsAdmin: true}, "tester--"+bucket, key)
	if err != nil {
		t.Fatal(err)
	}
	return apiclient.ParseFileMode(o.Meta)
}

func expectObjectMode(t *testing.T, store *filestore.Store, bucket, key string, want os.FileMode) {
	t.Helper()
	if mode, ok := objectMode(t, store, bucket, key); !ok || mode != want {
		t.Errorf("%s:%s mode %04o (kept %v), want %04o", bucket, key, mode, ok, want)
	}
}

func expectLocalMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != want {
		t.Errorf("%s mode %04o, want %04o", filepath.Base(path), info.Mode().Perm(), want)
	}
}

// writeMode writes a local file and gives it mode, whatever the umask.
func writeMode(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	writeFile(t, path, content)
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

// chmodRemote changes a bucket file's mode alone, as a chmod over NFS does.
func chmodRemote(t *testing.T, client *apiclient.ApiClient, bucket, key string, mode os.FileMode) {
	t.Helper()
	ctx := context.Background()
	info, err := client.HeadFileObject(ctx, bucket, key)
	if err != nil {
		t.Fatal(err)
	}
	meta := map[string]string{}
	for k, v := range info.Meta {
		meta[k] = v
	}
	meta[apiclient.FileModeMeta] = apiclient.FormatFileMode(mode)
	if _, err := client.CopyFileObject(ctx, apiclient.FileCopyRequest{
		SourceBucket: bucket, SourceKey: key, DestBucket: bucket, DestKey: key, Meta: meta,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCopyKeepsMode(t *testing.T) {
	client, store := newFilesServer(t)
	ctx := context.Background()
	src := filepath.Join(t.TempDir(), "run.sh")
	writeMode(t, src, "#!/bin/sh\n", 0755)

	if err := uploadFile(ctx, client, src, "sync-test", "cp/run.sh"); err != nil {
		t.Fatal(err)
	}
	expectObjectMode(t, store, "sync-test", "cp/run.sh", 0755)

	dst := filepath.Join(t.TempDir(), "run.sh")
	if err := downloadFile(ctx, client, "sync-test", "cp/run.sh", dst); err != nil {
		t.Fatal(err)
	}
	expectLocalMode(t, dst, 0755)

	// The bucket's mode replaces the mode of the file a download overwrites.
	os.Chmod(dst, 0600)
	if err := downloadFile(ctx, client, "sync-test", "cp/run.sh", dst); err != nil {
		t.Fatal(err)
	}
	expectLocalMode(t, dst, 0755)

	// A copy between buckets keeps it.
	if err := serverCopy(ctx, client, source{bucket: "sync-test", key: "cp/run.sh"}, "sync-test", "cp/copy.sh"); err != nil {
		t.Fatal(err)
	}
	expectObjectMode(t, store, "sync-test", "cp/copy.sh", 0755)
}

// A file written without a mode, as the web interface and scripts write,
// keeps the mode of the file it replaces.
func TestUploadWithoutModeKeepsMode(t *testing.T) {
	client, store := newFilesServer(t)
	ctx := context.Background()
	src := filepath.Join(t.TempDir(), "tool")
	writeMode(t, src, "v1", 0750)
	if err := uploadFile(ctx, client, src, "sync-test", "keep/tool"); err != nil {
		t.Fatal(err)
	}

	if _, err := client.PutFileObject(ctx, "sync-test", "keep/tool", strings.NewReader("v2"), 2, "", apiclient.FileAttrs{}); err != nil {
		t.Fatal(err)
	}
	expectObjectMode(t, store, "sync-test", "keep/tool", 0750)
	if c, _ := remoteContent(t, client, "sync-test", "keep/tool"); c != "v2" {
		t.Errorf("content %q, want v2", c)
	}

	// One that gives a mode sets it.
	if _, err := client.PutFileObject(ctx, "sync-test", "keep/tool", strings.NewReader("v3"), 2, "", apiclient.FileAttrs{Mode: 0600}); err != nil {
		t.Fatal(err)
	}
	expectObjectMode(t, store, "sync-test", "keep/tool", 0600)

	// A new file written without a mode, as from the web interface, gets
	// 0644, and a folder marker 0755.
	putRemote(t, client, "sync-test", "keep/plain.txt", "plain")
	expectObjectMode(t, store, "sync-test", "keep/plain.txt", 0644)
	if _, err := client.PutFileObject(ctx, "sync-test", "keep/folder/", strings.NewReader(""), 0, "", apiclient.FileAttrs{}); err != nil {
		t.Fatal(err)
	}
	if o, _ := store.HeadObject(&filestore.Principal{IsAdmin: true}, "tester--sync-test", "keep/folder/"); o.Meta[apiclient.FileModeMeta] != "40755" {
		t.Errorf("folder marker mode %q, want 40755", o.Meta[apiclient.FileModeMeta])
	}

	// A file kept from before with no mode at all is downloaded with the
	// mode of the file it replaces.
	putModeless(t, store, "keep/old.txt", "old")
	dst := filepath.Join(t.TempDir(), "old.txt")
	writeMode(t, dst, "older", 0640)
	if err := downloadFile(ctx, client, "sync-test", "keep/old.txt", dst); err != nil {
		t.Fatal(err)
	}
	expectLocalMode(t, dst, 0640)
}

func TestSyncUpMode(t *testing.T) {
	client, store := newFilesServer(t)
	ctx := context.Background()
	dir := t.TempDir()
	writeMode(t, filepath.Join(dir, "run.sh"), "#!/bin/sh\n", 0755)
	writeMode(t, filepath.Join(dir, "notes.txt"), "notes", 0644)
	// Uploaded before modes were kept: same content, no mode.
	putModeless(t, store, "up/notes.txt", "notes")

	if err := syncUp(ctx, client, dir, "sync-test", "up/", false, false); err != nil {
		t.Fatal(err)
	}
	expectObjectMode(t, store, "sync-test", "up/run.sh", 0755)
	expectObjectMode(t, store, "sync-test", "up/notes.txt", 0644)

	// A chmod alone is synced, without sending the content again.
	before, _ := store.HeadObject(&filestore.Principal{IsAdmin: true}, "tester--sync-test", "up/run.sh")
	os.Chmod(filepath.Join(dir, "run.sh"), 0700)
	if err := syncUp(ctx, client, dir, "sync-test", "up/", false, false); err != nil {
		t.Fatal(err)
	}
	expectObjectMode(t, store, "sync-test", "up/run.sh", 0700)
	after, _ := store.HeadObject(&filestore.Principal{IsAdmin: true}, "tester--sync-test", "up/run.sh")
	if after.SHA256 != before.SHA256 || after.Size != before.Size {
		t.Errorf("content changed by a chmod")
	}
	info, _ := os.Stat(filepath.Join(dir, "run.sh"))
	if mtime, ok := apiclient.ParseMtime(after.Meta["mtime"]); !ok || mtime.Sub(info.ModTime()).Abs() > time.Millisecond {
		t.Errorf("chmod lost the modification time: %v", after.Meta)
	}

	// A changed file is uploaded with its mode.
	writeMode(t, filepath.Join(dir, "run.sh"), "#!/bin/sh\necho v2\n", 0711)
	if err := syncUp(ctx, client, dir, "sync-test", "up/", false, false); err != nil {
		t.Fatal(err)
	}
	expectObjectMode(t, store, "sync-test", "up/run.sh", 0711)
}

func TestSyncDownMode(t *testing.T) {
	client, store := newFilesServer(t)
	ctx := context.Background()
	src := filepath.Join(t.TempDir(), "run.sh")
	writeMode(t, src, "#!/bin/sh\n", 0755)
	if err := uploadFile(ctx, client, src, "sync-test", "down/run.sh"); err != nil {
		t.Fatal(err)
	}
	putModeless(t, store, "down/plain.txt", "plain")

	dir := t.TempDir()
	writeMode(t, filepath.Join(dir, "plain.txt"), "plain", 0640)
	if err := syncDown(ctx, client, "sync-test", "down/", dir, false, false); err != nil {
		t.Fatal(err)
	}
	expectLocalMode(t, filepath.Join(dir, "run.sh"), 0755)
	// The bucket keeps no mode for it: the local file keeps its own.
	expectLocalMode(t, filepath.Join(dir, "plain.txt"), 0640)

	// A chmod in the bucket alone reaches the local file.
	chmodRemote(t, client, "sync-test", "down/run.sh", 0700)
	expectObjectMode(t, store, "sync-test", "down/run.sh", 0700)
	if err := syncDown(ctx, client, "sync-test", "down/", dir, false, false); err != nil {
		t.Fatal(err)
	}
	expectLocalMode(t, filepath.Join(dir, "run.sh"), 0700)
}

func TestSyncTwoWayMode(t *testing.T) {
	client, store := newFilesServer(t)
	dir := t.TempDir()
	state := filepath.Join(t.TempDir(), "state.json")
	local := filepath.Join(dir, "run.sh")

	writeMode(t, local, "#!/bin/sh\n", 0755)
	twoWay(t, client, dir, "sync-test:tw", state, false)
	expectObjectMode(t, store, "sync-test", "tw/run.sh", 0755)

	// Changed here: the bucket follows.
	os.Chmod(local, 0700)
	out := twoWay(t, client, dir, "sync-test:tw", state, false)
	expectObjectMode(t, store, "sync-test", "tw/run.sh", 0700)
	if !logged(out, "set mode", `"mode":"0700"`) {
		t.Errorf("mode change not reported:\n%s", out)
	}

	// Changed in the bucket: the local file follows.
	chmodRemote(t, client, "sync-test", "tw/run.sh", 0750)
	twoWay(t, client, dir, "sync-test:tw", state, false)
	expectLocalMode(t, local, 0750)

	// Changed on both sides: this side's wins.
	os.Chmod(local, 0711)
	chmodRemote(t, client, "sync-test", "tw/run.sh", 0704)
	twoWay(t, client, dir, "sync-test:tw", state, false)
	expectLocalMode(t, local, 0711)
	expectObjectMode(t, store, "sync-test", "tw/run.sh", 0711)

	// Nothing more to do.
	out = twoWay(t, client, dir, "sync-test:tw", state, false)
	if !logged(out, "synced", `"uploaded":0,"downloaded":0,"modes":0`) {
		t.Errorf("settled sync did something:\n%s", out)
	}

	// With no base to go by, the mode the bucket keeps wins.
	other := t.TempDir()
	writeMode(t, filepath.Join(other, "run.sh"), "#!/bin/sh\n", 0644)
	twoWay(t, client, other, "sync-test:tw", filepath.Join(t.TempDir(), "state.json"), false)
	expectLocalMode(t, filepath.Join(other, "run.sh"), 0711)
}

func TestSyncBucketsMode(t *testing.T) {
	client, store := newFilesServer(t)
	ctx := context.Background()
	p, _ := filestore.PrincipalFor(adminUser)
	if _, err := store.CreateBucket(p, "tester--sync-copy"); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "run.sh")
	writeMode(t, src, "#!/bin/sh\n", 0755)
	if err := uploadFile(ctx, client, src, "sync-test", "bb/run.sh"); err != nil {
		t.Fatal(err)
	}

	if err := syncBuckets(ctx, client, "sync-test", "bb/", "sync-copy", "bb/", false, false); err != nil {
		t.Fatal(err)
	}
	expectObjectMode(t, store, "sync-copy", "bb/run.sh", 0755)

	chmodRemote(t, client, "sync-test", "bb/run.sh", 0700)
	if err := syncBuckets(ctx, client, "sync-test", "bb/", "sync-copy", "bb/", false, false); err != nil {
		t.Fatal(err)
	}
	expectObjectMode(t, store, "sync-copy", "bb/run.sh", 0700)
}
