//go:build !windows

package command_files

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/filestore"
)

// mkdirMode makes a local folder with mode, whatever the umask.
func mkdirMode(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

// putMarker makes a bucket folder with mode, as a mkdir over NFS does.
func putMarker(t *testing.T, client *apiclient.ApiClient, bucket, key string, mode os.FileMode) {
	t.Helper()
	if _, err := client.PutFileObject(context.Background(), bucket, key, strings.NewReader(""), 0, "", apiclient.FileAttrs{Mode: os.ModeDir | mode}); err != nil {
		t.Fatalf("put %s:%s: %v", bucket, key, err)
	}
}

// expectMarker checks a bucket folder's marker holds a folder's mode, as the
// NFS server writes it.
func expectMarker(t *testing.T, store *filestore.Store, bucket, key, mode string) {
	t.Helper()
	o, err := store.HeadObject(&filestore.Principal{IsAdmin: true}, "tester--"+bucket, key)
	if err != nil {
		t.Errorf("%s:%s: %v", bucket, key, err)
		return
	}
	if got := o.Meta[apiclient.FileModeMeta]; got != mode {
		t.Errorf("%s:%s mode %q, want %q", bucket, key, got, mode)
	}
}

func expectDir(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		t.Errorf("%s is not a folder: %v", path, err)
		return
	}
	if info.Mode().Perm() != mode {
		t.Errorf("%s mode %04o, want %04o", filepath.Base(path), info.Mode().Perm(), mode)
	}
}

func expectNoPath(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Errorf("%s still there: %v", path, err)
	}
}

// writable makes folders writable again for the test's clean up.
func writable(t *testing.T, paths ...string) {
	t.Cleanup(func() {
		for _, p := range paths {
			os.Chmod(p, 0755)
		}
	})
}

// Every folder synced up gets a marker with its mode, empty or not, which
// follows a chmod; with --delete, a folder gone here goes from the bucket.
func TestSyncUpFolders(t *testing.T) {
	client, store := newFilesServer(t)
	ctx := context.Background()
	src := t.TempDir()
	mkdirMode(t, filepath.Join(src, "empty"), 0755)
	mkdirMode(t, filepath.Join(src, "secret"), 0700)
	writeFile(t, filepath.Join(src, "secret", "key.txt"), "k")
	mkdirMode(t, filepath.Join(src, "a"), 0755)
	mkdirMode(t, filepath.Join(src, "a", "b"), 0755)
	mkdirMode(t, filepath.Join(src, "a", "b", "c"), 0750)

	if err := runSync(ctx, client, src, "sync-test:up", false, false); err != nil {
		t.Fatal(err)
	}
	expectKeysWithPrefixIn(t, client, "sync-test", "up/", "up/a/", "up/a/b/", "up/a/b/c/", "up/empty/", "up/secret/", "up/secret/key.txt")
	expectMarker(t, store, "sync-test", "up/empty/", "40755")
	expectMarker(t, store, "sync-test", "up/secret/", "40700")
	expectMarker(t, store, "sync-test", "up/a/b/c/", "40750")

	// Nothing changed: no marker is written again.
	before, _ := store.HeadObject(admin, "tester--sync-test", "up/empty/")
	if err := runSync(ctx, client, src, "sync-test:up", false, false); err != nil {
		t.Fatal(err)
	}
	if again, _ := store.HeadObject(admin, "tester--sync-test", "up/empty/"); again.UpdatedAt != before.UpdatedAt {
		t.Error("an unchanged folder's marker was written again")
	}

	// A chmod alone reaches the bucket, keeping the marker's other
	// attributes, such as the inode number the NFS server gave the folder.
	info, err := client.HeadFileObject(ctx, "sync-test", "up/secret/")
	if err != nil {
		t.Fatal(err)
	}
	info.Meta["nfs-ino"] = "1234abcd"
	if _, err := client.CopyFileObject(ctx, apiclient.FileCopyRequest{SourceBucket: "sync-test", SourceKey: "up/secret/", DestBucket: "sync-test", DestKey: "up/secret/", Meta: info.Meta}); err != nil {
		t.Fatal(err)
	}
	os.Chmod(filepath.Join(src, "secret"), 0750)
	if err := runSync(ctx, client, src, "sync-test:up", false, false); err != nil {
		t.Fatal(err)
	}
	expectMarker(t, store, "sync-test", "up/secret/", "40750")
	if o, _ := store.HeadObject(admin, "tester--sync-test", "up/secret/"); o == nil || o.Meta["nfs-ino"] != "1234abcd" {
		t.Errorf("a folder's chmod lost the marker's other attributes: %+v", o)
	}

	// A folder removed here stays in the bucket without --delete, goes with
	// it, and a dry run removes nothing.
	os.Remove(filepath.Join(src, "empty"))
	os.RemoveAll(filepath.Join(src, "a", "b"))
	if err := runSync(ctx, client, src, "sync-test:up", false, false); err != nil {
		t.Fatal(err)
	}
	expectKeysWithPrefixIn(t, client, "sync-test", "up/", "up/a/", "up/a/b/", "up/a/b/c/", "up/empty/", "up/secret/", "up/secret/key.txt")
	if err := runSync(ctx, client, src, "sync-test:up", true, true); err != nil {
		t.Fatal(err)
	}
	expectKeysWithPrefixIn(t, client, "sync-test", "up/", "up/a/", "up/a/b/", "up/a/b/c/", "up/empty/", "up/secret/", "up/secret/key.txt")
	if err := runSync(ctx, client, src, "sync-test:up", true, false); err != nil {
		t.Fatal(err)
	}
	expectKeysWithPrefixIn(t, client, "sync-test", "up/", "up/a/", "up/secret/", "up/secret/key.txt")
}

// Bucket folders become local folders with their markers' modes, a
// read-only one too, once what is in it is written; with --delete, a local
// folder the bucket does not have goes when it is empty.
func TestSyncDownFolders(t *testing.T) {
	client, _ := newFilesServer(t)
	ctx := context.Background()
	putMarker(t, client, "sync-test", "dn/empty/", 0750)
	putMarker(t, client, "sync-test", "dn/priv/", 0700)
	putRemote(t, client, "sync-test", "dn/priv/f.txt", "p")
	putMarker(t, client, "sync-test", "dn/ro/", 0555)
	putRemote(t, client, "sync-test", "dn/ro/f.txt", "r")
	putRemote(t, client, "sync-test", "dn/plain/x.txt", "x") // a folder with no marker

	dst := t.TempDir()
	writable(t, filepath.Join(dst, "ro"))
	if err := runSync(ctx, client, "sync-test:dn", dst, false, false); err != nil {
		t.Fatal(err)
	}
	expectDir(t, filepath.Join(dst, "empty"), 0750)
	expectDir(t, filepath.Join(dst, "priv"), 0700)
	expectDir(t, filepath.Join(dst, "ro"), 0555)
	if got := readLocal(t, filepath.Join(dst, "ro", "f.txt")); got != "r" {
		t.Errorf("file in a read-only folder: %q", got)
	}
	if info, err := os.Stat(filepath.Join(dst, "plain")); err != nil || !info.IsDir() {
		t.Errorf("folder with no marker: %v", err)
	}

	// A chmod in the bucket alone reaches the local folder.
	chmodRemote(t, client, "sync-test", "dn/empty/", os.ModeDir|0700)
	if err := runSync(ctx, client, "sync-test:dn", dst, false, false); err != nil {
		t.Fatal(err)
	}
	expectDir(t, filepath.Join(dst, "empty"), 0700)

	// A folder the bucket holds files in is not one to delete, even while
	// it is empty here: a dry run says the file would be fetched, not that
	// the folder would go.
	os.Remove(filepath.Join(dst, "plain", "x.txt"))
	var out logBuf
	if err := runSyncRequest(ctx, client, "sync-test:dn", dst, syncRequest{del: true, dryRun: true, log: out.logger()}); err != nil {
		t.Fatal(err)
	}
	if logged(out.String(), "would delete folder", `"plain"`) || !logged(out.String(), "would download", "plain/x.txt") {
		t.Errorf("dry run:\n%s", out.String())
	}
	if err := runSync(ctx, client, "sync-test:dn", dst, false, false); err != nil {
		t.Fatal(err)
	}

	// Folders the bucket does not have stay without --delete; with it the
	// empty ones go, and those holding what the sync leaves out stay.
	if err := client.DeleteFileObject(ctx, "sync-test", "dn/empty/"); err != nil {
		t.Fatal(err)
	}
	mkdirMode(t, filepath.Join(dst, "stray", "deeper"), 0755)
	writeFile(t, filepath.Join(dst, "kept", "x.log"), "excluded")
	if err := runSync(ctx, client, "sync-test:dn", dst, false, false); err != nil {
		t.Fatal(err)
	}
	expectDir(t, filepath.Join(dst, "empty"), 0700)
	expectDir(t, filepath.Join(dst, "stray", "deeper"), 0755)
	if err := runSyncRequest(ctx, client, "sync-test:dn", dst, syncRequest{del: true, excludes: []string{"*.log"}}); err != nil {
		t.Fatal(err)
	}
	expectNoPath(t, filepath.Join(dst, "empty"))
	expectNoPath(t, filepath.Join(dst, "stray"))
	expectDir(t, filepath.Join(dst, "priv"), 0700)
	if _, err := os.Stat(filepath.Join(dst, "kept", "x.log")); err != nil {
		t.Errorf("a folder holding an excluded file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "plain", "x.txt")); err != nil {
		t.Errorf("a folder with no marker but files: %v", err)
	}

	// An empty folder in the bucket is kept here when its last file goes.
	putMarker(t, client, "sync-test", "dn/keep/", 0755)
	putRemote(t, client, "sync-test", "dn/keep/last.txt", "l")
	if err := runSync(ctx, client, "sync-test:dn", dst, true, false); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteFileObject(ctx, "sync-test", "dn/keep/last.txt"); err != nil {
		t.Fatal(err)
	}
	if err := runSync(ctx, client, "sync-test:dn", dst, true, false); err != nil {
		t.Fatal(err)
	}
	expectNoPath(t, filepath.Join(dst, "keep", "last.txt"))
	expectDir(t, filepath.Join(dst, "keep"), 0755)
}

// Two ways, a folder made on either side is made on the other, a chmod on
// either side reaches the other, and a folder removed on one side goes from
// the other with --delete and is put back without it.
func TestSyncTwoWayFolders(t *testing.T) {
	client, store := newFilesServer(t)
	ctx := context.Background()
	dir := t.TempDir()
	state := filepath.Join(t.TempDir(), "state.json")
	mkdirMode(t, filepath.Join(dir, "mine"), 0700)
	putMarker(t, client, "sync-test", "tw/theirs/", 0750)

	twoWay(t, client, dir, "sync-test:tw", state, true)
	expectMarker(t, store, "sync-test", "tw/mine/", "40700")
	expectDir(t, filepath.Join(dir, "theirs"), 0750)

	// A chmod on each side.
	os.Chmod(filepath.Join(dir, "theirs"), 0700)
	chmodRemote(t, client, "sync-test", "tw/mine/", os.ModeDir|0750)
	twoWay(t, client, dir, "sync-test:tw", state, true)
	expectMarker(t, store, "sync-test", "tw/theirs/", "40700")
	expectDir(t, filepath.Join(dir, "mine"), 0750)

	// Removed on each side, with --delete.
	os.Remove(filepath.Join(dir, "theirs"))
	if err := client.DeleteFileObject(ctx, "sync-test", "tw/mine/"); err != nil {
		t.Fatal(err)
	}
	out := twoWay(t, client, dir, "sync-test:tw", state, true)
	for _, k := range bucketKeys(t, client, "sync-test") {
		if strings.HasPrefix(k, "tw/") {
			t.Errorf("%s still in the bucket", k)
		}
	}
	expectNoPath(t, filepath.Join(dir, "mine"))
	if !logged(out, "deleted folder") {
		t.Errorf("no folder deletion logged:\n%s", out)
	}

	// Removed on each side, without --delete: put back.
	mkdirMode(t, filepath.Join(dir, "here"), 0755)
	putMarker(t, client, "sync-test", "tw/there/", 0755)
	twoWay(t, client, dir, "sync-test:tw", state, false)
	os.Remove(filepath.Join(dir, "there"))
	if err := client.DeleteFileObject(ctx, "sync-test", "tw/here/"); err != nil {
		t.Fatal(err)
	}
	twoWay(t, client, dir, "sync-test:tw", state, false)
	expectKeysWithPrefixIn(t, client, "sync-test", "tw/", "tw/here/", "tw/there/")
	expectDir(t, filepath.Join(dir, "there"), 0755)

	// A folder removed in the bucket while a file was added to it here is
	// kept, and its marker written again.
	writeFile(t, filepath.Join(dir, "here", "new.txt"), "n")
	if err := client.DeleteFileObject(ctx, "sync-test", "tw/here/"); err != nil {
		t.Fatal(err)
	}
	twoWay(t, client, dir, "sync-test:tw", state, true)
	expectKeysWithPrefixIn(t, client, "sync-test", "tw/", "tw/here/", "tw/here/new.txt", "tw/there/")
}

// While watching, a new empty folder, a chmod of a folder and a folder
// removed reach the bucket, and a folder made in the bucket arrives here.
func TestSyncWatchFolders(t *testing.T) {
	client, store := newFilesServer(t)
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "first.txt"), "1")
	startWatch(t, client, src, "sync-test:wf", syncRequest{del: true})
	waitUntil(t, "initial pass", func() bool { _, ok := remoteContent(t, client, "sync-test", "wf/first.txt"); return ok })

	markerMode := func(key string) string {
		o, err := store.HeadObject(admin, "tester--sync-test", key)
		if err != nil {
			return ""
		}
		return o.Meta[apiclient.FileModeMeta]
	}
	mkdirMode(t, filepath.Join(src, "empty"), 0755)
	waitUntil(t, "new empty folder", func() bool { return markerMode("wf/empty/") == "40755" })
	os.Chmod(filepath.Join(src, "empty"), 0700)
	waitUntil(t, "chmod of a folder", func() bool { return markerMode("wf/empty/") == "40700" })
	os.Remove(filepath.Join(src, "empty"))
	waitUntil(t, "folder removed", func() bool { return markerMode("wf/empty/") == "" })

	dst := t.TempDir()
	putRemote(t, client, "sync-test", "wg/first.txt", "1")
	ev := startWatch(t, client, "sync-test:wg", dst, syncRequest{})
	waitUntil(t, "initial pass down", func() bool { _, err := os.Stat(filepath.Join(dst, "first.txt")); return err == nil })
	putMarker(t, client, "sync-test", "wg/made/", 0750)
	ev.filesChanged()
	waitUntil(t, "folder made in the bucket", func() bool {
		info, err := os.Stat(filepath.Join(dst, "made"))
		return err == nil && info.IsDir() && info.Mode().Perm() == 0750
	})
}

// cp -r copies folders as folders, empty ones too, with their modes, both
// ways.
func TestCopyFolders(t *testing.T) {
	client, store := newFilesServer(t)
	src := t.TempDir()
	mkdirMode(t, filepath.Join(src, "empty"), 0755)
	mkdirMode(t, filepath.Join(src, "secret"), 0700)
	writeFile(t, filepath.Join(src, "secret", "k.txt"), "k")
	writeFile(t, filepath.Join(src, "ro", "f.txt"), "r")
	os.Chmod(filepath.Join(src, "ro"), 0555)
	writable(t, filepath.Join(src, "ro"))

	mustCopy(t, client, true, src, "sync-test:cpf/")
	expectKeysWithPrefixIn(t, client, "sync-test", "cpf/", "cpf/empty/", "cpf/ro/", "cpf/ro/f.txt", "cpf/secret/", "cpf/secret/k.txt")
	expectMarker(t, store, "sync-test", "cpf/empty/", "40755")
	expectMarker(t, store, "sync-test", "cpf/secret/", "40700")
	expectMarker(t, store, "sync-test", "cpf/ro/", "40555")

	// Copied again, a folder the bucket has is left as it is, and one whose
	// mode changed has just its mode changed.
	before, _ := store.HeadObject(admin, "tester--sync-test", "cpf/empty/")
	os.Chmod(filepath.Join(src, "secret"), 0750)
	mustCopy(t, client, true, src, "sync-test:cpf/")
	if again, _ := store.HeadObject(admin, "tester--sync-test", "cpf/empty/"); again.UpdatedAt != before.UpdatedAt {
		t.Error("copied again, an unchanged folder's marker was written again")
	}
	expectMarker(t, store, "sync-test", "cpf/secret/", "40750")
	os.Chmod(filepath.Join(src, "secret"), 0700)
	mustCopy(t, client, true, src, "sync-test:cpf/")

	dst := t.TempDir()
	writable(t, filepath.Join(dst, "ro"))
	mustCopy(t, client, true, "sync-test:cpf/", dst+"/")
	expectDir(t, filepath.Join(dst, "empty"), 0755)
	expectDir(t, filepath.Join(dst, "secret"), 0700)
	expectDir(t, filepath.Join(dst, "ro"), 0555)
	if got := readLocal(t, filepath.Join(dst, "ro", "f.txt")); got != "r" {
		t.Errorf("file in a read-only folder: %q", got)
	}
}
