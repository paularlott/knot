package filestore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/paularlott/gossip/hlc"
)

func contentSHA(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// Small content lives in the database, large content in a file, and both
// are shared between files with the same content and removed with the last.
func TestInlineAndFileContent(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir, nil)
	mustCreate(t, s, alice, "mix")

	small := strings.Repeat("s", 1000)
	large := strings.Repeat("L", inlineMax+1)
	exact := strings.Repeat("e", inlineMax)
	for name, content := range map[string]string{"small": small, "large": large, "exact": exact} {
		put(t, s, alice, "mix", name, content)
		put(t, s, alice, "mix", name+"-copy", content)
	}

	var inline = func(sha string) bool {
		found := false
		s.db.View(func(txn *badger.Txn) error { found, _ = hasKey(txn, shaKey('c', sha)); return nil })
		return found
	}
	if !inline(contentSHA(small)) || !inline(contentSHA(exact)) {
		t.Error("content up to the inline limit is not in the database")
	}
	if inline(contentSHA(large)) || !s.blobs.has(contentSHA(large)) {
		t.Error("larger content is not a file")
	}
	if s.blobs.has(contentSHA(small)) {
		t.Error("small content also written as a file")
	}

	s.Close()
	s = openStore(t, dir, nil)
	defer s.Close()
	for name, content := range map[string]string{"small": small, "large": large, "exact": exact} {
		if got := read(t, s, alice, "mix", name+"-copy"); got != content {
			t.Errorf("%s read back %d bytes after restart", name, len(got))
		}
	}

	// Content goes with its last file, not before.
	for _, content := range []string{small, large} {
		sha := contentSHA(content)
		k := map[string]string{small: "small", large: "large"}[content]
		s.DeleteObject(alice, "mix", k)
		if !s.holds(sha) {
			t.Errorf("%s content removed while another file uses it", k)
		}
		s.DeleteObject(alice, "mix", k+"-copy")
		if s.holds(sha) {
			t.Errorf("%s content kept after the last file went", k)
		}
	}

	// A file replaced by different content releases the old.
	put(t, s, alice, "mix", "exact", "now different")
	put(t, s, alice, "mix", "exact-copy", "also different")
	if s.holds(contentSHA(exact)) {
		t.Error("replaced content kept")
	}
}

// A store whose last shutdown was not clean recounts its statistics from the
// records, and they match what it held before.
func TestStatsRecountedAfterUncleanShutdown(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir, nil)
	mustCreate(t, s, alice, "count")
	for i := 0; i < 50; i++ {
		put(t, s, alice, "count", fmt.Sprintf("k%02d", i), strings.Repeat("x", i))
	}
	for i := 0; i < 50; i += 5 {
		s.DeleteObject(alice, "count", fmt.Sprintf("k%02d", i))
	}
	mustCreate(t, s, bob, "other")
	put(t, s, bob, "other", "a", "alpha")
	info, _ := s.GetBucket(alice, "count")
	cid := bid(s, "count")
	before, _ := s.SlotDigests(cid)
	digests, _ := s.Digests("", 0)

	s.damaged.Store(true) // closes without saving the statistics
	s.Close()

	s = openStore(t, dir, nil)
	defer s.Close()
	after, _ := s.GetBucket(alice, "count")
	if after.Size != info.Size || after.Count != info.Count || info.Count != 40 {
		t.Errorf("size/count %d/%d after recount, %d/%d before", after.Size, after.Count, info.Size, info.Count)
	}
	slots, _ := s.SlotDigests(cid)
	if fmt.Sprint(slots) != fmt.Sprint(before) {
		t.Error("slot digests differ after recount")
	}
	if d, _ := s.Digests("", 0); d[cid].Digest != digests[cid].Digest || d[cid].Count != digests[cid].Count {
		t.Errorf("digest after recount %+v, before %+v", d[cid], digests[cid])
	}
	if used := s.Usage("bob"); used.UsedBytes != 5 || used.Objects != 1 {
		t.Errorf("bob's usage %+v", used)
	}
}

// Tombstones are forgotten after the TTL, and a deleted bucket with them.
func TestTombstonesExpire(t *testing.T) {
	s := newStore(t)
	b := mustBucket(t, s, alice, "exp")
	put(t, s, alice, "exp", "keep", "1")
	put(t, s, alice, "exp", "old", "2")
	s.DeleteObject(alice, "exp", "old")

	// A tombstone from long ago, and a bucket deleted long ago.
	ancient := hlc.FromTime(time.Now().Add(-TombstoneTTL - time.Hour))
	s.Merge(nil, []*Object{{BucketId: b.Id, Key: "ancient", IsDeleted: true, UpdatedAt: ancient}})
	oldId := NewBucketId()
	s.Merge([]*Bucket{{Id: oldId, Name: "olddeleted", OwnerId: "bob", UpdatedAt: ancient}}, nil)
	s.Merge([]*Bucket{{Id: oldId, Name: "olddeleted", OwnerId: "bob", IsDeleted: true, UpdatedAt: ancient + 1}}, nil)

	if d, _ := s.Digests("", 0); d[b.Id].Count != 3 {
		t.Fatalf("records before expiry %d", d[b.Id].Count)
	}
	s.dropExpired()
	d, _ := s.Digests("", 0)
	if d[b.Id].Count != 2 {
		t.Errorf("records after expiry %d, want the file and the recent tombstone", d[b.Id].Count)
	}
	if _, ok := d[oldId]; ok {
		t.Error("long deleted bucket remembered")
	}
	if page, _ := s.ObjectPage(b.Id, "", 10, nil); len(page) != 2 {
		t.Errorf("page holds %d records", len(page))
	}
	if got := read(t, s, alice, "exp", "keep"); got != "1" {
		t.Errorf("kept file reads %q", got)
	}
}

// Uploads running at once cannot together pass the owner's quota.
func TestConcurrentUploadsRespectQuota(t *testing.T) {
	s := openStore(t, t.TempDir(), func(string) (int64, error) { return 1000, nil })
	defer s.Close()
	mustCreate(t, s, alice, "quota")

	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.PutObject(alice, "quota", fmt.Sprintf("k%02d", i), strings.NewReader(strings.Repeat("x", 100)), PutOptions{Size: 100})
			if err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			} else if !errors.Is(err, ErrQuotaExceeded) {
				t.Errorf("put: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if used := s.Usage("alice").UsedBytes; used > 1000 || ok != 10 {
		t.Errorf("%d uploads stored %d bytes against a limit of 1000", ok, used)
	}
}

// Many writers to the same key leave one consistent record, and the counts
// and reference counts add up.
func TestConcurrentWritesSameKey(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "race")
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			content := fmt.Sprintf("version %d", i)
			s.PutObject(alice, "race", "k", strings.NewReader(content), PutOptions{Size: int64(len(content))})
			if i%7 == 0 {
				s.DeleteObject(alice, "race", "k")
			}
		}(i)
	}
	wg.Wait()

	info, _ := s.GetBucket(alice, "race")
	_, err := s.HeadObject(alice, "race", "k")
	live := 0
	if err == nil {
		live = 1
	}
	if info.Count != live {
		t.Errorf("count %d with the file live=%v", info.Count, live == 1)
	}
	// Every content in the store is the one a record refers to.
	refs := 0
	s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.IteratorOptions{Prefix: []byte{'r'}})
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			refs++
		}
		return nil
	})
	if refs != live {
		t.Errorf("%d content references for %d live files", refs, live)
	}
}

// A bucket whose records were still being swept when the server stopped is
// finished at the next start.
func TestSweepResumesAfterRestart(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir, nil)
	mustCreate(t, s, alice, "resume")
	rid := bid(s, "resume")
	var shas []string
	for i := 0; i < 20; i++ {
		shas = append(shas, put(t, s, alice, "resume", fmt.Sprintf("k%d", i), fmt.Sprintf("content %d", i)).SHA256)
	}
	// Hold the sweeper back, then delete the bucket so its records stay behind.
	s.sweepPaused.Store(true)
	if err := s.DeleteBucket(alice, "resume", true); err != nil {
		t.Fatal(err)
	}
	if page, _ := s.ObjectPage(rid, "", 100, nil); len(page) != 20 {
		t.Fatalf("records swept early: %d", len(page))
	}
	s.Close()

	s = openStore(t, dir, nil)
	defer s.Close()
	waitFor(t, "records swept", func() bool {
		page, _ := s.ObjectPage(rid, "", 100, nil)
		return len(page) == 0
	})
	for _, sha := range shas {
		if s.holds(sha) {
			t.Fatal("content of the deleted bucket kept")
		}
	}
	d, _ := s.Digests("", 0)
	if d[rid].Count != 0 {
		t.Errorf("deleted bucket still counts %d records", d[rid].Count)
	}
}

// Listings work across the chunks records are written in, and a transfer
// moves nothing: the bucket keeps its id and its files stay where they are.
func TestLargeBucketTransferAndList(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "big")
	n := writeChunk*2 + 37
	ops := make([]objectOp, 0, n)
	id := bid(s, "big")
	for i := 0; i < n; i++ {
		content := fmt.Sprintf("c%d", i%50) // plenty of shared content
		ops = append(ops, objectOp{obj: &Object{
			BucketId: id, Key: fmt.Sprintf("d%d/f%05d", i%7, i), Size: int64(len(content)),
			SHA256: contentSHA(content), ETag: "e", UpdatedAt: hlc.Now(),
		}, content: contentInline, data: []byte(content)})
	}
	s.mu.RLock()
	got, err := s.applyObjects(ops)
	s.mu.RUnlock()
	if err != nil || len(got) != n {
		t.Fatalf("applied %d of %d: %v", len(got), n, err)
	}
	if info, _ := s.GetBucket(alice, "big"); info.Count != n {
		t.Fatalf("count %d", info.Count)
	}

	// Listed page by page, every file once.
	seen := map[string]bool{}
	after := ""
	for {
		res, err := s.ListObjects(alice, "big", "", "", after, 100)
		if err != nil {
			t.Fatal(err)
		}
		for _, o := range res.Objects {
			if seen[o.Key] {
				t.Fatalf("%s listed twice", o.Key)
			}
			seen[o.Key] = true
		}
		if !res.IsTruncated {
			break
		}
		after = res.Next
	}
	if len(seen) != n {
		t.Errorf("listed %d of %d files", len(seen), n)
	}
	// A folder listing sees each folder once.
	res, _ := s.ListObjects(alice, "big", "", "/", "", 1000)
	if len(res.Prefixes) != 7 || len(res.Objects) != 0 {
		t.Errorf("folders %v, files %d", res.Prefixes, len(res.Objects))
	}

	moved, err := s.TransferBucket(alice, "big", "carol", "carol", false)
	if err != nil {
		t.Fatal(err)
	}
	if moved.Id != id || moved.Name != "carol--big" {
		t.Errorf("transferred bucket %s %s", moved.Id, moved.Name)
	}
	if info, _ := s.GetBucket(carol, "carol--big"); info.Count != n {
		t.Errorf("new owner's bucket holds %d of %d", info.Count, n)
	}
	if _, err := s.GetBucket(alice, "big"); !errors.Is(err, ErrNoSuchBucket) {
		t.Errorf("old name still resolves: %v", err)
	}
	if page, _ := s.ObjectPage(id, "", 10, nil); len(page) != 10 {
		t.Error("records moved by a transfer")
	}
	if got := read(t, s, carol, "carol--big", "d0/f00000"); got != "c0" {
		t.Errorf("content after transfer %q", got)
	}
	if used := s.Usage("alice"); used.Objects != 0 || used.Buckets != 0 {
		t.Errorf("alice still holds %+v", used)
	}
	if used := s.Usage("carol"); used.Objects != n || used.Buckets != 1 {
		t.Errorf("carol holds %+v", used)
	}
}

// Records survive the encoding they are stored in, whatever they hold.
func TestRecordEncoding(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Nanosecond)
	o := &Object{
		BucketId: NewBucketId(), Key: "dir/файл-日本-🙂.txt", Size: 1 << 40, SHA256: contentSHA("x"), ETag: "etag",
		ContentType: "text/plain; charset=utf-8", Meta: map[string]string{"mtime": "1.5", "größe": "ü"},
		ModifiedAt: now, ModifiedBy: "user", SourceNode: "node", UpdatedAt: hlc.Now(), IsDeleted: false,
	}
	got, err := decodeObject(encodeRecord(o))
	if err != nil {
		t.Fatal(err)
	}
	if !got.ModifiedAt.Equal(o.ModifiedAt) {
		t.Errorf("time %v became %v", o.ModifiedAt, got.ModifiedAt)
	}
	got.ModifiedAt = o.ModifiedAt
	if fmt.Sprintf("%+v", got) != fmt.Sprintf("%+v", o) {
		t.Errorf("object changed:\n%+v\n%+v", o, got)
	}

	tomb := &Object{BucketId: NewBucketId(), Key: "k", UpdatedAt: hlc.Now(), IsDeleted: true}
	got, _ = decodeObject(encodeRecord(tomb))
	if !got.IsDeleted || got.Key != "k" || got.Meta != nil {
		t.Errorf("tombstone %+v", got)
	}

	b := &Bucket{Id: NewBucketId(), Name: "enc", OwnerId: "u", Grants: []Grant{{Type: GrantGroup, Id: "g", Access: GrantWrite}, {Type: GrantAll, Access: GrantRead}},
		CreatedAt: now, UpdatedAt: hlc.Now()}
	gb, err := decodeBucket(encodeRecord(b))
	if err != nil || len(gb.Grants) != 2 || gb.Grants[0] != b.Grants[0] || gb.Grants[1] != b.Grants[1] || !gb.CreatedAt.Equal(now) || gb.Id != b.Id {
		t.Errorf("bucket %+v %v", gb, err)
	}
	empty := &Bucket{Id: NewBucketId(), Name: "enc", OwnerId: "u", Grants: []Grant{}, CreatedAt: now, UpdatedAt: hlc.Now()}
	if gb, _ = decodeBucket(encodeRecord(empty)); len(gb.Grants) != 0 {
		t.Errorf("empty grants %+v", gb.Grants)
	}
}

// A bucket id is a lowercase UUID, and its keys are laid out as documented.
func TestBucketIdsAndKeys(t *testing.T) {
	id := NewBucketId()
	if !ValidBucketId(id) || ValidBucketId(strings.ToUpper(id)) || ValidBucketId("configs") || ValidBucketId("") {
		t.Errorf("id validity: %s", id)
	}
	if got := idFromRaw(rawId(id)); got != id {
		t.Errorf("id %s round-trips as %s", id, got)
	}
	k := objectKey(id, "a/b.txt")
	if k[0] != 'o' || len(k) != 1+16+len("a/b.txt") || string(k[17:]) != "a/b.txt" || string(k[1:17]) != string(rawId(id)) {
		t.Errorf("object key % x", k)
	}
	tk := tombKey(hlc.Timestamp(7), id, "x")
	if tk[0] != 't' || beUint64(tk[1:9]) != 7 || idFromRaw(tk[9:25]) != id || string(tk[25:]) != "x" {
		t.Errorf("tombstone key % x", tk)
	}
}

// Deleting a bucket frees its name, and a bucket made with it after is a
// different bucket that holds none of the old files.
func TestRecreatedBucketIsNew(t *testing.T) {
	s := newStore(t)
	first := mustBucket(t, s, alice, "again")
	put(t, s, alice, "again", "old.txt", "old")
	if err := s.DeleteBucket(alice, "again", true); err != nil {
		t.Fatal(err)
	}
	second := mustBucket(t, s, alice, "again")
	if first.Id == second.Id {
		t.Fatal("recreated bucket has the old id")
	}
	if _, err := s.HeadObject(alice, "again", "old.txt"); !errors.Is(err, ErrNoSuchKey) {
		t.Errorf("old file visible in the new bucket: %v", err)
	}
	// Records for the old bucket that arrive late are ignored.
	s.Merge(nil, []*Object{{BucketId: first.Id, Key: "late", SHA256: strings.Repeat("a", 64), UpdatedAt: hlc.Now()}})
	if _, err := s.HeadObject(alice, "again", "late"); !errors.Is(err, ErrNoSuchKey) {
		t.Errorf("late record of the old bucket visible: %v", err)
	}
	if info, _ := s.GetBucket(alice, "again"); info.Count != 0 || info.Id != second.Id {
		t.Errorf("new bucket %+v", info)
	}
}

// A bucket follows its owner's username, and nothing else about it changes.
func TestRenameOwnerBuckets(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir, nil)
	a := mustBucket(t, s, alice, "alice--docs")
	mustBucket(t, s, alice, "alice--logs")
	mustBucket(t, s, bob, "bob--docs")
	put(t, s, alice, "alice--docs", "f.txt", "hello")
	s.SetGrant(alice, "alice--docs", Grant{Type: GrantUser, Id: "bob", Access: GrantRead})

	renames, err := s.RenameOwnerBuckets("alice", "alicia")
	if err != nil || len(renames) != 2 {
		t.Fatalf("renamed %v: %v", renames, err)
	}
	if _, err := s.GetBucket(alice, "alice--docs"); !errors.Is(err, ErrNoSuchBucket) {
		t.Errorf("old name still resolves: %v", err)
	}
	alicia := &Principal{UserId: "alice", Username: "alicia", CanOwn: true, CanShare: true, CanTransfer: true}
	info, err := s.GetBucket(alicia, "alicia--docs")
	if err != nil || info.Id != a.Id || info.Count != 1 {
		t.Fatalf("renamed bucket %+v: %v", info, err)
	}
	if info.Id != bid(s, "alicia--docs") || bid(s, "alicia--logs") == "" {
		t.Error("name index not updated")
	}
	if got := read(t, s, bob, "alicia--docs", "f.txt"); got != "hello" {
		t.Errorf("shared reader after rename: %q", got)
	}
	if _, err := s.GetBucket(bob, "bob--docs"); err != nil {
		t.Errorf("another user's bucket: %v", err)
	}
	// Nothing to do when the name already follows.
	if renames, _ := s.RenameOwnerBuckets("alice", "alicia"); len(renames) != 0 {
		t.Errorf("renamed again: %v", renames)
	}

	s.Close()
	s = openStore(t, dir, nil)
	defer s.Close()
	if _, err := s.GetBucket(alicia, "alicia--docs"); err != nil {
		t.Errorf("after restart: %v", err)
	}
	if _, err := s.GetBucket(alice, "alice--docs"); !errors.Is(err, ErrNoSuchBucket) {
		t.Errorf("old name back after restart: %v", err)
	}

	// A name that is taken gets a suffix.
	mustBucket(t, s, carol, "carol--xyz")
	cid := bid(s, "carol--xyz")
	renames, err = s.RenameOwnerBuckets("carol", "alicia")
	if err != nil || len(renames) != 1 || renames[0].NewName != "alicia--xyz" {
		t.Fatalf("carol renamed %v: %v", renames, err)
	}
	if bid(s, "alicia--xyz") != cid {
		t.Error("renamed bucket not indexed")
	}
	mustBucket(t, s, bob, "bob--xyz")
	renames, _ = s.RenameOwnerBuckets("bob", "alicia")
	found := false
	for _, r := range renames {
		if r.OldName == "bob--xyz" {
			found = r.NewName == "alicia--xyz-2"
		}
	}
	if !found {
		t.Errorf("clashing name not suffixed: %v", renames)
	}
}

// Two buckets that claim one name, made on two servers at once, both live on:
// the one made later gets a suffix, on every server.
func TestNameClashRenamesTheLoser(t *testing.T) {
	a, b := newStore(t), newStore(t)
	first := mustBucket(t, a, alice, "alice--docs")
	time.Sleep(2 * time.Millisecond)
	second := mustBucket(t, b, alice, "alice--docs")
	put(t, b, alice, "alice--docs", "from-b.txt", "b data")
	if first.Id == second.Id {
		t.Fatal("same id")
	}

	// Each learns of the other's bucket.
	b.Merge([]*Bucket{first}, nil)
	a.Merge([]*Bucket{second}, nil)
	for i, st := range []*Store{a, b} {
		if st.byName["alice--docs"] != first.Id {
			t.Errorf("server %d: the earlier bucket does not keep the name", i)
		}
		if st.byName["alice--docs-2"] != second.Id {
			t.Errorf("server %d: the later bucket is not renamed: %v", i, st.byName)
		}
	}
	// The renamed bucket still holds its data.
	if got := read(t, b, alice, "alice--docs-2", "from-b.txt"); got != "b data" {
		t.Errorf("data of the renamed bucket: %q", got)
	}
	if info, _ := b.GetBucket(alice, "alice--docs"); info.Count != 0 || info.Id != first.Id {
		t.Errorf("winner %+v", info)
	}

	// The renamed record is newer than the one made, so everyone settles on it.
	renamed := b.buckets[second.Id].clone()
	a.Merge([]*Bucket{renamed}, nil)
	if a.byName["alice--docs-2"] != second.Id || len(a.byName) != 2 {
		t.Errorf("server a: %v", a.byName)
	}
}

// Empty content is fetched and stored like any other.
func TestFetchedEmptyContent(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "empty")
	id := bid(s, "empty")
	sha := contentSHA("")
	s.Merge(nil, []*Object{{BucketId: id, Key: "e", SHA256: sha, ETag: "x", UpdatedAt: hlc.Now()}})
	if s.holds(sha) {
		t.Fatal("held before it was fetched")
	}
	if err := s.storeContent(sha, 0, nil, "", false); err != nil {
		t.Fatal(err)
	}
	if !s.holds(sha) {
		t.Error("empty content not stored")
	}
	if got := read(t, s, alice, "empty", "e"); got != "" {
		t.Errorf("read %q", got)
	}
	if s.isMissing(sha) {
		t.Error("still marked missing")
	}
}

// With content lost, files and buckets can still be listed, deleted and
// copied, so a store with damaged content can be cleaned up and used again.
func TestMetadataSurvivesLostContent(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "lost")
	big := strings.Repeat("L", inlineMax+10)
	o := put(t, s, alice, "lost", "big.bin", big)
	small := put(t, s, alice, "lost", "small.txt", "small")
	put(t, s, alice, "lost", "keep.txt", "keep")

	// The content file and the inline content both go missing.
	s.forgetContent(o.SHA256)
	s.forgetContent(small.SHA256)

	if _, _, err := s.OpenObject(context.Background(), alice, "lost", "big.bin"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("reading lost content: %v", err)
	}
	if res, err := s.ListObjects(alice, "lost", "", "", "", 100); err != nil || len(res.Objects) != 3 {
		t.Errorf("listing: %v %v", res, err)
	}
	if _, err := s.CopyObject(alice, "lost", "big.bin", "lost", "copy.bin", nil); err != nil {
		t.Errorf("copying a record whose content is lost: %v", err)
	}
	if err := s.DeleteObject(alice, "lost", "big.bin"); err != nil {
		t.Errorf("deleting: %v", err)
	}
	if err := s.DeleteObject(alice, "lost", "copy.bin"); err != nil {
		t.Errorf("deleting the copy: %v", err)
	}
	if got := read(t, s, alice, "lost", "keep.txt"); got != "keep" {
		t.Errorf("other file: %q", got)
	}
	if err := s.DeleteBucket(alice, "lost", true); err != nil {
		t.Fatalf("deleting the bucket: %v", err)
	}
	waitFor(t, "records swept", func() bool {
		page, _ := s.ObjectPage(bid(s, "lost"), "", 10, nil)
		return len(page) == 0
	})
	if u := s.Usage("alice"); u.Objects != 0 || u.UsedBytes != 0 || u.Buckets != 0 {
		t.Errorf("usage after cleanup %+v", u)
	}
	mustCreate(t, s, alice, "lost")
	put(t, s, alice, "lost", "fresh.txt", "fresh")
}

// A server catching up on many small files fetches them many to a
// connection, not a connection each, and gets exactly what was written.
func TestSmallContentFetchedInBatches(t *testing.T) {
	cl, stores := newCluster(t, 2)
	a, b := stores[0], stores[1]
	mustCreate(t, a, alice, "batch")

	// Records reach b while it cannot fetch, so they pile up as missing.
	cl.setDown("node1", true)
	const n = 300
	want := map[string]string{}
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("f%03d", i)
		content := fmt.Sprintf("content of file %d ", i) + strings.Repeat("x", i)
		want[key] = content
		put(t, a, alice, "batch", key, content)
	}
	put(t, a, alice, "batch", "empty", "")
	want["empty"] = ""
	da, _ := a.Digests("", 0)
	b.Merge(digestBuckets(da), nil)
	page, _ := a.ObjectPage(bid(a, "batch"), "", 1000, nil)
	b.Merge(nil, page)
	cl.setDown("node1", false)

	waitFor(t, "b fetches every file", func() bool {
		b.missingMu.Lock()
		defer b.missingMu.Unlock()
		return len(b.missing) == 0
	})
	for key, content := range want {
		if got := read(t, b, alice, "batch", key); got != content {
			t.Fatalf("%s reads %q, want %q", key, got, content)
		}
	}
	cl.mu.Lock()
	batches, singles := cl.batches, cl.opens
	cl.mu.Unlock()
	if batches == 0 || batches > n/8 || singles > 20 {
		t.Errorf("%d files fetched in %d batches and %d single fetches", n+1, batches, singles)
	}
	clean(t, b, "after catching up")
}

// Content a batch cannot supply, because the server it asked lacks it, is
// fetched the usual way from another.
func TestBatchFallsBackToOtherServers(t *testing.T) {
	cl, stores := newCluster(t, 3)
	a, b, c := stores[0], stores[1], stores[2]
	mustCreate(t, a, alice, "fallback")
	cl.setDown("node2", true)
	o := put(t, a, alice, "fallback", "f", "only on a")
	cl.setDown("node2", false)
	da, _ := a.Digests("", 0)
	b.Merge(digestBuckets(da), nil)
	page, _ := a.ObjectPage(bid(a, "fallback"), "", 10, nil)

	// The record says the content was written on c, which never had it.
	for _, p := range page {
		p.SourceNode = c.nodeId
	}
	b.forgetContent(o.SHA256)
	b.Merge(nil, page)
	waitFor(t, "b gets the content from a", func() bool { return b.holds(o.SHA256) })
}

// A write that loses to a newer record from another server, at the same
// moment, still succeeds: it happened and was replaced, which is what last
// writer wins means. It is not an error to the client, and not gossiped.
func TestWriteOlderThanHeldRecordSucceeds(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "race")
	id := bid(s, "race")

	future := hlc.Now() + (1 << 40)
	s.Merge(nil, []*Object{{BucketId: id, Key: "k", Size: 3, SHA256: contentSHA("new"), ETag: "e", UpdatedAt: future}})

	o, err := s.PutObject(alice, "race", "k", strings.NewReader("old"), PutOptions{Size: 3})
	if err != nil || o == nil || o.Size != 3 {
		t.Fatalf("a write that lost: %v %v", o, err)
	}
	if head, err := s.HeadObject(alice, "race", "k"); err != nil || head.SHA256 != contentSHA("new") {
		t.Errorf("the newer record was replaced: %+v %v", head, err)
	}
	if _, err := s.CopyObject(alice, "race", "k", "race", "k", nil); err != nil {
		t.Errorf("a copy over a newer record: %v", err)
	}
	if err := s.DeleteObject(alice, "race", "k"); err != nil {
		t.Errorf("a delete older than the record: %v", err)
	}
	if head, _ := s.HeadObject(alice, "race", "k"); head == nil || head.SHA256 != contentSHA("new") {
		t.Error("the delete removed a newer record")
	}
	// A conditional write is still refused when its condition fails.
	if _, err := s.PutObject(alice, "race", "k", strings.NewReader("x"), PutOptions{Size: 1, IfNoneMatch: true}); !errors.Is(err, ErrPrecondition) {
		t.Errorf("if-none-match over a record: %v", err)
	}
}
