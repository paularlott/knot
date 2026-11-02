package nfsserver

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/rand"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
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

// opsBackend counts the lookups and deletes made of the server.
type opsBackend struct {
	Backend
	stats, deletes atomic.Int32
}

func (o *opsBackend) Stat(ctx context.Context, bucket, key string) (*Stat, error) {
	o.stats.Add(1)
	return o.Backend.Stat(ctx, bucket, key)
}

func (o *opsBackend) Delete(ctx context.Context, bucket, key string) error {
	o.deletes.Add(1)
	return o.Backend.Delete(ctx, bucket, key)
}

func TestListedFolderAnswersLookups(t *testing.T) {
	fsys, _ := newTestFS(t)
	ob := &opsBackend{Backend: fsys.backend}
	fsys.backend = ob
	fsys.Live(true)

	// A folder made here is known empty: creating in it asks nothing.
	if err := fsys.MkdirAll("data/new", 0755); err != nil {
		t.Fatal(err)
	}
	before := ob.stats.Load()
	for _, n := range []string{"a", "b", "c"} {
		writeFile(t, fsys, "data/new/"+n, n)
	}
	flushAll(t, fsys)
	if n := ob.stats.Load() - before; n != 0 {
		t.Fatalf("%d lookups to create files in a folder known empty", n)
	}
	if got := names(t, fsys, "data/new"); len(got) != 3 {
		t.Fatalf("listing = %v", got)
	}

	// After a listing, removing a file without an AppleDouble file is one
	// delete, not two.
	before = ob.deletes.Load()
	if err := fsys.Remove("data/new/a"); err != nil {
		t.Fatal(err)
	}
	if n := ob.deletes.Load() - before; n != 1 {
		t.Fatalf("%d deletes to remove a file with no AppleDouble file", n)
	}
	// One that has one still loses it.
	writeFile(t, fsys, "data/new/._b", "x")
	flushAll(t, fsys)
	names(t, fsys, "data/new")
	if err := fsys.Remove("data/new/b"); err != nil {
		t.Fatal(err)
	}
	if _, err := fsys.Stat("data/new/._b"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("AppleDouble file left: %v", err)
	}

	// A change elsewhere in the folder drops what was known of it.
	bucket, _ := fsys.bucket("data")
	fsys.Changed([]string{bucket.Id}, []FileChanged{{BucketId: bucket.Id, Key: "new/z", Writer: "other"}})
	if fsys.knownAbsent("data", "new/z") {
		t.Fatal("still known absent after another writer made it")
	}
}

func TestFolderTimeFollowsItsEntries(t *testing.T) {
	fsys, _ := newTestFS(t)
	fsys.MkdirAll("data/d", 0755)
	mtime := func() time.Time {
		t.Helper()
		fi, err := fsys.Stat("data/d")
		if err != nil {
			t.Fatal(err)
		}
		return fi.ModTime()
	}
	set := time.Unix(1500000000, 0)
	if err := fsys.Chtimes("data/d", set, set); err != nil {
		t.Fatal(err)
	}
	if !mtime().Equal(set) {
		t.Fatalf("time set not shown: %v", mtime())
	}

	// A file made in it moves its time on, as a local folder's does.
	time.Sleep(time.Millisecond)
	writeFile(t, fsys, "data/d/a", "a")
	flushAll(t, fsys)
	after := mtime()
	if !after.After(set) {
		t.Fatalf("folder time did not move when a file was made in it: %v", after)
	}
	// Listings show the same.
	infos, _ := fsys.ReadDir("data")
	for _, fi := range infos {
		if fi.Name() == "d" && !fi.ModTime().Equal(after) {
			t.Fatalf("listing shows %v, lookup %v", fi.ModTime(), after)
		}
	}

	// Set again once filled (as rsync does), the time set holds.
	if err := fsys.Chtimes("data/d", set, set); err != nil {
		t.Fatal(err)
	}
	if !mtime().Equal(set) {
		t.Fatalf("time set after the change not shown: %v", mtime())
	}

	// A removal, and a change by another writer, move it on again.
	time.Sleep(time.Millisecond)
	fsys.Remove("data/d/a")
	if !mtime().After(set) {
		t.Fatal("folder time did not move when a file was removed")
	}
	fsys.Chtimes("data/d", set, set)
	time.Sleep(time.Millisecond)
	bucket, _ := fsys.bucket("data")
	fsys.Changed([]string{bucket.Id}, []FileChanged{{BucketId: bucket.Id, Key: "d/z", Writer: "other"}})
	if !mtime().After(set) {
		t.Fatal("folder time did not move for another writer's change")
	}
}

// flakyBackend fails saves a number of times, as a server that is down.
type flakyBackend struct {
	Backend
	fails atomic.Int32
	err   error
}

func (f *flakyBackend) Put(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string, meta map[string]string) (*Object, error) {
	if f.fails.Add(-1) >= 0 {
		return nil, f.err
	}
	return f.Backend.Put(ctx, bucket, key, r, size, contentType, meta)
}

func TestSaveRetriedWhileServerDown(t *testing.T) {
	fsys, p := newTestFS(t)
	fb := &flakyBackend{Backend: fsys.backend, err: errors.New("unexpected status code: 502")}
	fb.fails.Store(20)
	fsys.backend = fb
	writeFile(t, fsys, "data/kept", "survives an outage")
	for i := 0; i < 20; i++ {
		if err := fsys.Commit("data/kept"); err == nil {
			t.Fatalf("commit %d reported a failed save as done", i)
		}
		if fsys.staging.get("data", "kept") == nil {
			t.Fatalf("writes dropped after %d failures", i+1)
		}
	}
	if err := fsys.Commit("data/kept"); err != nil {
		t.Fatalf("save once back: %v", err)
	}
	if o, err := fb.Backend.(*storeBackend).store.HeadObject(p, "alice--data", "kept"); err != nil || o.Size != 18 {
		t.Fatalf("not saved: %v %v", o, err)
	}

	// Refused for good: dropped, not retried for ever.
	fb.err = syscall.EDQUOT
	fb.fails.Store(1)
	writeFile(t, fsys, "data/over", "x")
	if err := fsys.Commit("data/over"); !errors.Is(err, syscall.EDQUOT) {
		t.Fatalf("over quota: %v", err)
	}
	if fsys.staging.get("data", "over") != nil {
		t.Fatal("refused writes kept")
	}
}

// A save the server refuses outside a commit (on the idle timer) is not
// lost silently: the next commit, and writes until then, report it.
func TestRefusedSaveReportedOnNextCommit(t *testing.T) {
	fsys, _ := newTestFS(t)
	writeFile(t, fsys, "data/q", "saved")
	flushAll(t, fsys)
	fb := &flakyBackend{Backend: fsys.backend, err: syscall.EDQUOT}
	fb.fails.Store(1)
	fsys.backend = fb
	f0, _ := fsys.OpenFile("data/q", os.O_RDWR, 0)
	f0.Write([]byte("over quota"))
	e := fsys.staging.get("data", "q")
	fsys.staging.flush(e) // as the idle timer does

	f, _ := fsys.OpenFile("data/q", os.O_RDWR, 0)
	if _, err := f.Write([]byte("more")); !errors.Is(err, syscall.EDQUOT) {
		t.Fatalf("write after dropped writes: %v", err)
	}
	if err := fsys.Commit("data/q"); !errors.Is(err, syscall.EDQUOT) {
		t.Fatalf("commit after dropped writes: %v", err)
	}
	// Told once; starting the file afresh works again.
	if err := fsys.Commit("data/q"); err != nil {
		t.Fatalf("reported twice: %v", err)
	}
	writeFile(t, fsys, "data/q", "fits now")
	if err := fsys.Commit("data/q"); err != nil {
		t.Fatalf("fresh write after the report: %v", err)
	}
}

// Cut short, then written past where it was cut: the gap is zeros, not the
// bytes the saved file had there, before and after saving.
func TestLargeFileCutThenWrittenPast(t *testing.T) {
	fsys, _ := newTestFS(t)
	want := make([]byte, sparseMin+1<<20)
	rand.New(rand.NewSource(5)).Read(want)
	f, _ := fsys.Create("data/big")
	f.Write(want)
	f.Close()
	flushAll(t, fsys)

	f, _ = fsys.OpenFile("data/big", os.O_RDWR, 0)
	f.Truncate(1 << 20)
	f.Seek(2<<20, io.SeekStart)
	f.Write([]byte("past the cut"))
	f.Close()
	want = append(want[:1<<20:1<<20], make([]byte, 1<<20)...)
	want = append(want, "past the cut"...)
	if got := readAll(t, fsys, "data/big", int64(len(want))); !bytes.Equal(got, want) {
		t.Fatal("staged: the gap shows the saved bytes")
	}
	flushAll(t, fsys)
	if got := readAll(t, fsys, "data/big", int64(len(want))); !bytes.Equal(got, want) {
		t.Fatal("saved: the gap holds the old bytes")
	}

	// Grown by a truncate well past everything: zeros, sent as such.
	f, _ = fsys.OpenFile("data/big", os.O_RDWR, 0)
	f.Truncate(int64(len(want)) + 3<<20)
	f.Close()
	want = append(want, make([]byte, 3<<20)...)
	flushAll(t, fsys)
	if got := readAll(t, fsys, "data/big", int64(len(want))); !bytes.Equal(got, want) {
		t.Fatal("grown: content differs")
	}
}

// slowBackend holds a lookup of one key until released, answering what it
// found when it was asked.
type slowBackend struct {
	Backend
	key     string
	held    atomic.Bool // the first lookup of key only
	asked   chan struct{}
	release chan struct{}
}

func (s *slowBackend) Stat(ctx context.Context, bucket, key string) (*Stat, error) {
	st, err := s.Backend.Stat(ctx, bucket, key)
	if key == s.key && s.held.CompareAndSwap(false, true) {
		close(s.asked)
		<-s.release
	}
	return st, err
}

// A lookup that began before a save and finishes after it must not leave
// its older answer kept in place of the save's.
func TestLookupRacingASaveIsNotKept(t *testing.T) {
	fsys, _ := newTestFS(t)
	writeFile(t, fsys, "data/r", "old")
	flushAll(t, fsys)
	fsys.Live(true)
	sb := &slowBackend{Backend: fsys.backend, key: "r", asked: make(chan struct{}), release: make(chan struct{})}
	fsys.backend = sb
	fsys.forget("data", "r", false)
	asked := sb.asked

	done := make(chan struct{})
	go func() {
		defer close(done)
		fsys.Stat("data/r") // sees "old", slowly
	}()
	<-asked
	f, _ := fsys.OpenFile("data/r", os.O_RDWR|os.O_TRUNC, 0)
	f.Write([]byte("much newer"))
	f.Close()
	flushAll(t, fsys)
	close(sb.release)
	<-done

	fi, err := fsys.Stat("data/r")
	if err != nil || fi.Size() != int64(len("much newer")) {
		t.Fatalf("stale lookup kept: size %d %v", fi.Size(), err)
	}
}

// failMoveBackend refuses moves, as a server that is down.
type failMoveBackend struct{ Backend }

func (failMoveBackend) Move(context.Context, string, string, string) error {
	return errors.New("unexpected status code: 502")
}

func TestOverwriteElsewhereKeepsHandles(t *testing.T) {
	fsys, _ := newTestFS(t)
	writeFile(t, fsys, "data/w", "nfs")
	flushAll(t, fsys)
	h := newHandler(fsys)
	fh := h.ToHandle(fsys, []string{"data", "w"})

	// Written whole by another client, which keeps no nfs-ino.
	if _, err := fsys.backend.Put(t.Context(), "data", "w", strings.NewReader("web upload"), 10, "", nil); err != nil {
		t.Fatal(err)
	}
	fsys.Changed(nil, nil)
	if _, _, err := h.FromHandle(fh); err != nil {
		t.Fatalf("handle stale after an overwrite elsewhere: %v", err)
	}
}

func TestFailedRenameKeepsSource(t *testing.T) {
	fsys, _ := newTestFS(t)
	fsys.Live(true)
	writeFile(t, fsys, "data/src", "x")
	flushAll(t, fsys)
	names(t, fsys, "data") // the folder's names known
	fsys.backend = failMoveBackend{fsys.backend}
	if err := fsys.Rename("data/src", "data/dst"); err == nil {
		t.Fatal("rename reported done")
	}
	if _, err := fsys.Stat("data/src"); err != nil {
		t.Fatalf("source hidden after a failed rename: %v", err)
	}
}
