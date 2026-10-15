package nfsserver

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/filestore"
)

func newTestFS(t *testing.T) (*storeFS, *filestore.Principal) {
	t.Helper()
	store, err := filestore.Open(filestore.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	p := &filestore.Principal{UserId: "u1", Username: "alice", CanOwn: true}
	if _, err := store.CreateBucket(p, "alice--data"); err != nil {
		t.Fatal(err)
	}
	fsys := newStoreFS(&storeBackend{store: store, p: p})
	fsys.staging = newStaging(fsys, t.TempDir(), time.Hour, time.Hour)
	t.Cleanup(fsys.staging.close)
	return fsys, p
}

func writeFile(t *testing.T, fsys *storeFS, name, content string) {
	t.Helper()
	f, err := fsys.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func readFile(t *testing.T, fsys *storeFS, name string) string {
	t.Helper()
	f, err := fsys.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	buf := make([]byte, 1<<20)
	n, err := f.ReadAt(buf, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	return string(buf[:n])
}

func names(t *testing.T, fsys *storeFS, dir string) []string {
	t.Helper()
	infos, err := fsys.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, fi := range infos {
		n := fi.Name()
		if fi.IsDir() {
			n += "/"
		}
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func flushAll(t *testing.T, fsys *storeFS) {
	t.Helper()
	fsys.staging.mu.Lock()
	var files []*dirtyFile
	for _, e := range fsys.staging.files {
		files = append(files, e)
	}
	fsys.staging.mu.Unlock()
	for _, e := range files {
		if err := fsys.staging.flush(e); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRootListsBuckets(t *testing.T) {
	fsys, _ := newTestFS(t)
	if got := names(t, fsys, ""); len(got) != 1 || got[0] != "data/" {
		t.Fatalf("root = %v", got)
	}
	fi, err := fsys.Stat("data")
	if err != nil || !fi.IsDir() {
		t.Fatalf("stat bucket: %v %v", fi, err)
	}
}

func TestWriteIsStagedThenSaved(t *testing.T) {
	fsys, p := newTestFS(t)
	writeFile(t, fsys, "data/hello.txt", "hello world")

	// Visible before it is saved.
	if got := readFile(t, fsys, "data/hello.txt"); got != "hello world" {
		t.Fatalf("staged read = %q", got)
	}
	if got := names(t, fsys, "data"); len(got) != 1 || got[0] != "hello.txt" {
		t.Fatalf("listing = %v", got)
	}
	if _, err := fsys.backend.(*storeBackend).store.HeadObject(p, "alice--data", "hello.txt"); !errors.Is(err, filestore.ErrNoSuchKey) {
		t.Fatalf("saved too early: %v", err)
	}

	flushAll(t, fsys)
	o, err := fsys.backend.(*storeBackend).store.HeadObject(p, "alice--data", "hello.txt")
	if err != nil || o.Size != 11 {
		t.Fatalf("saved object = %v, %v", o, err)
	}
	if got := readFile(t, fsys, "data/hello.txt"); got != "hello world" {
		t.Fatalf("saved read = %q", got)
	}

	// A partial overwrite keeps the rest of the saved content.
	f, _ := fsys.OpenFile("data/hello.txt", os.O_RDWR, 0)
	f.Seek(6, io.SeekStart)
	f.Write([]byte("there"))
	f.Close()
	flushAll(t, fsys)
	if got := readFile(t, fsys, "data/hello.txt"); got != "hello there" {
		t.Fatalf("after overwrite = %q", got)
	}
}

func TestSavedAfterDelay(t *testing.T) {
	fsys, p := newTestFS(t)
	fsys.staging.flushDelay = 20 * time.Millisecond
	writeFile(t, fsys, "data/a.txt", "x")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := fsys.backend.(*storeBackend).store.HeadObject(p, "alice--data", "a.txt"); err == nil {
			if fsys.staging.get("alice--data", "a.txt") == nil {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("file not saved after the flush delay")
}

func TestFolders(t *testing.T) {
	fsys, _ := newTestFS(t)
	if err := fsys.MkdirAll("data/docs", 0755); err != nil {
		t.Fatal(err)
	}
	fi, err := fsys.Stat("data/docs")
	if err != nil || !fi.IsDir() {
		t.Fatalf("stat empty folder: %v %v", fi, err)
	}
	if got := names(t, fsys, "data"); len(got) != 1 || got[0] != "docs/" {
		t.Fatalf("listing = %v", got)
	}
	if got := names(t, fsys, "data/docs"); len(got) != 0 {
		t.Fatalf("empty folder lists %v", got)
	}

	writeFile(t, fsys, "data/docs/a.txt", "a")
	if err := fsys.Remove("data/docs"); !errors.Is(err, syscall.ENOTEMPTY) {
		t.Fatalf("removing a folder with a staged file: %v", err)
	}
	flushAll(t, fsys)
	if err := fsys.Remove("data/docs"); !errors.Is(err, syscall.ENOTEMPTY) {
		t.Fatalf("removing a non-empty folder: %v", err)
	}
	if err := fsys.Remove("data/docs/a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := fsys.Remove("data/docs"); err != nil {
		t.Fatal(err)
	}
	if _, err := fsys.Stat("data/docs"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("folder still there: %v", err)
	}
}

func TestRename(t *testing.T) {
	fsys, _ := newTestFS(t)
	writeFile(t, fsys, "data/a.txt", "a") // staged, not saved
	if err := fsys.Rename("data/a.txt", "data/b.txt"); err != nil {
		t.Fatal(err)
	}
	if got := names(t, fsys, "data"); len(got) != 1 || got[0] != "b.txt" {
		t.Fatalf("after file rename = %v", got)
	}

	fsys.MkdirAll("data/dir", 0755)
	writeFile(t, fsys, "data/dir/c.txt", "c")
	if err := fsys.Rename("data/dir", "data/moved"); err != nil {
		t.Fatal(err)
	}
	if got := names(t, fsys, "data"); len(got) != 2 || got[0] != "b.txt" || got[1] != "moved/" {
		t.Fatalf("after folder rename = %v", got)
	}
	if got := readFile(t, fsys, "data/moved/c.txt"); got != "c" {
		t.Fatalf("moved content = %q", got)
	}

	// Replacing an existing file, as editors and atomic writers do.
	writeFile(t, fsys, "data/new.tmp", "new")
	if err := fsys.Rename("data/new.tmp", "data/b.txt"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, fsys, "data/b.txt"); got != "new" {
		t.Fatalf("replaced content = %q", got)
	}
}

func TestChmodAndTruncate(t *testing.T) {
	fsys, p := newTestFS(t)
	writeFile(t, fsys, "data/run.sh", "#!/bin/sh\necho hi\n")
	flushAll(t, fsys)

	if err := fsys.Chmod("data/run.sh", 0755); err != nil {
		t.Fatal(err)
	}
	fi, _ := fsys.Stat("data/run.sh")
	if fi.Mode().Perm() != 0755 {
		t.Fatalf("mode = %v", fi.Mode())
	}

	mt := time.Unix(1700000000, 0)
	if err := fsys.Chtimes("data/run.sh", mt, mt); err != nil {
		t.Fatal(err)
	}
	fi, _ = fsys.Stat("data/run.sh")
	if !fi.ModTime().Equal(mt) {
		t.Fatalf("mtime = %v", fi.ModTime())
	}

	f, _ := fsys.OpenFile("data/run.sh", os.O_RDWR, 0)
	if err := f.Truncate(2); err != nil {
		t.Fatal(err)
	}
	f.Close()
	flushAll(t, fsys)
	if got := readFile(t, fsys, "data/run.sh"); got != "#!" {
		t.Fatalf("truncated content = %q", got)
	}
	o, _, err := fsys.backend.(*storeBackend).store.OpenObject(context.Background(), p, "alice--data", "run.sh")
	if err != nil || modeFrom(o.Meta) != 0755 {
		t.Fatalf("mode lost on rewrite: %v %v", o, err)
	}
}

func TestHandles(t *testing.T) {
	fsys, _ := newTestFS(t)
	h := newHandler(fsys)
	long := "x" + strings.Repeat("y", 99)
	fsys.MkdirAll("data/a", 0755)
	writeFile(t, fsys, "data/a/b.txt", "b")
	writeFile(t, fsys, "data/"+long, "long")
	flushAll(t, fsys)

	for _, p := range [][]string{{}, {"data"}, {"data", "a"}, {"data", "a", "b.txt"}, {"data", long}} {
		fh := h.ToHandle(fsys, p)
		if len(fh) > maxHandle {
			t.Fatalf("handle length %d over the limit", len(fh))
		}
		_, got, err := h.FromHandle(fh)
		if err != nil || strings.Join(got, "/") != strings.Join(p, "/") {
			t.Fatalf("round trip %q: %q %v", p, got, err)
		}
	}
}

func TestHandlesGoStale(t *testing.T) {
	fsys, _ := newTestFS(t)
	h := newHandler(fsys)
	stale := func(fh []byte) bool {
		_, _, err := h.FromHandle(fh)
		return err != nil
	}

	// A file renamed away: its old handle is stale, so the client looks
	// the new name up again.
	writeFile(t, fsys, "data/a.txt", "a")
	flushAll(t, fsys)
	fh := h.ToHandle(fsys, []string{"data", "a.txt"})
	if err := fsys.Rename("data/a.txt", "data/b.txt"); err != nil {
		t.Fatal(err)
	}
	if !stale(fh) {
		t.Fatal("handle of a renamed file still valid")
	}

	// A new file at the old path is a different file.
	writeFile(t, fsys, "data/a.txt", "new")
	flushAll(t, fsys)
	if !stale(fh) {
		t.Fatal("handle reaches a different file now at its path")
	}

	// The moved file keeps its inode number.
	moved := h.ToHandle(fsys, []string{"data", "b.txt"})
	if binary.BigEndian.Uint64(moved[1:9]) != binary.BigEndian.Uint64(fh[1:9]) {
		t.Fatal("inode number lost on rename")
	}

	// Folders too.
	fsys.MkdirAll("data/d1", 0755)
	dh := h.ToHandle(fsys, []string{"data", "d1"})
	if err := fsys.Rename("data/d1", "data/d2"); err != nil {
		t.Fatal(err)
	}
	if !stale(dh) {
		t.Fatal("handle of a renamed folder still valid")
	}
	if _, err := fsys.Stat("data/d2"); err != nil {
		t.Fatal(err)
	}
}

func TestSymlink(t *testing.T) {
	fsys, _ := newTestFS(t)
	if err := fsys.Symlink("../target/file", "data/link"); err != nil {
		t.Fatal(err)
	}
	fi, err := fsys.Lstat("data/link")
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("lstat link: %v %v", fi, err)
	}
	if got, err := fsys.Readlink("data/link"); err != nil || got != "../target/file" {
		t.Fatalf("readlink = %q %v", got, err)
	}
	if err := fsys.Symlink("x", "data/link"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("symlink over existing: %v", err)
	}
}

func TestCommitSavesNow(t *testing.T) {
	fsys, p := newTestFS(t)
	writeFile(t, fsys, "data/c.txt", "committed")
	if err := fsys.Commit("data/c.txt"); err != nil {
		t.Fatal(err)
	}
	if o, err := fsys.backend.(*storeBackend).store.HeadObject(p, "alice--data", "c.txt"); err != nil || o.Size != 9 {
		t.Fatalf("not saved on commit: %v %v", o, err)
	}
	if fsys.staging.get("alice--data", "c.txt") != nil {
		t.Fatal("staged copy kept after commit")
	}
	if err := fsys.Commit("data/c.txt"); err != nil {
		t.Fatalf("commit with nothing staged: %v", err)
	}
}

func TestAppleDoubleHidden(t *testing.T) {
	fsys, _ := newTestFS(t)
	fsys.MkdirAll("data/d", 0755)
	writeFile(t, fsys, "data/d/a.txt", "a")
	writeFile(t, fsys, "data/d/._a.txt", "xattrs")
	writeFile(t, fsys, "data/._d", "xattrs")
	flushAll(t, fsys)

	if got := names(t, fsys, "data/d"); len(got) != 1 || got[0] != "a.txt" {
		t.Fatalf("listing shows AppleDouble files: %v", got)
	}
	if got := names(t, fsys, "data"); len(got) != 1 || got[0] != "d/" {
		t.Fatalf("listing shows AppleDouble files: %v", got)
	}
	// Still there by name, for macOS.
	if got := readFile(t, fsys, "data/d/._a.txt"); got != "xattrs" {
		t.Fatalf("AppleDouble file by name = %q", got)
	}

	// Removing a file takes its AppleDouble file with it, and a folder
	// holding only AppleDouble files is empty.
	if err := fsys.Remove("data/d/a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := fsys.Stat("data/d/._a.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("AppleDouble file left behind: %v", err)
	}
	writeFile(t, fsys, "data/d/._orphan", "x")
	flushAll(t, fsys)
	if err := fsys.Remove("data/d"); err != nil {
		t.Fatalf("removing a folder holding only AppleDouble files: %v", err)
	}
	if _, err := fsys.Stat("data/._d"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("folder's AppleDouble file left behind: %v", err)
	}
}

func TestOwnershipAndFolderAttributes(t *testing.T) {
	fsys, _ := newTestFS(t)
	writeFile(t, fsys, "data/f", "f")
	flushAll(t, fsys)
	if err := fsys.Lchown("data/f", 501, 20); err != nil {
		t.Fatal(err)
	}
	fi, _ := fsys.Stat("data/f")
	if info := fi.(*fileInfo); info.uid != 501 || info.gid != 20 {
		t.Fatalf("ownership = %d:%d", info.uid, info.gid)
	}

	// A folder with no marker gets one to keep its attributes, and a
	// listing reports them as a lookup does.
	writeFile(t, fsys, "data/implicit/x", "x")
	flushAll(t, fsys)
	h := newHandler(fsys)
	before := h.ToHandle(fsys, []string{"data", "implicit"})
	mt := time.Unix(1600000000, 0)
	if err := fsys.Chmod("data/implicit", 0750); err != nil {
		t.Fatal(err)
	}
	if err := fsys.Chtimes("data/implicit", mt, mt); err != nil {
		t.Fatal(err)
	}
	if err := fsys.Lchown("data/implicit", 501, 20); err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.FromHandle(before); err != nil {
		t.Fatalf("handle went stale when the folder got a marker: %v", err)
	}
	check := func(fi os.FileInfo, from string) {
		t.Helper()
		info := fi.(*fileInfo)
		if !fi.IsDir() || fi.Mode().Perm() != 0750 || !fi.ModTime().Equal(mt) || info.uid != 501 || info.gid != 20 {
			t.Fatalf("%s: mode %v mtime %v owner %d:%d", from, fi.Mode(), fi.ModTime(), info.uid, info.gid)
		}
	}
	fi, err := fsys.Stat("data/implicit")
	if err != nil {
		t.Fatal(err)
	}
	check(fi, "lookup")
	fsys.forget("data", "implicit", true)
	infos, err := fsys.ReadDir("data")
	if err != nil {
		t.Fatal(err)
	}
	for _, fi := range infos {
		if fi.Name() == "implicit" {
			check(fi, "listing")
		}
	}
}

func TestWritesAdvanceMtime(t *testing.T) {
	fsys, _ := newTestFS(t)
	writeFile(t, fsys, "data/f", "one")
	old := time.Unix(1500000000, 0)
	if err := fsys.Chtimes("data/f", old, old); err != nil {
		t.Fatal(err)
	}
	flushAll(t, fsys)
	f, _ := fsys.OpenFile("data/f", os.O_RDWR, 0)
	f.Write([]byte("two"))
	f.Close()
	flushAll(t, fsys)
	fi, _ := fsys.Stat("data/f")
	if !fi.ModTime().After(old.Add(time.Hour)) {
		t.Fatalf("mtime after a write = %v", fi.ModTime())
	}
}

func TestDefaultOwner(t *testing.T) {
	fsys, _ := newTestFS(t)
	fsys.uid, fsys.gid = 501, 20
	writeFile(t, fsys, "data/mine", "x")
	writeFile(t, fsys, "data/theirs", "x")
	flushAll(t, fsys)
	if err := fsys.Lchown("data/theirs", 0, 0); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]uint32{"data/mine": 501, "data/theirs": 0} {
		fi, err := fsys.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		if info := fi.(*fileInfo); info.uid != want {
			t.Fatalf("%s owned by %d, want %d", name, info.uid, want)
		}
	}
	infos, _ := fsys.ReadDir("data")
	for _, fi := range infos {
		if fi.Name() == "mine" && fi.(*fileInfo).uid != 501 {
			t.Fatalf("listing: mine owned by %d", fi.(*fileInfo).uid)
		}
	}
}
