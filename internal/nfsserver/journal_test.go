package nfsserver

import (
	"bytes"
	"errors"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/filestore"
)

// restartableFS is a storeFS over one store whose staging folder outlives
// it, as an nfs-server's does: start makes a new one over the folder, as an
// nfs-server started again after the last was killed.
type restartableFS struct {
	backend Backend
	dir     string
}

func newRestartableFS(t *testing.T) *restartableFS {
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
	return &restartableFS{backend: &storeBackend{store: store, p: p}, dir: t.TempDir()}
}

func (r *restartableFS) start() *storeFS {
	fsys := newStoreFS(r.backend)
	fsys.staging = newStaging(fsys, r.dir, time.Hour, time.Hour)
	fsys.staging.durable = true
	fsys.staging.recover()
	return fsys
}

// kill leaves a server's staged copies as a killed process does: their
// files written, nothing saved, nothing removed.
func kill(fsys *storeFS) {
	s := fsys.staging
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for _, e := range s.files {
		if e.timer != nil {
			e.timer.Stop()
		}
		e.journal.Close()
		e.f.Close()
	}
}

// Writes the nfs-server took and had not saved when it was killed are
// saved by the one started after it: the client was told they were taken,
// and may commit them to the new one without sending them again.
func TestStagedWritesOutliveTheServer(t *testing.T) {
	r := newRestartableFS(t)
	fsys := r.start()

	// A large file, changed in place, and a small one, saved first.
	big := make([]byte, sparseMin+3<<20)
	rand.New(rand.NewSource(7)).Read(big)
	writeFile(t, fsys, "data/big", string(big))
	writeFile(t, fsys, "data/small", "the saved version")
	writeFile(t, fsys, "data/cut", strings.Repeat("x", 100))
	flushAll(t, fsys)

	// Then changed, uncommitted: a part of the large file, the small one
	// rewritten, one cut short and a new one made with a mode.
	f, _ := fsys.OpenFile("data/big", os.O_RDWR, 0)
	part := bytes.Repeat([]byte("Y"), 1<<20)
	f.Seek(2<<20, io.SeekStart)
	if _, err := f.Write(part); err != nil {
		t.Fatal(err)
	}
	copy(big[2<<20:], part)
	writeFile(t, fsys, "data/small", "written, not committed")
	c, _ := fsys.OpenFile("data/cut", os.O_RDWR, 0)
	if err := c.Truncate(10); err != nil {
		t.Fatal(err)
	}
	n, err := fsys.OpenFile("data/new", os.O_CREATE|os.O_RDWR, 0640)
	if err != nil {
		t.Fatal(err)
	}
	n.Write([]byte("new file"))

	// Killed as a write is recorded: its record cut short, as it was never
	// answered, it is left out.
	e := fsys.staging.get("data", "new")
	e.mu.Lock()
	e.journal.Write([]byte(`{"op":"w","off":0,"n":99`))
	e.mu.Unlock()
	kill(fsys)

	fsys = r.start()
	if got := len(fsys.staging.files); got != 4 {
		t.Fatalf("took back %d staged copies, want 4", got)
	}
	// Reads before the save see the writes taken back.
	if got := readAll(t, fsys, "data/big", int64(len(big))); !bytes.Equal(got, big) {
		t.Fatal("large file taken back reads differently")
	}
	// A write after the restart goes over them.
	n, _ = fsys.OpenFile("data/new", os.O_RDWR, 0)
	if _, err := n.Write([]byte("NEW")); err != nil {
		t.Fatal(err)
	}
	if err := fsys.Commit("data/new"); err != nil {
		t.Fatal(err)
	}
	flushAll(t, fsys)

	if got := readAll(t, fsys, "data/big", int64(len(big))); !bytes.Equal(got, big) {
		t.Fatal("large file's uncommitted part lost")
	}
	for name, want := range map[string]string{
		"data/small": "written, not committed",
		"data/cut":   strings.Repeat("x", 10),
		"data/new":   "NEW file",
	} {
		if got := readFile(t, fsys, name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if fi, err := fsys.Stat("data/new"); err != nil || fi.Mode().Perm() != 0640 {
		t.Errorf("new file's mode: %v, %v; want 0640", fi.Mode(), err)
	}

	// Saved, nothing is left to take back again.
	left, _ := filepath.Glob(filepath.Join(r.dir, "stage-*"))
	if len(left) != 0 {
		t.Fatalf("left in the staging folder after saving: %v", left)
	}
	kill(fsys)
	if fsys = r.start(); len(fsys.staging.files) != 0 {
		t.Fatalf("saved copies taken back again: %d", len(fsys.staging.files))
	}
}

// Staged files that never got a journal (the server killed as it made
// one) are removed, not taken back.
func TestUnjournaledStagedFilesRemoved(t *testing.T) {
	r := newRestartableFS(t)
	os.WriteFile(filepath.Join(r.dir, "stage-123"), []byte("half made"), 0600)
	os.WriteFile(filepath.Join(r.dir, "stage-456"), []byte("no record"), 0600)
	os.WriteFile(filepath.Join(r.dir, "stage-456"+journalSuffix), nil, 0600)
	os.WriteFile(filepath.Join(r.dir, "stage-789"+journalSuffix), []byte("{}\n"), 0600)
	fsys := r.start()
	if len(fsys.staging.files) != 0 {
		t.Fatalf("took back %d copies", len(fsys.staging.files))
	}
	left, _ := filepath.Glob(filepath.Join(r.dir, "stage-*"))
	if len(left) != 0 {
		t.Fatalf("left: %v", left)
	}
}

// A server that cannot save as it closes keeps what it could not save,
// for the next to save.
func TestCloseKeepsWhatItCannotSave(t *testing.T) {
	r := newRestartableFS(t)
	fsys := r.start()
	writeFile(t, fsys, "data/a", "saved")
	flushAll(t, fsys)
	writeFile(t, fsys, "data/a", "kept over a close")
	down := &flakyBackend{Backend: r.backend, err: errors.New("server unreachable")}
	down.fails.Store(1 << 30)
	fsys.backend = down
	fsys.staging.close()

	fsys = r.start()
	flushAll(t, fsys)
	if got := readFile(t, fsys, "data/a"); got != "kept over a close" {
		t.Fatalf("got %q", got)
	}
}

// Two servers never share a staging folder.
func TestStagingFolderLocked(t *testing.T) {
	dir := t.TempDir()
	l, err := lockStaging(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if l == nil {
		t.Skip("no file locks here")
	}
	if _, err := lockStaging(dir); err == nil {
		t.Fatal("a second server locked the staging folder")
	}
}

// Of two copies of one file left (which a server never leaves), the newer is
// taken back and saved.
func TestNewerOfTwoCopiesTakenBack(t *testing.T) {
	r := newRestartableFS(t)
	older := r.start()
	writeFile(t, older, "data/a", "older")
	kill(older)
	newer := newStoreFS(r.backend) // over the same folder, not taking back
	newer.staging = newStaging(newer, r.dir, time.Hour, time.Hour)
	time.Sleep(10 * time.Millisecond)
	writeFile(t, newer, "data/a", "newer")
	kill(newer)

	fsys := r.start()
	if len(fsys.staging.files) != 1 {
		t.Fatalf("took back %d copies, want 1", len(fsys.staging.files))
	}
	flushAll(t, fsys)
	if got := readFile(t, fsys, "data/a"); got != "newer" {
		t.Fatalf("got %q", got)
	}
	if left, _ := filepath.Glob(filepath.Join(r.dir, "stage-*")); len(left) != 0 {
		t.Fatalf("left: %v", left)
	}
}
