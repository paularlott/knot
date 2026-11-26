package nfsserver

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	nfs "github.com/willscott/go-nfs"
)

// A WRITE looks its file up and opens it, then writes: a RENAME of the file
// can finish in between. The write goes to the file where it went, and no
// new file is made at the old path.
func TestWriteAfterRenameFollowsTheFile(t *testing.T) {
	defer func(d time.Duration) { writersWait = d }(writersWait)
	writersWait = 50 * time.Millisecond // the write comes after the rename
	fsys, _ := newTestFS(t)
	writeFile(t, fsys, "data/tmp/x", "abc")
	if err := fsys.Commit("data/tmp/x"); err != nil {
		t.Fatal(err)
	}
	f, err := fsys.OpenFile("data/tmp/x", os.O_RDWR, 0) // as onWrite opens it
	if err != nil {
		t.Fatal(err)
	}
	if err := fsys.Rename("data/tmp/x", "data/same/k"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(3, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("def")); err != nil {
		t.Fatalf("write after rename: %v", err)
	}
	// A stable write commits by the path it found: the write, staged where
	// the file went, is saved by it.
	if err := fsys.Commit("data/tmp/x"); err != nil {
		t.Fatal(err)
	}
	if e := fsys.staging.get("data", "same/k"); e != nil {
		t.Error("commit by the old path left the write unsaved")
	}
	if got := readFile(t, fsys, "data/same/k"); got != "abcdef" {
		t.Errorf("renamed file holds %q, want abcdef", got)
	}
	if _, err := fsys.Stat("data/tmp/x"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the old path was made again: %v", err)
	}
}

// A file removed here between a WRITE's lookup and the write is not made
// again by it: the write is stale.
func TestWriteAfterRemoveIsStale(t *testing.T) {
	fsys, _ := newTestFS(t)
	writeFile(t, fsys, "data/gone.txt", "abc")
	fsys.Commit("data/gone.txt")
	f, err := fsys.OpenFile("data/gone.txt", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := fsys.Remove("data/gone.txt"); err != nil {
		t.Fatal(err)
	}
	_, err = f.Write([]byte("x"))
	var st *nfs.NFSStatusError
	if !errors.As(err, &st) || st.NFSStatus != nfs.NFSStatusStale {
		t.Errorf("write after remove: %v, want stale", err)
	}
	fsys.Commit("data/gone.txt")
	if _, err := fsys.Stat("data/gone.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the removed file was made again: %v", err)
	}
}

// Making a file at a name renamed away is still a new file.
func TestCreateAfterRenameIsNew(t *testing.T) {
	fsys, _ := newTestFS(t)
	writeFile(t, fsys, "data/tmp/y", "old")
	fsys.Commit("data/tmp/y")
	if err := fsys.Rename("data/tmp/y", "data/same/y"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, fsys, "data/tmp/y", "new")
	fsys.Commit("data/tmp/y")
	if a, b := readFile(t, fsys, "data/tmp/y"), readFile(t, fsys, "data/same/y"); a != "new" || b != "old" {
		t.Errorf("tmp/y %q same/y %q, want new and old", a, b)
	}
}

// gatedMove holds a rename in its move until let go, so a write can be sent
// as it is made.
type gatedMove struct {
	Backend
	entered chan struct{}
	gate    chan struct{}
}

func (g *gatedMove) Move(ctx context.Context, bucket, from, to string) error {
	close(g.entered)
	<-g.gate
	return g.Backend.Move(ctx, bucket, from, to)
}

// The race the shared-folder test found: a WRITE that found the file, then a
// RENAME of it, the write's copy made as the rename runs. The write waits
// for the rename, then goes to the file where it went; nothing is saved at
// the old path, and the renamed file is not left without the write.
func TestWriteDuringRenameFollowsTheFile(t *testing.T) {
	// The write, begun before the rename, comes only once the rename has
	// stopped waiting for it and is moving the file.
	defer func(d time.Duration) { writersWait = d }(writersWait)
	writersWait = 50 * time.Millisecond
	fsys, _ := newTestFS(t)
	writeFile(t, fsys, "data/tmp/z", "")
	if err := fsys.Commit("data/tmp/z"); err != nil { // saved empty, as CREATE leaves it
		t.Fatal(err)
	}
	f, err := fsys.OpenFile("data/tmp/z", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	g := &gatedMove{Backend: fsys.backend, entered: make(chan struct{}), gate: make(chan struct{})}
	fsys.backend = g
	renamed := make(chan error, 1)
	go func() { renamed <- fsys.Rename("data/tmp/z", "data/same/z") }()
	<-g.entered // the rename has saved what was staged and is moving the file

	wrote := make(chan error, 1)
	go func() {
		_, err := f.Write([]byte("data"))
		wrote <- err
	}()
	select {
	case err := <-wrote:
		t.Fatalf("the write was made during the rename: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(g.gate)
	if err := <-renamed; err != nil {
		t.Fatal(err)
	}
	if err := <-wrote; err != nil {
		t.Fatalf("write: %v", err)
	}
	fsys.backend = g.Backend
	if err := fsys.Commit("data/same/z"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, fsys, "data/same/z"); got != "data" {
		t.Errorf("renamed file holds %q, want data", got)
	}
	if _, err := fsys.Stat("data/tmp/z"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the old path was made again: %v", err)
	}
	if e := fsys.staging.get("data", "tmp/z"); e != nil {
		t.Error("a copy is staged at the old path")
	}
}

// A WRITE opened before a RENAME of its file, its data not yet staged: the
// rename waits for it, and moves the file with it, so the renamed file is
// never seen without the write.
func TestRenameWaitsForWriteBegunBefore(t *testing.T) {
	fsys, _ := newTestFS(t)
	writeFile(t, fsys, "data/tmp/w", "")
	fsys.Commit("data/tmp/w")
	f, err := fsys.OpenFile("data/tmp/w", os.O_RDWR, 0) // the WRITE has begun
	if err != nil {
		t.Fatal(err)
	}
	renamed := make(chan error, 1)
	go func() { renamed <- fsys.Rename("data/tmp/w", "data/same/w") }()
	select {
	case err := <-renamed:
		t.Fatalf("the rename did not wait for the write: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if _, err := f.Write([]byte("data")); err != nil {
		t.Fatal(err)
	}
	if err := <-renamed; err != nil {
		t.Fatal(err)
	}
	// Moved with the write: no commit needed for the renamed file to hold it.
	if got := readFile(t, fsys, "data/same/w"); got != "data" {
		t.Errorf("renamed file holds %q, want data", got)
	}
	if _, err := fsys.Stat("data/tmp/w"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the old path is still there: %v", err)
	}
}

// A write that never comes (its call failed) holds a rename up only for a
// while.
func TestRenameDoesNotWaitForeverForAWrite(t *testing.T) {
	defer func(d time.Duration) { writersWait = d }(writersWait)
	writersWait = 200 * time.Millisecond
	fsys, _ := newTestFS(t)
	writeFile(t, fsys, "data/tmp/v", "v")
	fsys.Commit("data/tmp/v")
	if _, err := fsys.OpenFile("data/tmp/v", os.O_RDWR, 0); err != nil { // never written or closed
		t.Fatal(err)
	}
	start := time.Now()
	if err := fsys.Rename("data/tmp/v", "data/same/v"); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("rename took %s", d)
	}
	if got := readFile(t, fsys, "data/same/v"); got != "v" {
		t.Errorf("renamed file holds %q", got)
	}
}
