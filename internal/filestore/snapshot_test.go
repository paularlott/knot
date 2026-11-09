package filestore

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSnapshotFolder(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "src")
	mustCreate(t, s, alice, "dst")
	put(t, s, alice, "src", "proj/", "")
	put(t, s, alice, "src", "proj/a.go", "package a")
	put(t, s, alice, "src", "proj/sub/b.go", "package b")
	put(t, s, alice, "src", "proj/sub/", "")
	put(t, s, alice, "src", "projx", "beside the folder")
	mtime := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if _, err := s.PutObject(alice, "src", "proj/run.sh", strings.NewReader("#!/bin/sh"), PutOptions{Size: 9, Meta: map[string]string{"mode": "100755"}, ModifiedAt: mtime}); err != nil {
		t.Fatal(err)
	}
	src, _ := s.HeadObject(alice, "src", "proj/run.sh")

	snap, err := s.SnapshotFolder(alice, "src", "proj/", "dst", "snaps/one")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Files != 3 || snap.Folders != 2 || snap.Bytes != int64(len("package a")+len("package b")+9) {
		t.Errorf("snapshot reports %+v", snap)
	}
	for key, want := range map[string]string{"snaps/one/a.go": "package a", "snaps/one/sub/b.go": "package b", "snaps/one/run.sh": "#!/bin/sh"} {
		if got := read(t, s, alice, "dst", key); got != want {
			t.Errorf("%s reads %q", key, got)
		}
	}
	for _, key := range []string{"snaps/one/", "snaps/one/sub/"} {
		if _, err := s.HeadObject(alice, "dst", key); err != nil {
			t.Errorf("folder %s: %v", key, err)
		}
	}
	if _, err := s.HeadObject(alice, "dst", "snaps/onex"); !errors.Is(err, ErrNoSuchKey) {
		t.Errorf("a file beside the folder was copied: %v", err)
	}
	// A file's mode and times come with it.
	c, _ := s.HeadObject(alice, "dst", "snaps/one/run.sh")
	if c.Meta["mode"] != "100755" || !c.ModifiedAt.Equal(src.ModifiedAt) || c.SHA256 != src.SHA256 {
		t.Errorf("copy %+v of %+v", c, src)
	}
	// The manifest says what it is.
	var m Snapshot
	if err := json.Unmarshal([]byte(read(t, s, alice, "dst", "snaps/one/"+SnapshotManifest)), &m); err != nil {
		t.Fatal(err)
	}
	if m.SourceBucket != "src" || m.SourcePath != "proj" || m.Files != 3 || m.TakenAt.IsZero() {
		t.Errorf("manifest %+v", m)
	}

	// A snapshot of a snapshot has its own manifest, not the one copied.
	if _, err := s.SnapshotFolder(alice, "dst", "snaps/one", "dst", "snaps/two"); err != nil {
		t.Fatal(err)
	}
	var m2 Snapshot
	json.Unmarshal([]byte(read(t, s, alice, "dst", "snaps/two/"+SnapshotManifest)), &m2)
	if m2.SourceBucket != "dst" || m2.SourcePath != "snaps/one" {
		t.Errorf("second manifest %+v", m2)
	}

	// The whole bucket, into another.
	if snap, err := s.SnapshotFolder(alice, "src", "", "dst", "all"); err != nil || snap.Files != 4 {
		t.Fatalf("whole bucket: %+v, %v", snap, err)
	}
	if got := read(t, s, alice, "dst", "all/projx"); got != "beside the folder" {
		t.Errorf("whole bucket copy reads %q", got)
	}
}

func TestSnapshotRefusals(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "src")
	mustCreate(t, s, bob, "other")
	put(t, s, alice, "src", "proj/a", "a")
	put(t, s, alice, "src", "taken/x", "x")
	put(t, s, alice, "src", "file", "f")

	count := func() int {
		res, err := s.ListObjects(alice, "src", "", "", "", 1000)
		if err != nil {
			t.Fatal(err)
		}
		return len(res.Objects)
	}
	before := count()
	for name, c := range map[string]struct {
		from, to string
		want     error
	}{
		"onto a folder":     {"proj", "taken", ErrDestinationExists},
		"onto a file":       {"proj", "file", ErrDestinationExists},
		"into itself":       {"proj", "proj/inside", ErrInvalidKey},
		"onto itself":       {"proj", "proj", ErrInvalidKey},
		"whole bucket here": {"", "all", ErrInvalidKey},
		"nothing there":     {"none", "new", ErrNoSuchKey},
		"bad destination":   {"proj", "a/../b", ErrInvalidKey},
		"no destination":    {"proj", "/", ErrInvalidKey},
	} {
		if _, err := s.SnapshotFolder(alice, "src", c.from, "src", c.to); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
	if n := count(); n != before {
		t.Errorf("refusals wrote %d files", n-before)
	}

	// Reading needs read access, writing write access.
	if _, err := s.SnapshotFolder(bob, "src", "proj", "other", "copy"); !errors.Is(err, ErrNoSuchBucket) {
		t.Errorf("unshared source: %v", err)
	}
	if _, err := s.SnapshotFolder(alice, "src", "proj", "other", "copy"); !errors.Is(err, ErrNoSuchBucket) {
		t.Errorf("unshared destination: %v", err)
	}
	s.SetGrant(alice, "src", Grant{Type: GrantUser, Id: "bob", Access: GrantRead})
	if _, err := s.SnapshotFolder(bob, "src", "proj", "src", "bobs"); !errors.Is(err, ErrAccessDenied) && !errors.Is(err, ErrNoSuchBucket) {
		t.Errorf("read-only destination: %v", err)
	}
	if _, err := s.SnapshotFolder(bob, "src", "proj", "other", "copy"); err != nil {
		t.Errorf("read share into own bucket: %v", err)
	}
}

func TestSnapshotQuota(t *testing.T) {
	limits := map[string]int64{}
	s := openStore(t, t.TempDir(), func(userId string) (int64, error) { return limits[userId], nil })
	defer s.Close()
	mustCreate(t, s, alice, "src")
	put(t, s, alice, "src", "proj/a", strings.Repeat("a", 100))
	put(t, s, alice, "src", "proj/b", strings.Repeat("b", 100))

	// Charged in full, as a copy is: not enough room refuses it whole.
	limits["alice"] = 300
	if _, err := s.SnapshotFolder(alice, "src", "proj", "src", "snap"); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("over quota: %v", err)
	}
	if u := s.Usage("alice"); u.UsedBytes != 200 {
		t.Errorf("a refused snapshot left %d bytes", u.UsedBytes-200)
	}

	// Room for the files but not the manifest: what was written is taken
	// back.
	limits["alice"] = 405
	if _, err := s.SnapshotFolder(alice, "src", "proj", "src", "snap"); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("no room for the manifest: %v", err)
	}
	if u := s.Usage("alice"); u.UsedBytes != 200 || u.Objects != 2 {
		t.Errorf("a failed snapshot left %d bytes in %d files", u.UsedBytes-200, u.Objects-2)
	}
	if _, err := s.HeadObject(alice, "src", "snap/a"); !errors.Is(err, ErrNoSuchKey) {
		t.Errorf("file left from a failed snapshot: %v", err)
	}

	limits["alice"] = 1000
	if _, err := s.SnapshotFolder(alice, "src", "proj", "src", "snap"); err != nil {
		t.Fatalf("after the failure, the same snapshot: %v", err)
	}
	if u := s.Usage("alice"); u.UsedBytes < 400 {
		t.Errorf("snapshot charged %d bytes, want the full 200", u.UsedBytes-200)
	}
}

// A failed snapshot takes back only what is still as it wrote it.
func TestSnapshotTakeBackLeavesLaterWrites(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "src")
	put(t, s, alice, "src", "proj/a", "a")
	put(t, s, alice, "src", "proj/b", "b")

	_, written, err := s.snapshotOnce(alice, "src", "proj", "src", "snap", 0)
	if err != nil {
		t.Fatal(err)
	}
	put(t, s, alice, "src", "snap/b", "written since")
	s.mu.RLock()
	s.takeBack(written)
	s.mu.RUnlock()
	if _, err := s.HeadObject(alice, "src", "snap/a"); !errors.Is(err, ErrNoSuchKey) {
		t.Errorf("snapshot file not taken back: %v", err)
	}
	if got := read(t, s, alice, "src", "snap/b"); got != "written since" {
		t.Errorf("a later write was taken back: %q", got)
	}
}

// Every file is as it was at one moment, however fast the folder changes.
// A writer stamps each file of the folder with a generation, in order, so
// at any moment the generations fall, by at most one, from the first file
// to the last; a copy made a file at a time, catching later files after
// the writer has moved on, would show a later file newer than an earlier.
func TestSnapshotIsOfOneMoment(t *testing.T) {
	withGrace(t, time.Hour) // as a server holds replaced content
	s := newStore(t)
	mustCreate(t, s, alice, "src")
	const files = 60
	key := func(i int) string { return fmt.Sprintf("proj/f%03d", i) }
	for i := range files {
		put(t, s, alice, "src", key(i), "0")
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var gens atomic.Int64
	wg.Add(1)
	go func() {
		defer wg.Done()
		for g := 1; ; g++ {
			for i := range files {
				select {
				case <-stop:
					return
				default:
				}
				v := strconv.Itoa(g)
				if _, err := s.PutObject(alice, "src", key(i), strings.NewReader(v), PutOptions{Size: int64(len(v))}); err != nil {
					t.Error(err)
					return
				}
			}
			gens.Store(int64(g))
		}
	}()

	for n := range 40 {
		to := fmt.Sprintf("snap%d", n)
		if _, err := s.SnapshotFolder(alice, "src", "proj", "src", to); err != nil {
			t.Fatal(err)
		}
		prev := -1
		first := 0
		for i := range files {
			g, _ := strconv.Atoi(read(t, s, alice, "src", to+"/"+key(i)[len("proj/"):]))
			if i == 0 {
				first = g
			} else if g > prev || first-g > 1 {
				t.Fatalf("snapshot %d is not of one moment: file %d is at generation %d after %d (the first at %d)", n, i, g, prev, first)
			}
			prev = g
		}
	}
	close(stop)
	wg.Wait()
	if gens.Load() < 2 {
		t.Logf("the writer made only %d passes", gens.Load())
	}
}

// Content let go between the read and the write (none is held for reuse,
// as in these tests) is never shared by a snapshot: it is read again.
func TestSnapshotOfContentLetGo(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "src")
	put(t, s, alice, "src", "proj/a", "first")
	put(t, s, alice, "src", "proj/b", "b")

	tries := 0
	beforeSnapshotWrite = func() {
		tries++
		if tries == 1 {
			// Replaced after the read: "first" goes with its last reference.
			put(t, s, alice, "src", "proj/a", "second")
		}
	}
	defer func() { beforeSnapshotWrite = nil }()
	if _, err := s.SnapshotFolder(alice, "src", "proj", "src", "snap"); err != nil {
		t.Fatal(err)
	}
	if tries != 2 {
		t.Errorf("%d tries, want 2", tries)
	}
	if got := read(t, s, alice, "src", "snap/a"); got != "second" {
		t.Errorf("snapshot reads %q", got)
	}
	if len(s.missing) != 0 {
		t.Errorf("snapshot left content missing")
	}

	// Replaced on every try: refused, with nothing left behind.
	put(t, s, alice, "src", "other/a", "again 0")
	put(t, s, alice, "src", "other/b", "b")
	n := 0
	beforeSnapshotWrite = func() {
		n++
		put(t, s, alice, "src", "other/a", fmt.Sprintf("again %d", n))
	}
	if _, err := s.SnapshotFolder(alice, "src", "other", "src", "snap2"); !errors.Is(err, ErrSnapshotChanged) {
		t.Fatalf("always replaced: %v", err)
	}
	if res, _ := s.ListObjects(alice, "src", "snap2/", "", "", 10); len(res.Objects) != 0 {
		t.Errorf("a refused snapshot left %d files", len(res.Objects))
	}
}
