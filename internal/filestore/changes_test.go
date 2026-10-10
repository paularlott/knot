package filestore

import (
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	badger "github.com/dgraph-io/badger/v4"
)

func changeKeys(cl *ChangeList) []string {
	var out []string
	for _, o := range cl.Changes {
		k := o.Key
		if o.IsDeleted {
			k = "-" + k
		}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func listChanges(t *testing.T, s *Store, p *Principal, bucket, prefix, cursor string, max int) *ChangeList {
	t.Helper()
	cl, err := s.ListChanges(p, bucket, prefix, cursor, max)
	if err != nil {
		t.Fatalf("changes: %v", err)
	}
	return cl
}

func TestChangesFeed(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "feed")
	put(t, s, alice, "feed", "a.txt", "a")
	put(t, s, alice, "feed", "dir/b.txt", "b")
	put(t, s, alice, "feed", "gone.txt", "gone")
	s.DeleteObject(alice, "feed", "gone.txt")

	// Without a cursor: every file, no deletions.
	first := listChanges(t, s, alice, "feed", "", "", 0)
	if got := strings.Join(changeKeys(first), ","); got != "a.txt,dir/b.txt" || first.More || first.Reset || first.Cursor == "" {
		t.Fatalf("first %q more=%v reset=%v cursor=%q", got, first.More, first.Reset, first.Cursor)
	}

	// A cursor for now returns nothing, and follows on like any other.
	now := listChanges(t, s, alice, "feed", "", CursorNow, 0)
	if len(now.Changes) != 0 || now.Cursor != first.Cursor {
		t.Fatalf("now %v cursor %q, want %q", changeKeys(now), now.Cursor, first.Cursor)
	}

	// Nothing since.
	again := listChanges(t, s, alice, "feed", "", first.Cursor, 0)
	if len(again.Changes) != 0 || again.Cursor != first.Cursor {
		t.Fatalf("again %v cursor %q", changeKeys(again), again.Cursor)
	}

	// A change, a new file and a deletion, each once as it is now.
	put(t, s, alice, "feed", "a.txt", "a2")
	put(t, s, alice, "feed", "a.txt", "a3")
	put(t, s, alice, "feed", "dir/c.txt", "c")
	s.DeleteObject(alice, "feed", "dir/b.txt")
	next := listChanges(t, s, alice, "feed", "", first.Cursor, 0)
	if got := strings.Join(changeKeys(next), ","); got != "-dir/b.txt,a.txt,dir/c.txt" {
		t.Fatalf("next %q", got)
	}
	for _, o := range next.Changes {
		if o.Key == "a.txt" && o.SHA256 != contentSHA("a3") {
			t.Errorf("a.txt is not the latest version")
		}
	}

	// A prefix narrows the feed.
	sub := listChanges(t, s, alice, "feed", "dir/", first.Cursor, 0)
	if got := strings.Join(changeKeys(sub), ","); got != "-dir/b.txt,dir/c.txt" {
		t.Fatalf("prefix %q", got)
	}

	// Paging: one at a time, until no more.
	var paged []string
	cur := first.Cursor
	for i := 0; i < 10; i++ {
		page := listChanges(t, s, alice, "feed", "", cur, 1)
		paged = append(paged, changeKeys(page)...)
		cur = page.Cursor
		if !page.More {
			break
		}
	}
	sort.Strings(paged)
	if got := strings.Join(paged, ","); got != "-dir/b.txt,a.txt,dir/c.txt" {
		t.Fatalf("paged %q", got)
	}

	// Another bucket's changes are its own.
	mustCreate(t, s, alice, "other")
	put(t, s, alice, "other", "x", "x")
	if cl := listChanges(t, s, alice, "feed", "", cur, 0); len(cl.Changes) != 0 {
		t.Errorf("other bucket leaked %v", changeKeys(cl))
	}

	// The feed needs read access.
	if _, err := s.ListChanges(bob, "feed", "", "", 0); !errors.Is(err, ErrNoSuchBucket) && !errors.Is(err, ErrAccessDenied) {
		t.Errorf("bob read the feed: %v", err)
	}
}

func TestChangesCursorOfAnotherInstance(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "inst")
	if cl := listChanges(t, s, alice, "inst", "", "0123456789abcdef.5", 0); !cl.Reset {
		t.Errorf("foreign cursor not reset")
	}
	if _, err := s.ListChanges(alice, "inst", "", "garbage", 0); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("garbage cursor: %v", err)
	}
	if _, err := s.ListChanges(alice, "inst", "", s.changes.instance+".99999", 0); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("cursor ahead: %v", err)
	}
}

func TestChangesSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir, nil)
	mustCreate(t, s, alice, "keep")
	put(t, s, alice, "keep", "a", "a")
	cursor := listChanges(t, s, alice, "keep", "", "", 0).Cursor
	s.Close()

	// A clean restart keeps cursors valid.
	s = openStore(t, dir, nil)
	put(t, s, alice, "keep", "b", "b")
	cl := listChanges(t, s, alice, "keep", "", cursor, 0)
	if cl.Reset || strings.Join(changeKeys(cl), ",") != "b" {
		t.Fatalf("after restart reset=%v %v", cl.Reset, changeKeys(cl))
	}
	cursor = cl.Cursor
	// An unclean one rebuilds the index: old cursors start again.
	s.damaged.Store(true) // skips the clean-close marker
	s.Close()
	s = openStore(t, dir, nil)
	t.Cleanup(s.Close)
	if cl := listChanges(t, s, alice, "keep", "", cursor, 0); !cl.Reset {
		t.Fatalf("unclean restart kept the cursor")
	}
	if got := strings.Join(changeKeys(listChanges(t, s, alice, "keep", "", "", 0)), ","); got != "a,b" {
		t.Fatalf("rebuilt %q", got)
	}
}

func TestChangesForgottenDeletionsReset(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "old")
	put(t, s, alice, "old", "a", "a")
	cursor := listChanges(t, s, alice, "old", "", "", 0).Cursor
	s.DeleteObject(alice, "old", "a")

	// Forget the tombstone, as the TTL would: a client that never saw the
	// delete must start again.
	s.mu.RLock()
	var recs []*Object
	b := s.bucketByNameLocked("old")
	if o := func() *Object {
		cl, _ := s.ListChanges(alice, "old", "", cursor, 0)
		if cl == nil || len(cl.Changes) != 1 {
			return nil
		}
		return cl.Changes[0]
	}(); o != nil {
		recs = append(recs, o)
	}
	s.mu.RUnlock()
	if len(recs) != 1 || b == nil {
		t.Fatalf("tombstone not in the feed")
	}
	s.mu.RLock()
	err := s.dropRecords(recs)
	s.mu.RUnlock()
	if err != nil {
		t.Fatal(err)
	}
	if cl := listChanges(t, s, alice, "old", "", cursor, 0); !cl.Reset {
		t.Errorf("forgotten delete not reset")
	}
}

func withGrace(t *testing.T, d time.Duration) {
	old := contentGrace
	contentGrace = d
	t.Cleanup(func() { contentGrace = old })
}

func TestReuseHeldContent(t *testing.T) {
	withGrace(t, time.Hour)
	s := newStore(t)
	mustCreate(t, s, alice, "ren")
	big := strings.Repeat("R", inlineMax+10)
	small := put(t, s, alice, "ren", "small.txt", "small")
	large := put(t, s, alice, "ren", "large.bin", big)

	// A rename: deleted, then written again elsewhere without the content.
	s.DeleteObject(alice, "ren", "small.txt")
	s.DeleteObject(alice, "ren", "large.bin")
	if !s.holds(small.SHA256) || !s.holds(large.SHA256) {
		t.Fatal("content not held after delete")
	}
	o, err := s.ReuseObject(alice, "ren", "moved/small.txt", small.SHA256, PutOptions{})
	if err != nil {
		t.Fatalf("reuse small: %v", err)
	}
	if o.Size != small.Size || o.ETag != small.ETag {
		t.Errorf("reused record %+v", o)
	}
	if _, err := s.ReuseObject(alice, "ren", "moved/large.bin", large.SHA256, PutOptions{}); err != nil {
		t.Fatalf("reuse large: %v", err)
	}
	if got := read(t, s, alice, "ren", "moved/small.txt"); got != "small" {
		t.Errorf("read %q", got)
	}
	if got := read(t, s, alice, "ren", "moved/large.bin"); got != big {
		t.Errorf("large content differs")
	}

	// Not held in another bucket, nor content never stored.
	mustCreate(t, s, bob, "elsewhere")
	if _, err := s.ReuseObject(bob, "elsewhere", "steal", small.SHA256, PutOptions{}); !errors.Is(err, ErrContentNotHeld) {
		t.Errorf("reuse in another bucket: %v", err)
	}
	if _, err := s.ReuseObject(alice, "ren", "x", contentSHA("never"), PutOptions{}); !errors.Is(err, ErrContentNotHeld) {
		t.Errorf("reuse of unknown content: %v", err)
	}
	// Preconditions apply.
	if _, err := s.ReuseObject(alice, "ren", "moved/small.txt", small.SHA256, PutOptions{IfNoneMatch: true}); !errors.Is(err, ErrPrecondition) {
		t.Errorf("if-none-match: %v", err)
	}

	// Overwrites hold the old version too.
	v1 := put(t, s, alice, "ren", "doc", "version one")
	put(t, s, alice, "ren", "doc", "version two")
	if _, err := s.ReuseObject(alice, "ren", "doc", v1.SHA256, PutOptions{}); err != nil {
		t.Errorf("reuse after overwrite: %v", err)
	}

	// fsck agrees with the hold references.
	r := fsck(t, s, FsckOptions{})
	if len(r.Found) != 0 {
		t.Errorf("fsck found %v", r.Found)
	}
}

func TestHoldsExpire(t *testing.T) {
	withGrace(t, time.Hour)
	s := newStore(t)
	mustCreate(t, s, alice, "exp")
	big := put(t, s, alice, "exp", "big", strings.Repeat("E", inlineMax+10))
	small := put(t, s, alice, "exp", "small", "e")
	s.DeleteObject(alice, "exp", "big")
	s.DeleteObject(alice, "exp", "small")

	// Too close to expiry to claim.
	contentGrace = reuseMargin / 2
	put(t, s, alice, "exp", "tmp", "t")
	tmp := contentSHA("t")
	s.DeleteObject(alice, "exp", "tmp")
	if _, err := s.ReuseObject(alice, "exp", "tmp2", tmp, PutOptions{}); !errors.Is(err, ErrContentNotHeld) {
		t.Errorf("claimed content about to expire: %v", err)
	}

	// Expired holds release the content.
	contentGrace = -1
	s.db.DropPrefix([]byte{'e'})
	for _, sha := range []string{big.SHA256, small.SHA256, tmp} {
		s.db.Update(func(txn *badger.Txn) error { return setHold(txn, s.bucketByNameLocked("exp").Id, sha, &hold{expiry: 1, size: 1}) })
	}
	s.expireHolds()
	for _, sha := range []string{big.SHA256, small.SHA256, tmp} {
		if s.holds(sha) || s.refCount(sha) != 0 {
			t.Errorf("content %s kept after its hold expired", sha[:8])
		}
	}
	if r := fsck(t, s, FsckOptions{}); len(r.Found) != 0 {
		t.Errorf("fsck found %v", r.Found)
	}
}

func TestReuseReplicates(t *testing.T) {
	withGrace(t, time.Hour)
	_, stores := newCluster(t, 2)
	a, b := stores[0], stores[1]
	mustCreate(t, a, alice, "rrr")
	content := strings.Repeat("Z", inlineMax+100)
	o := put(t, a, alice, "rrr", "old", content)
	waitFor(t, "content on b", func() bool { return b.holds(o.SHA256) })

	a.DeleteObject(alice, "rrr", "old")
	waitFor(t, "delete on b", func() bool { _, err := b.HeadObject(alice, "rrr", "old"); return err != nil })
	if !b.holds(o.SHA256) {
		t.Fatal("b let the content go")
	}
	if _, err := a.ReuseObject(alice, "rrr", "new", o.SHA256, PutOptions{}); err != nil {
		t.Fatalf("reuse: %v", err)
	}
	waitFor(t, "reused file on b", func() bool { _, err := b.HeadObject(alice, "rrr", "new"); return err == nil })
	if got := read(t, b, alice, "rrr", "new"); got != content {
		t.Errorf("b read the wrong content")
	}
	// b's change feed reports the rename as it happened there.
	cl := listChanges(t, b, alice, "rrr", "", "", 0)
	if got := strings.Join(changeKeys(cl), ","); got != "new" {
		t.Errorf("b feed %q", got)
	}
}

func TestCopyObjectIf(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "cpy")
	put(t, s, alice, "cpy", "src", "content")
	dst := put(t, s, alice, "cpy", "dst", "old")
	if _, err := s.CopyObjectIf(alice, "cpy", "src", "cpy", "dst", nil, PutOptions{IfNoneMatch: true}); !errors.Is(err, ErrPrecondition) {
		t.Errorf("if-none-match over a file: %v", err)
	}
	if _, err := s.CopyObjectIf(alice, "cpy", "src", "cpy", "dst", nil, PutOptions{IfMatch: "nope"}); !errors.Is(err, ErrPrecondition) {
		t.Errorf("if-match wrong etag: %v", err)
	}
	if _, err := s.CopyObjectIf(alice, "cpy", "src", "cpy", "dst", nil, PutOptions{IfMatch: dst.ETag}); err != nil {
		t.Errorf("if-match right etag: %v", err)
	}
	if _, err := s.CopyObjectIf(alice, "cpy", "src", "cpy", "new", nil, PutOptions{IfNoneMatch: true}); err != nil {
		t.Errorf("if-none-match to a new name: %v", err)
	}
	if got := read(t, s, alice, "cpy", "new"); got != "content" {
		t.Errorf("copied %q", got)
	}
}

func TestMaySee(t *testing.T) {
	s := newStore(t)
	carol := &Principal{UserId: "carol", Username: "carol", Groups: []string{"devs"}}
	stranger := &Principal{UserId: "dave", Username: "dave"}
	mustCreate(t, s, alice, "aud")
	b, _ := s.GetBucket(alice, "aud")
	id := b.Id

	if !s.MaySee(alice, id) || s.MaySee(bob, id) || s.MaySee(stranger, id) {
		t.Fatal("owner only")
	}
	if _, err := s.SetGrant(alice, "aud", Grant{Type: GrantUser, Id: "bob", Access: GrantRead}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetGrant(alice, "aud", Grant{Type: GrantGroup, Id: "devs", Access: GrantWrite}); err != nil {
		t.Fatal(err)
	}
	if !s.MaySee(bob, id) || !s.MaySee(carol, id) || s.MaySee(stranger, id) {
		t.Fatal("shares not seen")
	}

	// Unshared: bob still hears of it for a while, so his view drops it.
	if _, err := s.RemoveGrant(alice, "aud", GrantUser, "bob"); err != nil {
		t.Fatal(err)
	}
	if !s.MaySee(bob, id) {
		t.Error("bob not told of the unshare")
	}
	s.audience.mu.Lock()
	for _, list := range s.audience.prev {
		for i := range list {
			list[i].at = list[i].at.Add(-2 * audienceKeep)
		}
	}
	s.audience.mu.Unlock()
	if s.MaySee(bob, id) {
		t.Error("bob still told after the unshare settled")
	}

	// Deleted: those who could see it are told.
	if err := s.DeleteBucket(alice, "aud", true); err != nil {
		t.Fatal(err)
	}
	if !s.MaySee(alice, id) || !s.MaySee(carol, id) || s.MaySee(stranger, id) {
		t.Error("deletion audience wrong")
	}
}

func TestDeleteObjectIf(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "del")
	o := put(t, s, alice, "del", "f", "v1")
	put(t, s, alice, "del", "f", "v2")
	if err := s.DeleteObjectIf(alice, "del", "f", o.ETag); !errors.Is(err, ErrPrecondition) {
		t.Fatalf("deleted a version it did not see: %v", err)
	}
	if got := read(t, s, alice, "del", "f"); got != "v2" {
		t.Fatalf("file %q", got)
	}
	now, _ := s.HeadObject(alice, "del", "f")
	if err := s.DeleteObjectIf(alice, "del", "f", `"`+now.ETag+`"`); err != nil {
		t.Fatalf("delete of the current version: %v", err)
	}
	if _, err := s.HeadObject(alice, "del", "f"); !errors.Is(err, ErrNoSuchKey) {
		t.Errorf("still there: %v", err)
	}
}
