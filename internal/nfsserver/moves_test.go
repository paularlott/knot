package nfsserver

import (
	"strings"
	"testing"
	"time"
)

// A handle made before a rename here still finds the file where it went:
// the Mac's client renames a file removed while open to ".nfs..." and goes
// on using it by its old handle, to write, unlock and close it. Told it was
// stale, its locks were never released and processes waiting for them
// waited forever.
func TestHandleFollowsRename(t *testing.T) {
	fsys, _ := newTestFS(t)
	h := newHandler(fsys)
	resolve := func(fh []byte) (string, error) {
		_, p, err := h.FromHandle(fh)
		return strings.Join(p, "/"), err
	}

	writeFile(t, fsys, "data/._f", "first")
	first := h.ToHandle(fsys, []string{"data", "._f"})
	if err := fsys.Rename("data/._f", "data/.nfs.1"); err != nil {
		t.Fatal(err)
	}
	if p, err := resolve(first); err != nil || p != "data/.nfs.1" {
		t.Errorf("a renamed file's handle: %q, %v; want data/.nfs.1", p, err)
	}

	// Another file made at the old name keeps it; the first's handle still
	// finds the first, renamed again.
	writeFile(t, fsys, "data/._f", "second")
	second := h.ToHandle(fsys, []string{"data", "._f"})
	if err := fsys.Rename("data/._f", "data/.nfs.2"); err != nil {
		t.Fatal(err)
	}
	if err := fsys.Rename("data/.nfs.1", "data/.nfs.3"); err != nil {
		t.Fatal(err)
	}
	if p, err := resolve(first); err != nil || p != "data/.nfs.3" {
		t.Errorf("the first file's handle after two renames: %q, %v; want data/.nfs.3", p, err)
	}
	if p, err := resolve(second); err != nil || p != "data/.nfs.2" {
		t.Errorf("the second file's handle: %q, %v; want data/.nfs.2", p, err)
	}

	// A file in a renamed folder.
	if err := fsys.MkdirAll("data/d", 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, fsys, "data/d/x", "x")
	inFolder := h.ToHandle(fsys, []string{"data", "d", "x"})
	if err := fsys.Rename("data/d", "data/e"); err != nil {
		t.Fatal(err)
	}
	if p, err := resolve(inFolder); err != nil || p != "data/e/x" {
		t.Errorf("the handle of a file in a renamed folder: %q, %v; want data/e/x", p, err)
	}

	// Removed, it is stale.
	if err := fsys.Remove("data/.nfs.3"); err != nil {
		t.Fatal(err)
	}
	if p, err := resolve(first); err == nil {
		t.Errorf("a removed file's handle found %q", p)
	}
}

// Moves are forgotten after a while, and their number is bounded.
func TestMovesForgotten(t *testing.T) {
	var m moves
	now := time.Now()
	m.add("b/a", "b/c", 7, now)
	if to, ok := m.find("b/a", 7, now); !ok || to != "b/c" {
		t.Errorf("find = %q, %v", to, ok)
	}
	if _, ok := m.find("b/a", 8, now); ok {
		t.Error("found the move of another file")
	}
	if _, ok := m.find("b/a", 7, now.Add(movesKept)); ok {
		t.Error("a move was remembered past movesKept")
	}
	for i := 0; i < movesMax+10; i++ {
		m.add("b/x", "b/y", 1, now)
	}
	if m.n > movesMax {
		t.Errorf("%d moves remembered, more than %d", m.n, movesMax)
	}
}

// A file another server replaced, while a client here had it open, is the
// same file changed: the client goes on with it, last write winning, rather
// than find its handle stale mid-write. A handle not used lately, or of a
// file removed here, is still of another file.
func TestHandleOfFileReplacedElsewhere(t *testing.T) {
	fsys, _ := newTestFS(t)
	h := newHandler(fsys)
	resolve := func(fh []byte) error {
		_, _, err := h.FromHandle(fh)
		return err
	}
	replace := func(name, ino string) {
		t.Helper()
		if _, err := fsys.backend.Put(t.Context(), "data", name, strings.NewReader("there"), 5, "", map[string]string{metaIno: ino}); err != nil {
			t.Fatal(err)
		}
		fsys.forgetChanged("data", name)
	}

	// In use here, then replaced elsewhere: the same file.
	writeFile(t, fsys, "data/f", "here")
	flushAll(t, fsys)
	fh := h.ToHandle(fsys, []string{"data", "f"})
	if err := resolve(fh); err != nil {
		t.Fatal(err)
	}
	replace("f", "abc1")
	if err := resolve(fh); err != nil {
		t.Errorf("the handle of a file replaced elsewhere while in use: %v", err)
	}

	// Not used lately: another file.
	writeFile(t, fsys, "data/g", "here")
	flushAll(t, fsys)
	fg := h.ToHandle(fsys, []string{"data", "g"})
	fsys.moved.mu.Lock()
	clear(fsys.moved.seen)
	fsys.moved.mu.Unlock()
	replace("g", "abc2")
	if err := resolve(fg); err == nil {
		t.Error("a handle not used lately reached a file replaced since")
	}

	// Removed here and made again: another file.
	writeFile(t, fsys, "data/h", "here")
	flushAll(t, fsys)
	fhh := h.ToHandle(fsys, []string{"data", "h"})
	if err := resolve(fhh); err != nil {
		t.Fatal(err)
	}
	if err := fsys.Remove("data/h"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, fsys, "data/h", "again")
	flushAll(t, fsys)
	if err := resolve(fhh); err == nil {
		t.Error("the handle of a file removed here reached the one made again")
	}
}
