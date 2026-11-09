package nfsserver

import (
	"bytes"
	"context"
	"io"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// stageMergeOver stages a change to a large file, "mine" at 1 MB, and has
// another client save other over the file before it is saved.
func stageMergeOver(t *testing.T, fsys *storeFS, data, other []byte) {
	t.Helper()
	f, _ := fsys.Create("data/m")
	f.Write(data)
	f.Close()
	flushAll(t, fsys)
	f, _ = fsys.OpenFile("data/m", os.O_RDWR, 0)
	f.Seek(1<<20, io.SeekStart)
	f.Write([]byte("mine"))
	f.Close()
	if _, err := fsys.backend.Put(t.Context(), "data", "m", bytes.NewReader(other), int64(len(other)), "", nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
}

// A change made to a merged file before it is recorded again, its mode
// say, is kept: the file is recorded again only as it was saved.
func TestMergeNoticeKeepsLaterChanges(t *testing.T) {
	defer func(d time.Duration) { mergeNotice = d }(mergeNotice)
	mergeNotice = 200 * time.Millisecond
	fsys, _ := newTestFS(t)
	data := make([]byte, sparseMin+1<<20)
	stageMergeOver(t, fsys, data, bytes.Repeat([]byte("x"), len(data)))
	flushAll(t, fsys)
	if err := fsys.Chmod("data/m", 0600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * mergeNotice)
	fsys.forget("data", "m", false)
	fi, err := fsys.Stat("data/m")
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("mode %v: recording the merged file again undid the chmod made since", fi.Mode().Perm())
	}
}

// Changes saved over a newer version shorter than the one they were
// written to are kept, what lies past its end and was not written here
// reading as zeros, rather than refused and dropped.
func TestMergeOverAShorterVersionIsSaved(t *testing.T) {
	fsys, _ := newTestFS(t)
	data := bytes.Repeat([]byte("o"), sparseMin+2<<20)
	shorter := bytes.Repeat([]byte("n"), sparseMin+1<<20)
	stageMergeOver(t, fsys, data, shorter)
	flushAll(t, fsys)
	fsys.forget("data", "m", false)
	got := readAll(t, fsys, "data/m", int64(len(data)))
	want := make([]byte, len(data))
	copy(want, shorter)
	copy(want[1<<20:], "mine")
	if !bytes.Equal(got, want) {
		t.Errorf("the merged file is not the shorter version with the change over it, zeros past its end (%d bytes)", len(got))
	}
}

// busyPatchBackend answers patches busy until it is told to stop.
type busyPatchBackend struct {
	Backend
	busy atomic.Bool
}

func (b *busyPatchBackend) Patch(ctx context.Context, bucket, key string, ranges []Range, size int64, ifMatch, contentType string, meta map[string]string) (*Object, error) {
	if b.busy.Load() {
		return nil, errBusy
	}
	return b.Backend.Patch(ctx, bucket, key, ranges, size, ifMatch, contentType, meta)
}

// While a save is tried again and again, the file being busy, the file
// can be looked at: the copy is not held between tries.
func TestFileCanBeLookedAtWhileItsSaveWaits(t *testing.T) {
	fsys, _ := newTestFS(t)
	data := make([]byte, sparseMin+1<<20)
	f, _ := fsys.Create("data/m")
	f.Write(data)
	f.Close()
	flushAll(t, fsys)
	bb := &busyPatchBackend{Backend: fsys.backend}
	bb.busy.Store(true)
	fsys.backend = bb
	f, _ = fsys.OpenFile("data/m", os.O_RDWR, 0)
	f.Seek(1<<20, io.SeekStart)
	f.Write([]byte("mine"))
	f.Close()

	saved := make(chan error, 1)
	e := fsys.staging.get("data", "m")
	go func() { saved <- fsys.staging.flush(e) }()
	time.Sleep(300 * time.Millisecond)
	start := time.Now()
	if _, err := fsys.Stat("data/m"); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 200*time.Millisecond {
		t.Errorf("looking at the file waited %v for its save", d)
	}
	bb.busy.Store(false)
	if err := <-saved; err != nil {
		t.Fatal(err)
	}
}

// preconditionBackend has every read of a given version find the file
// changed, as a file rewritten faster than it can be looked at and read.
type preconditionBackend struct{ Backend }

func (b *preconditionBackend) ReadAt(ctx context.Context, bucket, key, etag string, p []byte, off int64) (int, error) {
	if etag != "" {
		return 0, errPrecondition
	}
	return b.Backend.ReadAt(ctx, bucket, key, "", p, off)
}

func (b *preconditionBackend) Get(ctx context.Context, bucket, key, etag string) (io.ReadCloser, *Object, error) {
	if etag != "" {
		return nil, nil, errPrecondition
	}
	return b.Backend.Get(ctx, bucket, key, "")
}

// A file that has changed every time it is read is read as it is, rather
// than the read failing.
func TestFileChangingAsReadIsStillRead(t *testing.T) {
	fsys, _ := newTestFS(t)
	writeFile(t, fsys, "data/c", "content")
	flushAll(t, fsys)
	fsys.backend = &preconditionBackend{Backend: fsys.backend}
	fsys.cache = nil
	f, err := fsys.Open("data/c")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	buf := make([]byte, 7)
	if n, err := f.ReadAt(buf, 0); string(buf[:n]) != "content" {
		t.Errorf("read %q, %v", buf[:n], err)
	}
}
