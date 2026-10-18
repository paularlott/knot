package nfsserver

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/rand"
	"os"
	"sync/atomic"
	"testing"
)

func TestSpans(t *testing.T) {
	var s spans
	s.add(10, 20)
	s.add(30, 40)
	s.add(20, 25) // touches the first
	s.add(5, 8)
	if want := (spans{{5, 8}, {10, 25}, {30, 40}}); !equalSpans(s, want) {
		t.Fatalf("after adds: %v", s)
	}
	s.add(7, 35) // joins all three
	if want := (spans{{5, 40}}); !equalSpans(s, want) {
		t.Fatalf("after join: %v", s)
	}
	s.add(50, 60)
	s.trim(55)
	if want := (spans{{5, 40}, {50, 55}}); !equalSpans(s, want) {
		t.Fatalf("after trim: %v", s)
	}
	for _, c := range []struct {
		off    int64
		inside bool
		until  int64
	}{{0, false, 5}, {5, true, 40}, {39, true, 40}, {40, false, 50}, {52, true, 55}} {
		if in, until := s.at(c.off); in != c.inside || until != c.until {
			t.Errorf("at(%d) = %v %d", c.off, in, until)
		}
	}
	if s.size() != 40 {
		t.Errorf("size = %d", s.size())
	}
}

func equalSpans(a, b spans) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func readAll(t *testing.T, fsys *storeFS, name string, size int64) []byte {
	t.Helper()
	f, err := fsys.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := make([]byte, 0, size)
	buf := make([]byte, 1<<20)
	for off := int64(0); ; {
		n, err := f.ReadAt(buf, off)
		out = append(out, buf[:n]...)
		off += int64(n)
		if errors.Is(err, io.EOF) || n == 0 {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestLargeFileChangedInPlace(t *testing.T) {
	fsys, _ := newTestFS(t)
	want := make([]byte, sparseMin+1<<20)
	rand.New(rand.NewSource(3)).Read(want)
	f, _ := fsys.Create("data/big")
	f.Write(want)
	f.Close()
	flushAll(t, fsys)

	before := patches.Load()
	f, _ = fsys.OpenFile("data/big", os.O_RDWR, 0)
	if e := fsys.staging.get("data", "big"); e != nil {
		t.Fatal("staged before any write")
	}
	// A change in the middle and an append.
	f.Seek(5<<20, io.SeekStart)
	f.Write([]byte("changed in the middle"))
	copy(want[5<<20:], "changed in the middle")
	f.Seek(int64(len(want)), io.SeekStart)
	f.Write([]byte("appended"))
	want = append(want, "appended"...)
	f.Close()

	e := fsys.staging.get("data", "big")
	if e == nil || !e.sparse || e.written.size() > 100 {
		t.Fatalf("not changed in place: %+v", e)
	}
	// Read before it is saved: the writes over the saved file.
	if got := readAll(t, fsys, "data/big", int64(len(want))); !bytes.Equal(got, want) {
		t.Fatalf("staged read differs (%d bytes, want %d)", len(got), len(want))
	}
	flushAll(t, fsys)
	if patches.Load() != before+1 {
		t.Fatalf("saved whole, not patched")
	}
	if got := readAll(t, fsys, "data/big", int64(len(want))); !bytes.Equal(got, want) {
		t.Fatalf("saved content differs")
	}

	// Cut short and grown again: what was cut reads as zeros.
	f, _ = fsys.OpenFile("data/big", os.O_RDWR, 0)
	f.Truncate(1 << 20)
	f.Truncate(2 << 20)
	f.Close()
	want = append(want[:1<<20:1<<20], make([]byte, 1<<20)...)
	flushAll(t, fsys)
	if got := readAll(t, fsys, "data/big", int64(len(want))); !bytes.Equal(got, want) {
		t.Fatalf("after cut and grow: differs")
	}
}

// countingBackend counts reads of file content.
type countingBackend struct {
	Backend
	reads atomic.Int32
}

func (c *countingBackend) ReadAt(ctx context.Context, bucket, key string, p []byte, off int64) (int, error) {
	c.reads.Add(1)
	return c.Backend.ReadAt(ctx, bucket, key, p, off)
}

func (c *countingBackend) Get(ctx context.Context, bucket, key string) (io.ReadCloser, *Object, error) {
	c.reads.Add(1)
	return c.Backend.Get(ctx, bucket, key)
}

func TestDiskCache(t *testing.T) {
	fsys, _ := newTestFS(t)
	cb := &countingBackend{Backend: fsys.backend}
	fsys.backend = cb
	cache, err := openDiskCache(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	fsys.cache = cache
	fsys.staging.dir = cache.stagingDir()
	os.MkdirAll(fsys.staging.dir, 0700)

	// A file saved is kept: reading it needs nothing from the server.
	writeFile(t, fsys, "data/a.txt", "cached content")
	flushAll(t, fsys)
	if got := readFile(t, fsys, "data/a.txt"); got != "cached content" {
		t.Fatalf("read = %q", got)
	}
	if n := cb.reads.Load(); n != 0 {
		t.Fatalf("%d reads from the server for a kept file", n)
	}
	// Changing it again copies it from the cache.
	f, _ := fsys.OpenFile("data/a.txt", os.O_RDWR, 0)
	f.Seek(0, io.SeekStart)
	f.Write([]byte("CACHED"))
	f.Close()
	flushAll(t, fsys)
	if n := cb.reads.Load(); n != 0 {
		t.Fatalf("%d reads from the server to change a kept file", n)
	}
	if got := readFile(t, fsys, "data/a.txt"); got != "CACHED content" {
		t.Fatalf("after change = %q", got)
	}

	// The cache stays within its size, dropping the least recently used.
	for i := 0; i < 8; i++ {
		writeFile(t, fsys, "data/f"+string(rune('a'+i)), string(make([]byte, 200<<10)))
		flushAll(t, fsys)
	}
	cache.mu.Lock()
	total := cache.total
	cache.mu.Unlock()
	if total > 1<<20 {
		t.Fatalf("cache holds %d bytes, over its size", total)
	}

	// Opened again, it finds what it held.
	again, err := openDiskCache(cache.dir, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if again.total != total {
		t.Fatalf("reopened cache holds %d, want %d", again.total, total)
	}
}

func TestChangeEventsDropDescriptions(t *testing.T) {
	fsys, p := newTestFS(t)
	writeFile(t, fsys, "data/x", "one")
	flushAll(t, fsys)
	fsys.Live(true)
	if fsys.ttl() != liveStatTTL {
		t.Fatal("not kept longer while live")
	}
	if _, err := fsys.Stat("data/x"); err != nil {
		t.Fatal(err)
	}

	// Changed behind the filesystem's back, then told.
	store := fsys.backend.(*storeBackend).store
	if _, err := store.PutObject(p, "alice--data", "x", bytes.NewReader([]byte("changed")), putOpts(7)); err != nil {
		t.Fatal(err)
	}
	if fi, _ := fsys.Stat("data/x"); fi.Size() != 3 {
		t.Fatalf("expected the kept description before the event, size %d", fi.Size())
	}
	b, _ := fsys.bucket("data")
	fsys.Changed([]string{b.Id}, nil)
	if fi, _ := fsys.Stat("data/x"); fi.Size() != 7 {
		t.Fatalf("description kept after the event, size %d", fi.Size())
	}

	fsys.Live(false)
	if fsys.ttl() != statTTL {
		t.Fatal("kept long while not live")
	}
}

func TestChangeEventsDropOnlyWhatChanged(t *testing.T) {
	fsys, p := newTestFS(t)
	fsys.writer = "me"
	for _, k := range []string{"a/x", "a/y", "b"} {
		writeFile(t, fsys, "data/"+k, "one")
	}
	flushAll(t, fsys)
	fsys.Live(true)
	for _, k := range []string{"a", "a/x", "a/y", "b"} {
		if _, err := fsys.Stat("data/" + k); err != nil {
			t.Fatal(err)
		}
	}
	bucket, _ := fsys.bucket("data")
	kept := func(key string) bool {
		_, ok := fsys.cached("data", key)
		return ok
	}

	// Another writer changed a/x: it and the folders above go, the rest stays.
	fsys.Changed([]string{bucket.Id}, []FileChanged{{BucketId: bucket.Id, Key: "a/x", Writer: "someone-else"}})
	if kept("a/x") || kept("a") || kept("") {
		t.Error("the changed file or the folders above it still kept")
	}
	if !kept("a/y") || !kept("b") {
		t.Error("files that did not change were dropped")
	}

	// This server's own change is passed over.
	fsys.Stat("data/a/x")
	fsys.Changed([]string{bucket.Id}, []FileChanged{{BucketId: bucket.Id, Key: "a/x", Writer: "me"}})
	if !kept("a/x") {
		t.Error("dropped for its own change")
	}

	// A bucket named without files changed as a whole.
	fsys.Changed([]string{bucket.Id}, nil)
	if kept("a/y") || kept("b") {
		t.Error("whole-bucket change kept files")
	}
	_ = p
}
