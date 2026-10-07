package filestore

import (
	"context"
	"strings"
	"testing"

	badger "github.com/dgraph-io/badger/v4"
)

func fsck(t *testing.T, s *Store, opt FsckOptions) *FsckReport {
	t.Helper()
	r, err := s.Fsck(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func clean(t *testing.T, s *Store, what string) {
	t.Helper()
	if r := fsck(t, s, FsckOptions{Deep: true}); len(r.Found) != 0 {
		t.Fatalf("%s: found %v %v", what, r.Found, r.Issues)
	}
}

func TestFsckHealthyStore(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "fine")
	put(t, s, alice, "fine", "a.txt", "alpha")
	put(t, s, alice, "fine", "copy.txt", "alpha")
	put(t, s, alice, "fine", "big.bin", strings.Repeat("B", inlineMax+5))
	put(t, s, alice, "fine", "gone.txt", "gone")
	s.DeleteObject(alice, "fine", "gone.txt")
	mustCreate(t, s, bob, "other")
	s.DeleteBucket(bob, "other", false)
	waitFor(t, "sweep", func() bool { return s.refCount(contentSHA("gone")) == 0 })

	r := fsck(t, s, FsckOptions{Deep: true})
	if len(r.Found) != 0 || r.Unresolved() != 0 || r.Files != 3 || r.Buckets != 1 {
		t.Errorf("report %+v", r)
	}
}

func TestFsckMissingContentWithoutCluster(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "lost")
	small := put(t, s, alice, "lost", "small.txt", "small")
	big := put(t, s, alice, "lost", "big.bin", strings.Repeat("B", inlineMax+5))
	s.forgetContent(small.SHA256)
	s.forgetContent(big.SHA256)

	r := fsck(t, s, FsckOptions{})
	if r.Found[FsckMissingContent] != 2 || r.Found[FsckMissingMarker] != 2 {
		t.Fatalf("found %v", r.Found)
	}
	r = fsck(t, s, FsckOptions{Repair: true})
	if r.Repaired[FsckMissingMarker] != 2 || r.Repaired[FsckMissingContent] != 0 || r.Unresolved() != 2 || len(r.Notes) == 0 {
		t.Errorf("repair %+v", r)
	}
	// The files are queued, so a cluster would fetch them; the records stay.
	if len(s.missing) != 2 {
		t.Errorf("%d queued", len(s.missing))
	}
	if r = fsck(t, s, FsckOptions{}); r.Found[FsckMissingMarker] != 0 || r.Found[FsckMissingContent] != 2 {
		t.Errorf("after repair %v", r.Found)
	}
}

func TestFsckFetchesFromCluster(t *testing.T) {
	_, stores := newCluster(t, 2)
	a, b := stores[0], stores[1]
	mustCreate(t, a, alice, "heal")
	small := put(t, a, alice, "heal", "small.txt", "small")
	big := put(t, a, alice, "heal", "big.bin", strings.Repeat("B", inlineMax+5))
	waitFor(t, "b has them", func() bool { return b.holds(small.SHA256) && b.holds(big.SHA256) })

	b.forgetContent(small.SHA256)
	b.forgetContent(big.SHA256)
	// A copy of the small content that is the wrong size, and big content
	// that is the right size with the wrong bytes.
	r := fsck(t, b, FsckOptions{Repair: true})
	if r.Repaired[FsckMissingContent] != 2 || r.Unresolved() != 0 {
		t.Fatalf("repair %+v", r)
	}
	if got := read(t, b, alice, "heal", "small.txt"); got != "small" {
		t.Errorf("small %q", got)
	}
	clean(t, b, "after fetching")

	// Content that is the wrong bytes is found by a deep check, and replaced.
	b.db.Update(func(txn *badger.Txn) error { return txn.Set(shaKey('c', small.SHA256), []byte("smalX")) })
	if r = fsck(t, b, FsckOptions{}); r.Found[FsckCorruptContent] != 0 {
		t.Error("a shallow check read the content")
	}
	r = fsck(t, b, FsckOptions{Deep: true, Repair: true})
	if r.Found[FsckCorruptContent] != 1 || r.Repaired[FsckCorruptContent] != 1 || r.Unresolved() != 0 {
		t.Fatalf("deep repair %+v", r)
	}
	if got := read(t, b, alice, "heal", "small.txt"); got != "small" {
		t.Errorf("replaced content %q", got)
	}

	// Content of the wrong size is found without reading it.
	b.db.Update(func(txn *badger.Txn) error { return txn.Set(shaKey('c', small.SHA256), []byte("sm")) })
	if r = fsck(t, b, FsckOptions{Repair: true}); r.Found[FsckCorruptContent] != 1 || r.Unresolved() != 0 {
		t.Fatalf("size repair %+v", r)
	}
	clean(t, b, "after the size repair")
}

func TestFsckRepairsCountsAndIndexes(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "counts")
	a := put(t, s, alice, "counts", "a.txt", "alpha")
	put(t, s, alice, "counts", "b.txt", "alpha")
	put(t, s, alice, "counts", "c.txt", "gamma")
	put(t, s, alice, "counts", "d.txt", "delta")
	s.DeleteObject(alice, "counts", "d.txt")

	s.db.Update(func(txn *badger.Txn) error {
		if err := setRef(txn, a.SHA256, 7); err != nil { // wrong
			return err
		}
		if err := setRef(txn, contentSHA("nobody"), 1); err != nil { // no file
			return err
		}
		if err := txn.Set(shaKey('c', contentSHA("stray")), []byte("stray")); err != nil {
			return err
		}
		if err := txn.Set(shaKey('m', contentSHA("alpha")), []byte{1}); err != nil { // held, not wanted
			return err
		}
		// The deletion index loses an entry and gains one.
		o, _ := getObject(txn, bid(s, "counts"), "d.txt")
		if err := txn.Delete(tombKey(o.UpdatedAt, o.BucketId, o.Key)); err != nil {
			return err
		}
		return txn.Set(tombKey(1, o.BucketId, "never"), []byte{})
	})
	// And the statistics in memory drift.
	st := s.statsFor(bid(s, "counts"))
	st.mu.Lock()
	st.size += 100
	st.count++
	st.mu.Unlock()

	r := fsck(t, s, FsckOptions{})
	for _, kind := range []string{FsckRefCount, FsckOrphanRef, FsckOrphanInline, FsckStaleMarker, FsckStats} {
		if r.Found[kind] != 1 {
			t.Errorf("%s: found %d", kind, r.Found[kind])
		}
	}
	if r.Found[FsckTombstone] != 2 {
		t.Errorf("tombstone index: found %d", r.Found[FsckTombstone])
	}

	r = fsck(t, s, FsckOptions{Repair: true})
	if r.Unresolved() != 0 {
		t.Errorf("unresolved after repair: %+v", r)
	}
	clean(t, s, "after repair")
	if info, _ := s.GetBucket(alice, "counts"); info.Count != 3 || info.Size != int64(len("alpha")*2+len("gamma")) {
		t.Errorf("statistics %d files %d bytes", info.Count, info.Size)
	}
	// What was fixed holds: removing both users of shared content releases it.
	s.DeleteObject(alice, "counts", "a.txt")
	s.DeleteObject(alice, "counts", "b.txt")
	if s.holds(a.SHA256) {
		t.Error("content kept after its last file went")
	}
}

func TestFsckDeletedBucketAndOrphanFiles(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "dead")
	put(t, s, alice, "dead", "a", "1")
	s.sweepPaused.Store(true)
	s.DeleteBucket(alice, "dead", true)

	r := fsck(t, s, FsckOptions{})
	if r.Found[FsckDeadRecords] != 1 {
		t.Fatalf("found %v", r.Found)
	}
	// A record for a bucket this server has not heard of.
	s.Merge(nil, []*Object{{BucketId: NewBucketId(), Key: "early", SHA256: contentSHA("e"), UpdatedAt: 5}})
	// A content file nothing uses.
	orphan := strings.Repeat("c", 64)
	s.blobs.install(writeTempFile(t, s, "junk"), orphan)

	s.sweepPaused.Store(false)
	r = fsck(t, s, FsckOptions{Repair: true})
	if r.Found[FsckUnknownBucket] != 1 || r.Found[FsckOrphanFile] != 1 || r.Repaired[FsckOrphanFile] != 1 || r.Repaired[FsckDeadRecords] != 1 {
		t.Errorf("report %+v", r)
	}
	waitFor(t, "dead records swept", func() bool {
		page, _ := s.ObjectPage(bid(s, "dead"), "", 10, nil)
		return len(page) == 0 && !s.blobs.has(orphan)
	})
}

func writeTempFile(t *testing.T, s *Store, content string) string {
	t.Helper()
	tw, err := s.blobs.newTemp()
	if err != nil {
		t.Fatal(err)
	}
	tw.Write([]byte(content))
	if err := tw.finish(); err != nil {
		t.Fatal(err)
	}
	return tw.Path()
}

// Buckets of a deleted user that slipped through are removed; those of a
// user the database does not know are only reported.
func TestOwnersReconciled(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "alice--one")
	mustCreate(t, s, bob, "bob--two")
	mustCreate(t, s, carol, "carol--three")
	mustCreate(t, s, dave2, "dave--four")
	put(t, s, alice, "alice--one", "f", "x")
	s.SetGrant(bob, "bob--two", Grant{Type: GrantUser, Id: "alice", Access: GrantRead})

	states := map[string]OwnerState{"alice": OwnerDeleted, "bob": OwnerActive, "carol": OwnerMissing, "dave": OwnerUnknown}
	s.ownerState = func(id string) OwnerState { return states[id] }

	r := fsck(t, s, FsckOptions{})
	if r.Found[FsckOwnerDeleted] != 1 || r.Found[FsckOwnerMissing] != 1 || len(r.Notes) != 1 {
		t.Fatalf("report %+v", r)
	}
	if n := s.reconcileOwners(); n != 1 {
		t.Errorf("deleted %d buckets", n)
	}
	if _, err := s.GetBucket(admin, "alice--one"); err == nil {
		t.Error("deleted user's bucket kept")
	}
	for _, name := range []string{"bob--two", "carol--three", "dave--four"} {
		if _, err := s.GetBucket(admin, name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	info, _ := s.GetBucket(admin, "bob--two")
	if len(info.Grants) != 0 {
		t.Errorf("deleted user's grant kept: %v", info.Grants)
	}
	// A server with no user database leaves owners alone.
	s.ownerState = nil
	if n := s.reconcileOwners(); n != 0 {
		t.Errorf("deleted %d without a user database", n)
	}
}

var dave2 = &Principal{UserId: "dave", Username: "dave", CanOwn: true}
