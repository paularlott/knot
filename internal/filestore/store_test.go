package filestore

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	badger "github.com/dgraph-io/badger/v4"
	"github.com/paularlott/gossip/hlc"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	alice = &Principal{UserId: "alice", Username: "alice", Groups: []string{"devs"}, CanOwn: true, CanShare: true, CanTransfer: true}
	bob   = &Principal{UserId: "bob", Username: "bob", Groups: []string{"ops"}, CanOwn: true, CanShare: true, CanTransfer: true}
	carol = &Principal{UserId: "carol", Username: "carol", Groups: []string{"devs"}, CanOwn: true, CanShare: true, CanTransfer: true}
	dave  = &Principal{UserId: "dave", Username: "dave", Groups: []string{"devs"}} // no file storage permission
	admin = &Principal{UserId: "admin", Username: "admin", IsAdmin: true}
)

func openStore(t *testing.T, dir string, quota QuotaFunc) *Store {
	t.Helper()
	s, err := Open(Config{Dir: dir, NodeId: "node-" + dir[len(dir)-3:], Quota: quota})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return s
}

func newStore(t *testing.T) *Store {
	t.Helper()
	s := openStore(t, t.TempDir(), nil)
	t.Cleanup(s.Close)
	return s
}

func put(t *testing.T, s *Store, p *Principal, bucket, key, content string) *Object {
	t.Helper()
	o, err := s.PutObject(p, bucket, key, strings.NewReader(content), PutOptions{Size: int64(len(content))})
	if err != nil {
		t.Fatalf("put %s/%s: %v", bucket, key, err)
	}
	return o
}

func read(t *testing.T, s *Store, p *Principal, bucket, key string) string {
	t.Helper()
	_, f, err := s.OpenObject(context.Background(), p, bucket, key)
	if err != nil {
		t.Fatalf("open %s/%s: %v", bucket, key, err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	return string(data)
}

func mustCreate(t *testing.T, s *Store, p *Principal, name string) {
	t.Helper()
	if _, err := s.CreateBucket(p, name); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
}

func TestBucketNames(t *testing.T) {
	for name, want := range map[string]bool{
		"abc": true, "my-bucket-1": true, "ab": false, "Abc": false, "-abc": false, "abc-": false,
		"a.b.c": true, "a--b": true, "a--b--c": false, "a..b": false, strings.Repeat("a", 63): true, strings.Repeat("a", 64): false,
	} {
		if got := ValidBucketName(name); got != want {
			t.Errorf("ValidBucketName(%q) = %v, want %v", name, got, want)
		}
	}
	for key, want := range map[string]bool{"a": true, "a/b/c.txt": true, "": false, "a/../b": false, "./a": false} {
		if got := ValidKey(key); got != want {
			t.Errorf("ValidKey(%q) = %v, want %v", key, got, want)
		}
	}
}

func TestPutGetDelete(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "configs")

	content := "hello world"
	o := put(t, s, alice, "configs", "app/settings.toml", content)

	md5sum := md5.Sum([]byte(content))
	shasum := sha256.Sum256([]byte(content))
	if o.ETag != hex.EncodeToString(md5sum[:]) {
		t.Errorf("etag %s is not the md5", o.ETag)
	}
	if o.SHA256 != hex.EncodeToString(shasum[:]) || o.Size != int64(len(content)) {
		t.Errorf("unexpected object %+v", o)
	}
	if got := read(t, s, alice, "configs", "app/settings.toml"); got != content {
		t.Errorf("read %q", got)
	}

	// Overwrite replaces content and drops the old blob.
	put(t, s, alice, "configs", "app/settings.toml", "v2")
	if got := read(t, s, alice, "configs", "app/settings.toml"); got != "v2" {
		t.Errorf("read after overwrite %q", got)
	}
	if s.holds(o.SHA256) {
		t.Error("old content still stored after overwrite")
	}

	if err := s.DeleteObject(alice, "configs", "app/settings.toml"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.HeadObject(alice, "configs", "app/settings.toml"); !errors.Is(err, ErrNoSuchKey) {
		t.Errorf("head after delete: %v", err)
	}
	if err := s.DeleteObject(alice, "configs", "app/settings.toml"); !errors.Is(err, ErrNoSuchKey) {
		t.Errorf("second delete: %v", err)
	}
	if u := s.Usage("alice"); u.UsedBytes != 0 || u.Objects != 0 {
		t.Errorf("usage after delete %+v", u)
	}
}

func TestChecksums(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "sums")

	data := []byte("checked")
	sum := sha256.Sum256(data)
	if _, err := s.PutObject(alice, "sums", "ok", bytes.NewReader(data), PutOptions{Size: -1, SHA256: hex.EncodeToString(sum[:])}); err != nil {
		t.Errorf("matching sha256: %v", err)
	}
	if _, err := s.PutObject(alice, "sums", "bad", bytes.NewReader(data), PutOptions{Size: -1, SHA256: strings.Repeat("0", 64)}); !errors.Is(err, ErrContentMismatch) {
		t.Errorf("wrong sha256: %v", err)
	}
	if _, err := s.PutObject(alice, "sums", "bad", bytes.NewReader(data), PutOptions{Size: -1, MD5: make([]byte, 16)}); !errors.Is(err, ErrContentMismatch) {
		t.Errorf("wrong md5: %v", err)
	}
	if _, err := s.PutObject(alice, "sums", "short", bytes.NewReader(data), PutOptions{Size: 100}); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("short body: %v", err)
	}
	if _, err := s.HeadObject(alice, "sums", "bad"); !errors.Is(err, ErrNoSuchKey) {
		t.Errorf("rejected upload was stored: %v", err)
	}
}

func TestAccessControl(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "shared")
	put(t, s, alice, "shared", "f", "x")

	// Others cannot see a private bucket at all.
	if _, err := s.HeadObject(bob, "shared", "f"); !errors.Is(err, ErrNoSuchBucket) {
		t.Errorf("bob head private: %v", err)
	}
	if len(s.ListBuckets(bob, false)) != 0 {
		t.Error("bob lists alice's private bucket")
	}

	// Only the owner may manage it.
	if _, err := s.SetGrant(bob, "shared", Grant{Type: GrantUser, Id: "bob", Access: GrantWrite}); !errors.Is(err, ErrNoSuchBucket) {
		t.Errorf("bob granted himself: %v", err)
	}

	// Read grant to bob.
	if _, err := s.SetGrant(alice, "shared", Grant{Type: GrantUser, Id: "bob", Access: GrantRead}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, s, bob, "shared", "f"); got != "x" {
		t.Errorf("bob read %q", got)
	}
	if _, err := s.PutObject(bob, "shared", "g", strings.NewReader("y"), PutOptions{Size: 1}); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("bob write with read grant: %v", err)
	}
	if err := s.DeleteObject(bob, "shared", "f"); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("bob delete with read grant: %v", err)
	}
	if _, err := s.SetGrant(bob, "shared", Grant{Type: GrantAll, Access: GrantWrite}); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("reader changed grants: %v", err)
	}
	if err := s.DeleteBucket(bob, "shared", true); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("reader deleted bucket: %v", err)
	}

	// Upgrading the grant replaces it.
	b, err := s.SetGrant(alice, "shared", Grant{Type: GrantUser, Id: "bob", Access: GrantWrite})
	if err != nil || len(b.Grants) != 1 {
		t.Fatalf("upgrade grant: %v %+v", err, b)
	}
	put(t, s, bob, "shared", "g", "y")

	// Group grant reaches carol through "devs".
	if _, err := s.HeadObject(carol, "shared", "f"); !errors.Is(err, ErrNoSuchBucket) {
		t.Errorf("carol before group grant: %v", err)
	}
	s.SetGrant(alice, "shared", Grant{Type: GrantGroup, Id: "devs", Access: GrantRead})
	if got := read(t, s, carol, "shared", "g"); got != "y" {
		t.Errorf("carol read %q", got)
	}

	// Unsharing removes access.
	if _, err := s.RemoveGrant(alice, "shared", GrantUser, "bob"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.HeadObject(bob, "shared", "f"); !errors.Is(err, ErrNoSuchBucket) {
		t.Errorf("bob after unshare: %v", err)
	}

	// All users grant.
	s.SetGrant(alice, "shared", Grant{Type: GrantAll, Id: "ignored", Access: GrantRead})
	if got := read(t, s, bob, "shared", "f"); got != "x" {
		t.Errorf("bob read via all grant %q", got)
	}
	if info, _ := s.GetBucket(bob, "shared"); info.Access != AccessRead {
		t.Errorf("bob access %d", info.Access)
	}

	// Administrators reach everything; listing all needs the flag.
	if got := read(t, s, admin, "shared", "f"); got != "x" {
		t.Errorf("admin read %q", got)
	}
	if len(s.ListBuckets(admin, false)) != 1 { // via the all-users grant
		t.Error("admin should see shared bucket")
	}
	mustCreate(t, s, bob, "bobs")
	if n := len(s.ListBuckets(admin, false)); n != 1 {
		t.Errorf("admin without all sees %d buckets", n)
	}
	if n := len(s.ListBuckets(admin, true)); n != 2 {
		t.Errorf("admin with all sees %d buckets", n)
	}
	if n := len(s.ListBuckets(alice, true)); n != 1 {
		t.Errorf("non-admin all sees %d buckets", n)
	}
}

func TestBucketLifecycle(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "life")
	if _, err := s.CreateBucket(bob, "life"); !errors.Is(err, ErrBucketExists) {
		t.Errorf("duplicate create: %v", err)
	}
	if _, err := s.CreateBucket(alice, "Bad_Name"); !errors.Is(err, ErrInvalidName) {
		t.Errorf("bad name: %v", err)
	}

	o := put(t, s, alice, "life", "a", "content-a")
	if err := s.DeleteBucket(alice, "life", false); !errors.Is(err, ErrBucketNotEmpty) {
		t.Errorf("delete non-empty: %v", err)
	}
	if err := s.DeleteBucket(alice, "life", true); err != nil {
		t.Fatal(err)
	}
	// The bucket's records are swept shortly after it is deleted.
	waitFor(t, "content removed after force delete", func() bool { return !s.holds(o.SHA256) })
	if _, err := s.GetBucket(alice, "life"); !errors.Is(err, ErrNoSuchBucket) {
		t.Errorf("get deleted bucket: %v", err)
	}

	// A new bucket of the same name starts empty.
	mustCreate(t, s, bob, "life")
	res, err := s.ListObjects(bob, "life", "", "", "", 0)
	if err != nil || len(res.Objects) != 0 {
		t.Errorf("recreated bucket lists %v %v", res, err)
	}
	if _, err := s.HeadObject(bob, "life", "a"); !errors.Is(err, ErrNoSuchKey) {
		t.Errorf("old object visible in new generation: %v", err)
	}
}

// bid is the id of the live bucket with a name.
func bid(s *Store, name string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.byName[name]
}

func mustBucket(t *testing.T, s *Store, p *Principal, name string) *Bucket {
	t.Helper()
	b, err := s.CreateBucket(p, name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestQuota(t *testing.T) {
	limits := map[string]int64{"alice": 10}
	s := openStore(t, t.TempDir(), func(userId string) (int64, error) { return limits[userId], nil })
	defer s.Close()

	mustCreate(t, s, alice, "small")
	put(t, s, alice, "small", "a", "12345")
	put(t, s, alice, "small", "b", "12345")
	if _, err := s.PutObject(alice, "small", "c", strings.NewReader("1"), PutOptions{Size: 1}); !errors.Is(err, ErrQuotaExceeded) {
		t.Errorf("over quota: %v", err)
	}
	// Size unknown up front is still caught after the upload.
	if _, err := s.PutObject(alice, "small", "c", strings.NewReader("1"), PutOptions{Size: -1}); !errors.Is(err, ErrQuotaExceeded) {
		t.Errorf("over quota unknown size: %v", err)
	}
	// Overwriting counts only the difference.
	put(t, s, alice, "small", "a", "123")
	put(t, s, alice, "small", "a", "12345")

	// Writes by a grantee count against the owner.
	s.SetGrant(alice, "small", Grant{Type: GrantUser, Id: "bob", Access: GrantWrite})
	if _, err := s.PutObject(bob, "small", "d", strings.NewReader("1"), PutOptions{Size: 1}); !errors.Is(err, ErrQuotaExceeded) {
		t.Errorf("grantee over owner quota: %v", err)
	}

	// Copies count too.
	if _, err := s.CopyObject(alice, "small", "a", "small", "copy", nil); !errors.Is(err, ErrQuotaExceeded) {
		t.Errorf("copy over quota: %v", err)
	}

	// Transfer checks the new owner's room unless forced.
	limits["bob"] = 5
	if _, err := s.TransferBucket(admin, "small", "bob", "bob", true); !errors.Is(err, ErrQuotaExceeded) {
		t.Errorf("transfer over quota: %v", err)
	}
	b, err := s.TransferBucket(admin, "small", "bob", "bob", false)
	if err != nil || b.OwnerId != "bob" || b.Name != "bob--small" {
		t.Fatalf("forced transfer: %v %+v", err, b)
	}
	if len(b.Grants) != 0 {
		t.Errorf("grant to new owner kept: %+v", b.Grants)
	}
	if used := s.Usage("bob").UsedBytes; used != 10 {
		t.Errorf("bob usage %d", used)
	}
	if used := s.Usage("alice").UsedBytes; used != 0 {
		t.Errorf("alice usage %d", used)
	}
	// Alice lost access with ownership.
	if _, err := s.GetBucket(alice, "small"); !errors.Is(err, ErrNoSuchBucket) {
		t.Errorf("old owner access: %v", err)
	}
}

func TestListObjects(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "tree")
	for _, k := range []string{"a.txt", "dir/one", "dir/two", "dir/sub/three", "other/x", "z"} {
		put(t, s, alice, "tree", k, k)
	}

	res, _ := s.ListObjects(alice, "tree", "", "/", "", 0)
	if fmt.Sprint(keys(res.Objects)) != "[a.txt z]" || fmt.Sprint(res.Prefixes) != "[dir/ other/]" {
		t.Errorf("top level: %v %v", keys(res.Objects), res.Prefixes)
	}

	res, _ = s.ListObjects(alice, "tree", "dir/", "/", "", 0)
	if fmt.Sprint(keys(res.Objects)) != "[dir/one dir/two]" || fmt.Sprint(res.Prefixes) != "[dir/sub/]" {
		t.Errorf("dir level: %v %v", keys(res.Objects), res.Prefixes)
	}

	res, _ = s.ListObjects(alice, "tree", "dir/", "", "", 0)
	if len(res.Objects) != 3 {
		t.Errorf("recursive: %v", keys(res.Objects))
	}

	// Paging through everything one entry at a time, with roll-ups.
	var seen []string
	after := ""
	for i := 0; i < 10; i++ {
		res, _ = s.ListObjects(alice, "tree", "", "/", after, 1)
		seen = append(seen, keys(res.Objects)...)
		seen = append(seen, res.Prefixes...)
		if !res.IsTruncated {
			break
		}
		after = res.Next
	}
	if fmt.Sprint(seen) != "[a.txt dir/ other/ z]" {
		t.Errorf("paged: %v", seen)
	}
}

func keys(objs []*Object) []string {
	out := make([]string, len(objs))
	for i, o := range objs {
		out[i] = o.Key
	}
	return out
}

func TestCopyObject(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "src")
	mustCreate(t, s, alice, "dst")
	o := put(t, s, alice, "src", "file", "shared content")

	c, err := s.CopyObject(alice, "src", "file", "dst", "copy", &PutOptions{Meta: map[string]string{"Mtime": "1"}})
	if err != nil {
		t.Fatal(err)
	}
	if c.SHA256 != o.SHA256 || c.Meta["mtime"] != "1" {
		t.Errorf("copy %+v", c)
	}
	s.DeleteObject(alice, "src", "file")
	if got := read(t, s, alice, "dst", "copy"); got != "shared content" {
		t.Errorf("copy content after source delete %q", got)
	}
	if _, err := s.CopyObject(bob, "dst", "copy", "dst", "x", nil); !errors.Is(err, ErrNoSuchBucket) {
		t.Errorf("unauthorised copy: %v", err)
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir, nil)
	mustCreate(t, s, alice, "keep")
	put(t, s, alice, "keep", "a", "alpha")
	put(t, s, alice, "keep", "b", "beta")
	s.SetGrant(alice, "keep", Grant{Type: GrantUser, Id: "bob", Access: GrantRead})
	s.DeleteObject(alice, "keep", "b")
	mustCreate(t, s, alice, "gone")
	goneId, keepId := bid(s, "gone"), bid(s, "keep")
	s.DeleteBucket(alice, "gone", false)
	s.Close()

	s = openStore(t, dir, nil)
	defer s.Close()
	if got := read(t, s, bob, "keep", "a"); got != "alpha" {
		t.Errorf("after reopen %q", got)
	}
	if _, err := s.HeadObject(alice, "keep", "b"); !errors.Is(err, ErrNoSuchKey) {
		t.Errorf("deleted object back after reopen: %v", err)
	}
	if _, err := s.GetBucket(alice, "gone"); !errors.Is(err, ErrNoSuchBucket) {
		t.Errorf("deleted bucket back after reopen: %v", err)
	}
	// Tombstones survive compaction so peers cannot resurrect them.
	if d, _ := s.Digests("", 0); !d[goneId].Bucket.IsDeleted || d[keepId].Count != 2 {
		t.Errorf("digests after reopen %+v", d)
	}
}

func TestMultipart(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "big")

	id, err := s.CreateMultipart(alice, "big", "file.bin", PutOptions{ContentType: "application/x-test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UploadPart(bob, "big", "file.bin", id, 1, strings.NewReader("x"), 1, "", nil); err == nil {
		t.Error("bob uploaded a part")
	}

	parts := []string{"first-", "second-", "third"}
	var done []CompletedPart
	var etagRaw []byte
	for i, p := range parts {
		etag, err := s.UploadPart(alice, "big", "file.bin", id, i+1, strings.NewReader(p), int64(len(p)), "", nil)
		if err != nil {
			t.Fatal(err)
		}
		sum := md5.Sum([]byte(p))
		etagRaw = append(etagRaw, sum[:]...)
		done = append(done, CompletedPart{Number: i + 1, ETag: `"` + etag + `"`})
	}

	if listed, _ := s.ListParts(alice, "big", "file.bin", id); len(listed) != 3 {
		t.Errorf("listed %d parts", len(listed))
	}
	if ups, _ := s.ListMultipartUploads(alice, "big"); len(ups) != 1 {
		t.Errorf("listed %d uploads", len(ups))
	}

	// Out of order and wrong ETags are rejected.
	if _, err := s.CompleteMultipart(alice, "big", "file.bin", id, []CompletedPart{done[1], done[0]}, "alice"); !errors.Is(err, ErrInvalidPart) {
		t.Errorf("out of order: %v", err)
	}

	o, err := s.CompleteMultipart(alice, "big", "file.bin", id, done, "alice")
	if err != nil {
		t.Fatal(err)
	}
	want := md5.Sum(etagRaw)
	if o.ETag != hex.EncodeToString(want[:])+"-3" || o.ContentType != "application/x-test" {
		t.Errorf("object %+v", o)
	}
	if got := read(t, s, alice, "big", "file.bin"); got != strings.Join(parts, "") {
		t.Errorf("assembled %q", got)
	}
	if _, err := s.ListParts(alice, "big", "file.bin", id); !errors.Is(err, ErrNoSuchUpload) {
		t.Errorf("upload still present: %v", err)
	}

	id2, _ := s.CreateMultipart(alice, "big", "abort", PutOptions{})
	if err := s.AbortMultipart(alice, "big", "abort", id2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UploadPart(alice, "big", "abort", id2, 1, strings.NewReader("x"), 1, "", nil); !errors.Is(err, ErrNoSuchUpload) {
		t.Errorf("part after abort: %v", err)
	}
}

// memCluster wires stores together like the gossip cluster does: updates
// are delivered to every other store and chunks are read directly.
type memCluster struct {
	mu     sync.Mutex
	stores map[string]*Store
	down   map[string]bool
	// breakAfter, when set, cuts each content stream after that many
	// bytes, as a dropped connection would.
	breakAfter int64
	opens      int
	batches    int
}

type memReplicator struct {
	c    *memCluster
	self string
}

func (r *memReplicator) BroadcastFiles(buckets []*Bucket, objects []*Object) {
	r.c.mu.Lock()
	var targets []*Store
	for id, s := range r.c.stores {
		if id != r.self && !r.c.down[id] && !r.c.down[r.self] {
			targets = append(targets, s)
		}
	}
	r.c.mu.Unlock()
	for _, s := range targets {
		s.Merge(buckets, objects)
	}
}

// setDown takes a store off or back on the network, first delivering what
// every store has queued so changes land on the right side of the outage.
func (c *memCluster) setDown(id string, down bool) {
	c.mu.Lock()
	stores := make([]*Store, 0, len(c.stores))
	for _, s := range c.stores {
		stores = append(stores, s)
	}
	c.mu.Unlock()
	for _, s := range stores {
		s.flushBroadcasts()
	}
	c.mu.Lock()
	c.down[id] = down
	c.mu.Unlock()
}

func (r *memReplicator) FileNodes() []string {
	r.c.mu.Lock()
	defer r.c.mu.Unlock()
	var ids []string
	for id := range r.c.stores {
		if id != r.self && !r.c.down[id] {
			ids = append(ids, id)
		}
	}
	return ids
}

func (r *memReplicator) OpenContent(ctx context.Context, nodeId, sha string, offset int64) (io.ReadCloser, error) {
	r.c.mu.Lock()
	s := r.c.stores[nodeId]
	r.c.opens++
	limit := r.c.breakAfter
	r.c.mu.Unlock()
	if s == nil {
		return nil, ErrUnavailable
	}
	var buf bytes.Buffer
	if err := s.WriteContent(&buf, sha, offset); err != nil {
		return nil, err
	}
	if limit > 0 && int64(buf.Len()) > limit {
		return io.NopCloser(io.MultiReader(io.LimitReader(&buf, limit), errReader{io.ErrUnexpectedEOF})), nil
	}
	return io.NopCloser(&buf), nil
}

func (r *memReplicator) OpenContentBatch(ctx context.Context, nodeId string, shas []string) (io.ReadCloser, error) {
	r.c.mu.Lock()
	s := r.c.stores[nodeId]
	r.c.batches++
	r.c.mu.Unlock()
	if s == nil {
		return nil, ErrUnavailable
	}
	var buf bytes.Buffer
	if err := s.WriteContentBatch(&buf, shas); err != nil {
		return nil, err
	}
	return io.NopCloser(&buf), nil
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

// antiEntropy runs the exchange the cluster performs between a and b:
// bucket records both ways, then the records of the digest slots that
// differ. It returns how many object records moved.
func antiEntropy(a, b *Store) int {
	db, _ := b.Digests("", 0)
	a.Merge(digestBuckets(db), nil)
	b.Merge(a.NewerBuckets(db), nil)

	moved := 0
	syncSlots := func(from, to *Store) {
		all, _ := from.Digests("", 0)
		for name := range all {
			fd, fc := from.SlotDigests(name)
			td, tc := to.SlotDigests(name)
			var slots []int
			for i := range fd {
				if fd[i] != td[i] || fc[i] != tc[i] {
					slots = append(slots, i)
				}
			}
			if len(slots) == 0 {
				continue
			}
			for after := ""; ; {
				page, next := from.ObjectPage(name, after, 2, slots)
				to.Merge(nil, page)
				moved += len(page)
				if next == "" {
					break
				}
				after = next
			}
		}
	}
	syncSlots(b, a)
	syncSlots(a, b)
	return moved
}

func digestBuckets(d map[string]BucketDigest) []*Bucket {
	var out []*Bucket
	for _, v := range d {
		out = append(out, v.Bucket)
	}
	return out
}

func newCluster(t *testing.T, n int) (*memCluster, []*Store) {
	t.Helper()
	c := &memCluster{stores: map[string]*Store{}, down: map[string]bool{}}
	var stores []*Store
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("node%d", i)
		s, err := Open(Config{Dir: t.TempDir(), NodeId: id})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(s.Close)
		c.stores[id] = s
		s.bcNow = true
		s.SetReplicator(&memReplicator{c: c, self: id})
		stores = append(stores, s)
	}
	return c, stores
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestReplication(t *testing.T) {
	_, stores := newCluster(t, 3)
	a, b, c := stores[0], stores[1], stores[2]

	mustCreate(t, a, alice, "rep")
	// Content larger than a chunk so it is pulled in pieces.
	big := strings.Repeat("0123456789abcdef", 300*1024)
	o := put(t, a, alice, "rep", "big.bin", big)

	for i, s := range []*Store{b, c} {
		waitFor(t, fmt.Sprintf("content on store %d", i+1), func() bool { return s.holds(o.SHA256) })
		if got := read(t, s, alice, "rep", "big.bin"); got != big {
			t.Errorf("store %d content differs", i+1)
		}
	}

	// A write on another server replicates back.
	put(t, c, alice, "rep", "small", "from c")
	if got := read(t, a, alice, "rep", "small"); got != "from c" {
		t.Errorf("a read %q", got)
	}

	// Grants and deletes replicate; content is removed everywhere.
	a.SetGrant(alice, "rep", Grant{Type: GrantUser, Id: "bob", Access: GrantRead})
	if got := read(t, b, bob, "rep", "small"); got != "from c" {
		t.Errorf("bob read on b %q", got)
	}
	b.DeleteObject(alice, "rep", "big.bin")
	for i, s := range stores {
		if s.holds(o.SHA256) {
			t.Errorf("store %d still holds deleted content", i)
		}
	}
}

func TestReadFetchesMissingContent(t *testing.T) {
	cl, stores := newCluster(t, 2)
	a, b := stores[0], stores[1]

	mustCreate(t, a, alice, "lazy")
	// Deliver the metadata to b without letting it fetch in the background.
	cl.setDown("node1", true)
	o := put(t, a, alice, "lazy", "f", "pulled on read")
	cl.setDown("node1", false)
	da, _ := a.Digests("", 0)
	b.Merge(digestBuckets(da), nil)
	page, _ := a.ObjectPage(bid(a, "lazy"), "", 10, nil)
	b.forgetContent(o.SHA256)
	b.Merge(nil, page)

	if got := read(t, b, alice, "lazy", "f"); got != "pulled on read" {
		t.Errorf("read %q", got)
	}
}

func TestAntiEntropy(t *testing.T) {
	cl, stores := newCluster(t, 2)
	a, b := stores[0], stores[1]

	// b is partitioned while a changes.
	cl.setDown("node1", true)

	mustCreate(t, a, alice, "aes")
	for i := 0; i < 5; i++ {
		put(t, a, alice, "aes", fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i))
	}
	a.DeleteObject(alice, "aes", "k0")
	mustCreate(t, a, alice, "tmp")
	a.DeleteBucket(alice, "tmp", false)

	// b made its own change meanwhile.
	mustCreate(t, b, bob, "bonly")
	put(t, b, bob, "bonly", "x", "y")

	if _, err := b.GetBucket(alice, "aes"); !errors.Is(err, ErrNoSuchBucket) {
		t.Fatal("partition leaked")
	}

	cl.setDown("node1", false)
	antiEntropy(a, b)

	da, _ := a.Digests("", 0)
	db, _ := b.Digests("", 0)
	for name, d := range da {
		if db[name].Digest != d.Digest || db[name].Count != d.Count {
			t.Errorf("bucket %s differs after sync: %+v vs %+v", name, d, db[name])
		}
	}
	if len(da) != len(db) {
		t.Errorf("bucket counts differ %d vs %d", len(da), len(db))
	}
	waitFor(t, "content on b", func() bool {
		o, err := b.HeadObject(alice, "aes", "k4")
		return err == nil && b.holds(o.SHA256)
	})
	if got := read(t, b, alice, "aes", "k3"); got != "v3" {
		t.Errorf("b read %q", got)
	}
	if _, err := b.HeadObject(alice, "aes", "k0"); !errors.Is(err, ErrNoSuchKey) {
		t.Errorf("deleted object on b: %v", err)
	}
	if got := read(t, a, bob, "bonly", "x"); got != "y" {
		t.Errorf("a read b's object %q", got)
	}
}

func TestLastWriterWins(t *testing.T) {
	_, stores := newCluster(t, 2)
	a, b := stores[0], stores[1]
	mustCreate(t, a, alice, "lww")

	older := put(t, a, alice, "lww", "k", "older")
	newer := *older
	newer.SHA256 = older.SHA256
	newer.UpdatedAt = older.UpdatedAt + 1
	newer.Size = 999

	// Applying the older record after the newer one changes nothing.
	b.Merge(nil, []*Object{&newer})
	b.Merge(nil, []*Object{older})
	if o, _ := b.HeadObject(alice, "lww", "k"); o.Size != 999 {
		t.Errorf("older record won: %+v", o)
	}
}

func TestDeleteUser(t *testing.T) {
	_, stores := newCluster(t, 2)
	a, b := stores[0], stores[1]

	mustCreate(t, a, alice, "alice-one")
	mustCreate(t, a, alice, "alice-two")
	mustCreate(t, a, bob, "bobs-bucket")
	o := put(t, a, alice, "alice-one", "f", "alice's file")
	a.SetGrant(bob, "bobs-bucket", Grant{Type: GrantUser, Id: "alice", Access: GrantWrite})
	a.SetGrant(bob, "bobs-bucket", Grant{Type: GrantUser, Id: "carol", Access: GrantRead})
	waitFor(t, "content on b", func() bool { return b.holds(o.SHA256) })

	if n := a.DeleteUser("alice"); n != 2 {
		t.Errorf("deleted %d buckets", n)
	}
	for i, s := range stores {
		if _, err := s.GetBucket(admin, "alice-one"); !errors.Is(err, ErrNoSuchBucket) {
			t.Errorf("store %d: alice's bucket remains: %v", i, err)
		}
		waitFor(t, fmt.Sprintf("store %d to drop alice's content", i), func() bool { return !s.holds(o.SHA256) })
		info, err := s.GetBucket(bob, "bobs-bucket")
		if err != nil || len(info.Grants) != 1 || info.Grants[0].Id != "carol" {
			t.Errorf("store %d: bob's bucket grants %+v %v", i, info, err)
		}
		if used := s.Usage("alice").UsedBytes; used != 0 {
			t.Errorf("store %d: alice usage %d", i, used)
		}
	}
	// The names are free again.
	mustCreate(t, b, bob, "alice-one")
}

func TestUserWithoutPermission(t *testing.T) {
	s := newStore(t)

	// Cannot own buckets...
	if _, err := s.CreateBucket(dave, "daves"); !errors.Is(err, ErrCannotOwn) {
		t.Errorf("create without permission: %v", err)
	}

	// ...but reaches what is shared with them, at the access the owner chose.
	mustCreate(t, s, alice, "team")
	put(t, s, alice, "team", "f", "x")
	if _, err := s.GetBucket(dave, "team"); !errors.Is(err, ErrNoSuchBucket) {
		t.Errorf("unshared: %v", err)
	}
	s.SetGrant(alice, "team", Grant{Type: GrantGroup, Id: "devs", Access: GrantRead})
	if got := read(t, s, dave, "team", "f"); got != "x" {
		t.Errorf("read via group %q", got)
	}
	if _, err := s.PutObject(dave, "team", "g", strings.NewReader("y"), PutOptions{Size: 1}); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("write with read grant: %v", err)
	}
	s.SetGrant(alice, "team", Grant{Type: GrantUser, Id: "dave", Access: GrantWrite})
	put(t, s, dave, "team", "g", "y")
	if _, err := s.SetGrant(dave, "team", Grant{Type: GrantAll, Access: GrantRead}); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("manage without permission: %v", err)
	}
	if n := len(s.ListBuckets(dave, false)); n != 1 {
		t.Errorf("dave lists %d buckets", n)
	}

	// An owner who loses the permission loses owner rights over their buckets.
	transferred, err := s.TransferBucket(admin, "team", "dave", "dave", false)
	if err != nil || transferred.OwnerId != "dave" {
		t.Fatalf("transfer: %v", err)
	}
	// Dave still reads it through the devs group grant, but cannot manage it.
	if got := read(t, s, dave, "dave--team", "f"); got != "x" {
		t.Errorf("read after transfer %q", got)
	}
	if err := s.DeleteBucket(dave, "dave--team", true); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("owner without permission deleted bucket: %v", err)
	}
}

func TestDeletedBucketDropsRecords(t *testing.T) {
	s := newStore(t)
	b := mustBucket(t, s, alice, "gone")
	put(t, s, alice, "gone", "a", "1")
	put(t, s, alice, "gone", "b", "2")
	s.DeleteBucket(alice, "gone", true)

	waitFor(t, "records of the deleted bucket swept", func() bool {
		page, _ := s.ObjectPage(b.Id, "", 10, nil)
		return len(page) == 0
	})
	// A late update for the deleted generation is ignored.
	s.Merge(nil, []*Object{{BucketId: b.Id, Key: "late", SHA256: strings.Repeat("b", 64), UpdatedAt: b.UpdatedAt + 100}})
	if page, _ := s.ObjectPage(b.Id, "", 10, nil); len(page) != 0 {
		t.Errorf("late object for deleted bucket stored")
	}
}

func TestCleanup(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir, nil)
	defer s.Close()

	mustCreate(t, s, alice, "keep")
	kept := put(t, s, alice, "keep", "k", "kept content")

	// Content nothing references, e.g. left by a crash.
	orphan := strings.Repeat("c", 64)
	os.MkdirAll(filepath.Join(dir, "blobs", "cc", "cc"), 0700)
	os.WriteFile(filepath.Join(dir, "blobs", "cc", "cc", orphan), []byte("junk"), 0600)

	// A temporary file abandoned an hour ago, and one still being written.
	old := filepath.Join(dir, "tmp", "upload-old")
	fresh := filepath.Join(dir, "tmp", "upload-fresh")
	os.WriteFile(old, []byte("x"), 0600)
	os.WriteFile(fresh, []byte("x"), 0600)
	past := time.Now().Add(-2 * time.Hour)
	os.Chtimes(old, past, past)

	// A multipart upload into a bucket that is then deleted.
	mustCreate(t, s, alice, "temp")
	id, _ := s.CreateMultipart(alice, "temp", "big", PutOptions{})
	s.UploadPart(alice, "temp", "big", id, 1, strings.NewReader("part"), 4, "", nil)
	s.DeleteBucket(alice, "temp", false)

	// Content of a deleted object: removed straight away, its directories by the sweep.
	gone := put(t, s, alice, "keep", "gone", "deleted content")
	s.DeleteObject(alice, "keep", "gone")

	st := s.cleanup()
	if st.Blobs != 1 || st.TempFiles != 1 || st.Uploads != 1 {
		t.Errorf("cleanup removed %+v", st)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("abandoned temp file kept")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Error("in-progress temp file removed")
	}
	if !s.holds(kept.SHA256) {
		t.Error("referenced content removed")
	}
	for _, sha := range []string{orphan, gone.SHA256} {
		if _, err := os.Stat(filepath.Join(dir, "blobs", sha[:2])); !os.IsNotExist(err) {
			t.Errorf("empty content directory %s kept", sha[:2])
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "multipart", id)); !os.IsNotExist(err) {
		t.Error("upload for deleted bucket kept")
	}
}

func TestAccessibleBuckets(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "a-owned")
	mustCreate(t, s, bob, "b-direct")
	mustCreate(t, s, bob, "b-group")
	mustCreate(t, s, bob, "b-all")
	mustCreate(t, s, bob, "b-private")
	mustCreate(t, s, bob, "b-both")
	s.SetGrant(bob, "b-direct", Grant{Type: GrantUser, Id: "alice", Access: GrantRead})
	s.SetGrant(bob, "b-group", Grant{Type: GrantGroup, Id: "devs", Access: GrantWrite})
	s.SetGrant(bob, "b-all", Grant{Type: GrantAll, Access: GrantRead})
	// The stronger route wins: write via group beats read granted directly.
	s.SetGrant(bob, "b-both", Grant{Type: GrantUser, Id: "alice", Access: GrantRead})
	s.SetGrant(bob, "b-both", Grant{Type: GrantGroup, Id: "devs", Access: GrantWrite})

	got := map[string]BucketAccess{}
	for _, ba := range s.AccessibleBuckets(alice) {
		got[ba.Name] = ba
	}
	want := map[string][3]string{
		"a-owned":  {"3", ViaOwner, ""},
		"b-direct": {"1", ViaUser, ""},
		"b-group":  {"2", ViaGroup, "devs"},
		"b-all":    {"1", ViaAll, ""},
		"b-both":   {"2", ViaGroup, "devs"},
	}
	if len(got) != len(want) {
		t.Errorf("got %d buckets: %+v", len(got), got)
	}
	for name, w := range want {
		g := got[name]
		if fmt.Sprint(g.Access) != w[0] || g.Via != w[1] || g.GroupId != w[2] {
			t.Errorf("%s: %+v, want %v", name, g, w)
		}
	}

	// An administrator's blanket access is not listed, only real grants.
	if l := s.AccessibleBuckets(admin); len(l) != 1 || l[0].Name != "b-all" || l[0].Via != ViaAll {
		t.Errorf("admin listed %+v", l)
	}
	// Without the permission an owner's own bucket is not owner access:
	// alice keeps b-direct, b-all and b-both through their grants only.
	if n := len(s.AccessibleBuckets(&Principal{UserId: "alice", Groups: []string{"ops"}})); n != 3 {
		t.Errorf("alice without permission sees %d buckets", n)
	}
}

func TestConditionalPut(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "cond")
	o := put(t, s, alice, "cond", "f", "v1")

	if _, err := s.PutObject(alice, "cond", "f", strings.NewReader("x"), PutOptions{Size: -1, IfNoneMatch: true}); !errors.Is(err, ErrPrecondition) {
		t.Errorf("create-only over existing: %v", err)
	}
	if _, err := s.PutObject(alice, "cond", "new", strings.NewReader("x"), PutOptions{Size: -1, IfNoneMatch: true}); err != nil {
		t.Errorf("create-only new: %v", err)
	}
	if _, err := s.PutObject(alice, "cond", "f", strings.NewReader("v2"), PutOptions{Size: -1, IfMatch: "\"" + o.ETag + "\""}); err != nil {
		t.Errorf("matching etag: %v", err)
	}
	// The ETag read before the last write is now stale.
	if _, err := s.PutObject(alice, "cond", "f", strings.NewReader("v3"), PutOptions{Size: -1, IfMatch: o.ETag}); !errors.Is(err, ErrPrecondition) {
		t.Errorf("stale etag: %v", err)
	}
	if got := read(t, s, alice, "cond", "f"); got != "v2" {
		t.Errorf("content %q", got)
	}
}

func TestBucketLimit(t *testing.T) {
	limits := map[string]int{"alice": 2, "bob": 1}
	s, err := Open(Config{Dir: t.TempDir(), BucketLimit: func(id string) (int, error) { return limits[id], nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	mustCreate(t, s, alice, "a-one")
	mustCreate(t, s, alice, "a-two")
	if _, err := s.CreateBucket(alice, "a-three"); !errors.Is(err, ErrBucketLimit) {
		t.Errorf("third bucket: %v", err)
	}
	// Deleting one frees a place.
	s.DeleteBucket(alice, "a-two", false)
	mustCreate(t, s, alice, "a-three")
	if n := s.Usage("alice").Buckets; n != 2 {
		t.Errorf("owned %d", n)
	}

	// Transfers respect the new owner's limit unless forced.
	mustCreate(t, s, bob, "b-one")
	if _, err := s.TransferBucket(admin, "a-one", "bob", "bob", true); !errors.Is(err, ErrBucketLimit) {
		t.Errorf("transfer over limit: %v", err)
	}
	if _, err := s.TransferBucket(admin, "a-one", "bob", "bob", false); err != nil {
		t.Errorf("forced transfer: %v", err)
	}
	// No limit set means unlimited.
	for i := 0; i < 5; i++ {
		mustCreate(t, s, carol, fmt.Sprintf("c-%d", i))
	}
}

func TestBucketInfoGranted(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "rel")
	s.SetGrant(alice, "rel", Grant{Type: GrantGroup, Id: "devs", Access: GrantRead})

	name := "rel"
	check := func(p *Principal, access, granted int, via string) {
		t.Helper()
		info, err := s.GetBucket(p, name)
		if err != nil {
			t.Fatal(err)
		}
		if info.Access != access || info.Granted != granted || info.Via != via {
			t.Errorf("%s: access %d granted %d via %q", p.UserId, info.Access, info.Granted, info.Via)
		}
	}
	check(alice, AccessOwner, AccessOwner, ViaOwner)
	check(carol, AccessRead, AccessRead, ViaGroup)
	// An administrator manages it but holds nothing in their own right.
	check(admin, AccessOwner, AccessNone, "")

	// After a transfer the previous owner keeps only what grants give them.
	s.SetGrant(alice, "rel", Grant{Type: GrantUser, Id: "alice", Access: GrantRead})
	s.TransferBucket(admin, "rel", "bob", "bob", false)
	name = "bob--rel"
	check(alice, AccessRead, AccessRead, ViaUser)
}

func TestTransferRenames(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "alice--notes")
	put(t, s, alice, "alice--notes", "a.txt", "hello")
	put(t, s, alice, "alice--notes", "dir/b.txt", "world")
	s.SetGrant(alice, "alice--notes", Grant{Type: GrantGroup, Id: "devs", Access: GrantRead})

	// Without the transfer permission the owner cannot give it away.
	noTransfer := *alice
	noTransfer.CanTransfer = false
	if _, err := s.TransferBucket(&noTransfer, "alice--notes", "bob", "bob", false); !errors.Is(err, ErrCannotTransfer) {
		t.Errorf("transfer without permission: %v", err)
	}
	// Nor can someone else transfer it.
	if _, err := s.TransferBucket(carol, "alice--notes", "bob", "bob", false); err == nil {
		t.Errorf("non-owner transferred")
	}

	// The name is taken in the new owner's namespace.
	mustCreate(t, s, bob, "bob--notes")
	if _, err := s.TransferBucket(alice, "alice--notes", "bob", "bob", false); !errors.Is(err, ErrTransferNameTaken) {
		t.Errorf("name taken: %v", err)
	}
	s.DeleteBucket(bob, "bob--notes", true)

	b, err := s.TransferBucket(alice, "alice--notes", "bob", "Bob", false)
	if err != nil {
		t.Fatal(err)
	}
	if b.Name != "bob--notes" || b.OwnerId != "bob" {
		t.Fatalf("transferred %+v", b)
	}
	// Content, grants and the old name moved with it.
	if got := read(t, s, bob, "bob--notes", "dir/b.txt"); got != "world" {
		t.Errorf("content %q", got)
	}
	if got := read(t, s, carol, "bob--notes", "a.txt"); got != "hello" {
		t.Errorf("group grant lost: %q", got)
	}
	if _, err := s.GetBucket(admin, "alice--notes"); !errors.Is(err, ErrNoSuchBucket) {
		t.Errorf("old name still exists: %v", err)
	}
	// The old name is free for alice again and starts empty.
	mustCreate(t, s, alice, "alice--notes")
	if res, _ := s.ListObjects(alice, "alice--notes", "", "", "", 0); len(res.Objects) != 0 {
		t.Errorf("recreated bucket has %d objects", len(res.Objects))
	}
}

func TestSharePermission(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "alice--shared")
	noShare := *alice
	noShare.CanShare = false
	if _, err := s.SetGrant(&noShare, "alice--shared", Grant{Type: GrantUser, Id: "bob", Access: GrantRead}); !errors.Is(err, ErrCannotShare) {
		t.Errorf("share without permission: %v", err)
	}
	if _, err := s.SetGrant(alice, "alice--shared", Grant{Type: GrantUser, Id: "bob", Access: GrantRead}); err != nil {
		t.Errorf("share: %v", err)
	}
	if _, err := s.RemoveGrant(&noShare, "alice--shared", GrantUser, "bob"); !errors.Is(err, ErrCannotShare) {
		t.Errorf("unshare without permission: %v", err)
	}
	// An administrator may always manage sharing.
	if _, err := s.RemoveGrant(admin, "alice--shared", GrantUser, "bob"); err != nil {
		t.Errorf("admin unshare: %v", err)
	}
}

// A small difference in a large bucket moves only the records of the digest
// slots that differ, not the whole bucket.
func TestAntiEntropyMovesOnlyDifferingSlots(t *testing.T) {
	c, stores := newCluster(t, 2)
	a, b := stores[0], stores[1]
	mustCreate(t, a, alice, "big")
	for i := 0; i < 300; i++ {
		put(t, a, alice, "big", fmt.Sprintf("k/%04d", i), "v")
	}
	if moved := antiEntropy(a, b); moved != 0 {
		t.Fatalf("in-sync stores moved %d records", moved)
	}

	// Three changes b never hears about.
	c.setDown("node1", true)
	put(t, a, alice, "big", "k/0001", "changed")
	put(t, a, alice, "big", "k/0150", "changed")
	if err := a.DeleteObject(alice, "big", "k/0299"); err != nil {
		t.Fatal(err)
	}
	c.setDown("node1", false)

	moved := antiEntropy(a, b)
	// Each differing slot holds about 300/64 records.
	if moved == 0 || moved > 60 {
		t.Errorf("moved %d records for 3 changes in 300", moved)
	}
	da, _ := a.Digests("", 0)
	db, _ := b.Digests("", 0)
	if da["big"].Digest != db["big"].Digest || da["big"].Count != db["big"].Count {
		t.Errorf("not converged: %+v vs %+v", da["big"], db["big"])
	}
	if got := read(t, b, alice, "big", "k/0150"); got != "changed" {
		t.Errorf("k/0150 = %q", got)
	}
}

// ListObjects seeks through the sorted keys; check it against a plain
// filter-and-sort for random keys, prefixes, delimiters and page sizes.
func TestListObjectsMatchesReference(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "lst")
	r := rand.New(rand.NewSource(1))
	parts := []string{"a", "b", "a/b", "c", "z", "a-b", "a.b", "dir"}
	var live []string
	for i := 0; i < 150; i++ {
		k := fmt.Sprintf("%s/%s/%d", parts[r.Intn(len(parts))], parts[r.Intn(len(parts))], r.Intn(30))
		put(t, s, alice, "lst", k, "x")
	}
	// Some tombstones, which listings must skip.
	res, _ := s.ListObjects(alice, "lst", "", "", "", 1000)
	for i, o := range res.Objects {
		if i%5 == 0 {
			s.DeleteObject(alice, "lst", o.Key)
		}
	}
	res, _ = s.ListObjects(alice, "lst", "", "", "", 1000)
	for _, o := range res.Objects {
		live = append(live, o.Key)
	}

	reference := func(prefix, delimiter string) []string {
		var out []string
		seen := map[string]bool{}
		for _, k := range live {
			if !strings.HasPrefix(k, prefix) {
				continue
			}
			entry := k
			if delimiter != "" {
				if i := strings.Index(k[len(prefix):], delimiter); i >= 0 {
					entry = k[:len(prefix)+i+len(delimiter)]
				}
			}
			if !seen[entry] {
				seen[entry] = true
				out = append(out, entry)
			}
		}
		sort.Strings(out)
		return out
	}

	for _, prefix := range []string{"", "a", "a/", "a/b/", "dir/", "zz", "c/a"} {
		for _, delimiter := range []string{"", "/", "-"} {
			for _, max := range []int{1, 3, 7, 1000} {
				var got []string
				after := ""
				for pages := 0; ; pages++ {
					res, err := s.ListObjects(alice, "lst", prefix, delimiter, after, max)
					if err != nil || pages > 1000 {
						t.Fatalf("list %q %q: %v", prefix, delimiter, err)
					}
					var page []string
					for _, o := range res.Objects {
						page = append(page, o.Key)
					}
					page = append(page, res.Prefixes...)
					sort.Strings(page)
					got = append(got, page...)
					if !res.IsTruncated {
						break
					}
					after = res.Next
				}
				want := reference(prefix, delimiter)
				if strings.Join(got, ",") != strings.Join(want, ",") {
					t.Errorf("prefix %q delimiter %q max %d:\n got %v\nwant %v", prefix, delimiter, max, got, want)
				}
			}
		}
	}
}

func TestRecordLimits(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "lim")

	big := map[string]string{"k": strings.Repeat("v", MaxMetaBytes)}
	if _, err := s.PutObject(alice, "lim", "a", strings.NewReader("x"), PutOptions{Size: 1, Meta: big}); !errors.Is(err, ErrMetadataTooLarge) {
		t.Errorf("large metadata: %v", err)
	}
	if _, err := s.PutObject(alice, "lim", "a", strings.NewReader("x"), PutOptions{Size: 1, ContentType: strings.Repeat("t", MaxContentTypeSize+1)}); !errors.Is(err, ErrMetadataTooLarge) {
		t.Errorf("long content type: %v", err)
	}
	if _, err := s.CreateMultipart(alice, "lim", "m", PutOptions{Meta: big}); !errors.Is(err, ErrMetadataTooLarge) {
		t.Errorf("multipart metadata: %v", err)
	}

	if _, err := s.SetGrant(alice, "lim", Grant{Type: GrantUser, Id: "bob", Access: "admin"}); !errors.Is(err, ErrInvalidGrant) {
		t.Errorf("bad access: %v", err)
	}
	for i := 0; i < MaxGrants; i++ {
		if _, err := s.SetGrant(alice, "lim", Grant{Type: GrantUser, Id: fmt.Sprintf("u%d", i), Access: GrantRead}); err != nil {
			t.Fatalf("grant %d: %v", i, err)
		}
	}
	if _, err := s.SetGrant(alice, "lim", Grant{Type: GrantUser, Id: "one-more", Access: GrantRead}); !errors.Is(err, ErrTooManyGrants) {
		t.Errorf("grant past the limit: %v", err)
	}
	// Changing an existing grant is still allowed.
	if _, err := s.SetGrant(alice, "lim", Grant{Type: GrantUser, Id: "u0", Access: GrantWrite}); err != nil {
		t.Errorf("update at the limit: %v", err)
	}
}

func TestUsageAndAccessibleBuckets(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "u-one")
	mustCreate(t, s, alice, "u-two")
	put(t, s, alice, "u-one", "a", "12345")
	put(t, s, alice, "u-two", "b", "123")
	if u := s.Usage("alice"); u.UsedBytes != 8 || u.Objects != 2 || u.Buckets != 2 {
		t.Errorf("usage %+v", u)
	}
	s.DeleteBucket(alice, "u-two", true)
	if u := s.Usage("alice"); u.UsedBytes != 5 || u.Objects != 1 || u.Buckets != 1 {
		t.Errorf("usage after delete %+v", u)
	}

	if s.HasAccessibleBucket(dave) {
		t.Error("dave reaches a bucket before any share")
	}
	s.SetGrant(alice, "u-one", Grant{Type: GrantGroup, Id: "devs", Access: GrantRead})
	if !s.HasAccessibleBucket(dave) {
		t.Error("dave does not reach the bucket shared with devs")
	}
}

// A fetch whose stream breaks resumes from what it already has rather than
// starting again.
func TestFetchResumesBrokenStreams(t *testing.T) {
	cl, stores := newCluster(t, 2)
	a, b := stores[0], stores[1]
	mustCreate(t, a, alice, "resume")

	cl.setDown("node1", true)
	content := strings.Repeat("resumable content ", 20000) // ~360KB
	o := put(t, a, alice, "resume", "f", content)
	cl.mu.Lock()
	cl.breakAfter = 100 * 1024 // every stream breaks after 100KB
	cl.opens = 0
	cl.mu.Unlock()
	cl.setDown("node1", false)

	da, _ := a.Digests("", 0)
	b.Merge(digestBuckets(da), nil)
	page, _ := a.ObjectPage(bid(a, "resume"), "", 10, nil)
	b.Merge(nil, page)

	if got := read(t, b, alice, "resume", "f"); got != content {
		t.Fatalf("content differs: %d bytes", len(got))
	}
	cl.mu.Lock()
	opens := cl.opens
	cl.mu.Unlock()
	// 360KB in 100KB pieces: four streams, each resuming where the last broke.
	if opens < 4 || opens > 6 {
		t.Errorf("opened %d streams for a 360KB blob broken every 100KB", opens)
	}
	if !b.holds(o.SHA256) {
		t.Error("content not stored")
	}
}

// A body of unknown size stops at the owner's remaining quota instead of
// filling the disk first.
func TestUnknownSizeUploadCappedByQuota(t *testing.T) {
	s, err := Open(Config{Dir: t.TempDir(), Quota: func(string) (int64, error) { return 1000, nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	mustCreate(t, s, alice, "cap")
	put(t, s, alice, "cap", "a", strings.Repeat("x", 600))

	body := &countingReader{r: strings.NewReader(strings.Repeat("y", 1<<20))}
	if _, err := s.PutObject(alice, "cap", "b", body, PutOptions{Size: -1}); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("unknown size over quota: %v", err)
	}
	if body.n > 401+256*1024 {
		t.Errorf("read %d bytes of a body that could only ever be refused", body.n)
	}
	if entries, _ := os.ReadDir(filepath.Join(s.dir, "tmp")); len(entries) != 0 {
		t.Errorf("temp file left: %d", len(entries))
	}
	// Exactly what is left fits, size unknown.
	if _, err := s.PutObject(alice, "cap", "c", strings.NewReader(strings.Repeat("z", 400)), PutOptions{Size: -1}); err != nil {
		t.Errorf("fill to the quota: %v", err)
	}
}

type countingReader struct {
	r io.Reader
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

func TestUploadPartCopy(t *testing.T) {
	limits := map[string]int64{}
	s, err := Open(Config{Dir: t.TempDir(), Quota: func(id string) (int64, error) { return limits[id], nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	mustCreate(t, s, alice, "src")
	mustCreate(t, s, bob, "dst")
	put(t, s, alice, "src", "f", "0123456789")

	id, err := s.CreateMultipart(bob, "dst", "copy", PutOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Bob cannot read alice's bucket yet.
	if _, err := s.UploadPartCopy(ctx, bob, "dst", "copy", id, 1, "src", "f", 0, -1); !errors.Is(err, ErrNoSuchBucket) {
		t.Errorf("copy from unreadable bucket: %v", err)
	}
	s.SetGrant(alice, "src", Grant{Type: GrantUser, Id: "bob", Access: GrantRead})

	e1, err := s.UploadPartCopy(ctx, bob, "dst", "copy", id, 1, "src", "f", 2, 5)
	if err != nil {
		t.Fatal(err)
	}
	e2, err := s.UploadPartCopy(ctx, bob, "dst", "copy", id, 2, "src", "f", 0, -1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UploadPartCopy(ctx, bob, "dst", "copy", id, 3, "src", "f", 5, 10); !errors.Is(err, ErrInvalidRange) {
		t.Errorf("range past the end: %v", err)
	}
	if _, err := s.CompleteMultipart(bob, "dst", "copy", id, []CompletedPart{{1, e1}, {2, e2}}, "bob"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, s, bob, "dst", "copy"); got != "23450123456789" {
		t.Errorf("assembled %q", got)
	}

	// The copied part counts against the destination owner's quota.
	limits["bob"] = 20
	id, _ = s.CreateMultipart(bob, "dst", "big", PutOptions{})
	if _, err := s.UploadPartCopy(ctx, bob, "dst", "big", id, 1, "src", "f", 0, -1); !errors.Is(err, ErrQuotaExceeded) {
		t.Errorf("copy part over quota: %v", err)
	}
}

// Records travel as JSON, which replaces invalid UTF-8, so a key or
// metadata that isn't valid UTF-8 would change on restart and part this
// server from the others. They are refused instead.
func TestInvalidUTF8Refused(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "utf")
	put(t, s, alice, "utf", "src", "x")
	bad := "bad\xff\xfeutf8"

	if _, err := s.PutObject(alice, "utf", bad, strings.NewReader("x"), PutOptions{Size: 1}); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("invalid key: %v", err)
	}
	for name, opts := range map[string]PutOptions{
		"content type": {Size: 1, ContentType: "text/" + bad},
		"meta value":   {Size: 1, Meta: map[string]string{"k": bad}},
		"meta key":     {Size: 1, Meta: map[string]string{bad: "v"}},
	} {
		if _, err := s.PutObject(alice, "utf", "k", strings.NewReader("x"), opts); !errors.Is(err, ErrInvalidMetadata) {
			t.Errorf("put with invalid %s: %v", name, err)
		}
		if _, err := s.CreateMultipart(alice, "utf", "m", opts); !errors.Is(err, ErrInvalidMetadata) {
			t.Errorf("multipart with invalid %s: %v", name, err)
		}
		o := opts
		if _, err := s.CopyObject(alice, "utf", "src", "utf", "dst", &o); !errors.Is(err, ErrInvalidMetadata) {
			t.Errorf("copy with invalid %s: %v", name, err)
		}
	}
	if _, err := s.CopyObject(alice, "utf", "src", "utf", bad, nil); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("copy to invalid key: %v", err)
	}
	if _, err := s.CreateMultipart(alice, "utf", bad, PutOptions{}); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("multipart to invalid key: %v", err)
	}

	// A record with an invalid key from another server is not applied.
	s.mu.RLock()
	b := s.bucketByNameLocked("utf")
	s.mu.RUnlock()
	rec := &Object{BucketId: b.Id, Key: bad, SHA256: strings.Repeat("a", 64), UpdatedAt: hlc.Now()}
	s.Merge(nil, []*Object{rec})
	var held *Object
	s.db.View(func(txn *badger.Txn) (err error) { held, err = getObject(txn, b.Id, bad); return })
	if held != nil {
		t.Error("merged a record with an invalid UTF-8 key")
	}
}

func TestUnicodeSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir, nil)
	mustCreate(t, s, alice, "utf")
	key := "dir/файл-ü-日本-🙂.txt"
	if _, err := s.PutObject(alice, "utf", key, strings.NewReader("x"), PutOptions{Size: 1, ContentType: "text/plain; charset=ü",
		Meta: map[string]string{"note": "größe"}}); err != nil {
		t.Fatal(err)
	}
	_, f0, err := s.OpenObject(context.Background(), alice, "utf", key)
	if err != nil {
		t.Fatal(err)
	}
	f0.Close()
	before, _ := s.SlotDigests(bid(s, "utf"))
	s.Close()

	s = openStore(t, dir, nil)
	defer s.Close()
	o, f, err := s.OpenObject(context.Background(), alice, "utf", key)
	if err != nil {
		t.Fatalf("key changed across restart: %v", err)
	}
	f.Close()
	if o.Meta["note"] != "größe" || o.ContentType != "text/plain; charset=ü" {
		t.Errorf("metadata changed across restart: %+v", o)
	}
	if after, _ := s.SlotDigests(bid(s, "utf")); fmt.Sprint(after) != fmt.Sprint(before) {
		t.Error("digests changed across restart")
	}
}

// NoSync only skips the forced flush to disk: writes still reach the files,
// survive a clean restart.
func TestNoSync(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(Config{Dir: dir, NoSync: true})
	if err != nil {
		t.Fatal(err)
	}
	if !s.noSync || !s.blobs.noSync {
		t.Fatal("NoSync did not reach the store and its content")
	}
	if tw, err := s.blobs.newTemp(); err != nil || !tw.noSync {
		t.Fatalf("temporary files do not follow NoSync: %v", err)
	} else {
		tw.discard()
	}
	mustCreate(t, s, alice, "nosync")
	put(t, s, alice, "nosync", "a.txt", "alpha")
	put(t, s, alice, "nosync", "b.txt", "beta")
	if err := s.DeleteObject(alice, "nosync", "b.txt"); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s, err = Open(Config{Dir: dir, NoSync: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got := read(t, s, alice, "nosync", "a.txt"); got != "alpha" {
		t.Errorf("after restart %q", got)
	}
	if _, _, err := s.OpenObject(context.Background(), alice, "nosync", "b.txt"); err == nil {
		t.Error("a deleted file came back")
	}

	// The default syncs.
	d := newStore(t)
	if d.noSync || d.blobs.noSync {
		t.Error("the default does not sync")
	}
}

// forgetContent removes stored content, as if this server had never held it.
func (s *Store) forgetContent(sha string) {
	s.db.Update(func(txn *badger.Txn) error { return txn.Delete(shaKey('c', sha)) })
	s.blobs.remove(sha)
}
