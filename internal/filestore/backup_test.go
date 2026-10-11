package filestore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/paularlott/gossip/hlc"
)

// export reads what a backup of the store would hold.
func export(t *testing.T, s *Store, keep func(*Bucket) bool) ([]*Bucket, []*Object) {
	t.Helper()
	var buckets []*Bucket
	var objects []*Object
	if err := s.StreamBuckets(keep, func(b *Bucket) error { buckets = append(buckets, b); return nil }); err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, b := range buckets {
		ids[b.Id] = true
	}
	if err := s.StreamObjects(keep, func(o *Object) error {
		if ids[o.BucketId] {
			objects = append(objects, o)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return buckets, objects
}

func nameOf(buckets []*Bucket, o *Object) string {
	for _, b := range buckets {
		if b.Id == o.BucketId {
			return b.Name
		}
	}
	return ""
}

func TestBackupRoundTrip(t *testing.T) {
	src := newStore(t)
	mustCreate(t, src, alice, "configs")
	mustCreate(t, src, bob, "logs")
	mustCreate(t, src, bob, "gone")
	put(t, src, alice, "configs", "a.txt", "alpha")
	put(t, src, alice, "configs", "dir/b.txt", "beta")
	put(t, src, alice, "configs", "same.txt", "alpha") // shares content with a.txt
	put(t, src, alice, "configs", "old.txt", "old")
	put(t, src, alice, "configs", "big.bin", strings.Repeat("B", inlineMax+9))
	if err := src.DeleteObject(alice, "configs", "old.txt"); err != nil {
		t.Fatal(err)
	}
	put(t, src, bob, "logs", "x.log", "log line")
	put(t, src, bob, "gone", "y", "y")
	if err := src.DeleteBucket(bob, "gone", true); err != nil {
		t.Fatal(err)
	}
	src.SetGrant(alice, "configs", Grant{Type: GrantUser, Id: "bob", Access: GrantRead})

	buckets, objects := export(t, src, nil)
	if len(buckets) != 2 || len(objects) != 5 {
		t.Fatalf("backup holds %d buckets and %d files, want 2 and 5", len(buckets), len(objects))
	}
	for _, o := range objects {
		if o.Key == "old.txt" || nameOf(buckets, o) == "gone" {
			t.Errorf("deleted file %s/%s backed up", nameOf(buckets, o), o.Key)
		}
	}

	dst := newStore(t)
	if n, err := dst.RestoreBuckets(buckets); n != 2 || err != nil {
		t.Errorf("restore changed %d buckets: %v", n, err)
	}
	if n, err := dst.RestoreObjects(objects); n != 5 || err != nil {
		t.Errorf("restore changed %d files: %v", n, err)
	}
	// Restoring again changes nothing.
	if n, _ := dst.RestoreBuckets(buckets); n != 0 {
		t.Errorf("second restore changed %d buckets", n)
	}
	if n, _ := dst.RestoreObjects(objects); n != 0 {
		t.Errorf("second restore changed %d files", n)
	}

	// Until the content is copied the files are listed but cannot be read.
	missing := dst.MissingContent([]string{objects[0].SHA256, objects[1].SHA256, objects[2].SHA256, objects[3].SHA256, objects[4].SHA256})
	if len(missing) == 0 {
		t.Fatal("content held before it was copied")
	}
	for _, o := range objects {
		if dst.HasContent(o.SHA256) {
			continue
		}
		c, err := src.ReadContent(context.Background(), o.SHA256)
		if err != nil {
			t.Fatal(err)
		}
		err = dst.ImportContent(c, o.SHA256)
		c.Close()
		if err != nil {
			t.Fatalf("import %s/%s: %v", nameOf(buckets, o), o.Key, err)
		}
	}
	if got := dst.MissingContent([]string{objects[0].SHA256, objects[4].SHA256}); len(got) != 0 {
		t.Errorf("still missing %v", got)
	}
	for _, c := range []struct {
		p              *Principal
		bucket, key, v string
	}{{alice, "configs", "dir/b.txt", "beta"}, {alice, "configs", "same.txt", "alpha"}, {bob, "logs", "x.log", "log line"}, {bob, "configs", "a.txt", "alpha"}} {
		if got := read(t, dst, c.p, c.bucket, c.key); got != c.v {
			t.Errorf("restored %s/%s = %q", c.bucket, c.key, got)
		}
	}
	if got := read(t, dst, alice, "configs", "big.bin"); len(got) != inlineMax+9 {
		t.Errorf("restored large file is %d bytes", len(got))
	}
	if _, _, err := dst.OpenObject(t.Context(), alice, "configs", "old.txt"); err == nil {
		t.Error("deleted file restored")
	}
	dst.missingMu.Lock()
	queued := len(dst.missing)
	dst.missingMu.Unlock()
	if queued != 0 {
		t.Errorf("%d files still queued for content after import", queued)
	}
	clean(t, dst, "restored store")

	// A backup limited to one owner holds just their buckets.
	owned, ownedObjects := export(t, src, func(b *Bucket) bool { return b.OwnerId == "bob" })
	if len(owned) != 1 || owned[0].Name != "logs" || len(ownedObjects) != 1 {
		t.Errorf("filtered backup holds %d buckets, %d files", len(owned), len(ownedObjects))
	}
}

func TestRestoreKeepsNewer(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "configs")
	put(t, s, alice, "configs", "a.txt", "v1")
	buckets, objects := export(t, s, nil)
	put(t, s, alice, "configs", "a.txt", "v2")
	s.RestoreBuckets(buckets)
	s.RestoreObjects(objects)
	if got := read(t, s, alice, "configs", "a.txt"); got != "v2" {
		t.Errorf("restore replaced a newer version: %q", got)
	}
}

func TestRestorePersists(t *testing.T) {
	src := newStore(t)
	mustCreate(t, src, alice, "configs")
	put(t, src, alice, "configs", "a.txt", "alpha")
	buckets, objects := export(t, src, nil)

	dir := t.TempDir()
	dst := openStore(t, dir, nil)
	dst.RestoreBuckets(buckets)
	dst.RestoreObjects(objects)
	f, _ := src.ReadContent(context.Background(), objects[0].SHA256)
	if err := dst.ImportContent(f, objects[0].SHA256); err != nil {
		t.Fatal(err)
	}
	f.Close()
	dst.Close()

	// The restore survives reopening, and the imported content is kept.
	dst = openStore(t, dir, nil)
	defer dst.Close()
	if got := read(t, dst, alice, "configs", "a.txt"); got != "alpha" {
		t.Errorf("content after reopen %q", got)
	}
}

// Content a peer holds is fetched for a backup, and a backup of a bucket that
// is being written to is complete up to where it started.
func TestReadContentFetchesFromPeers(t *testing.T) {
	_, stores := newCluster(t, 2)
	a, b := stores[0], stores[1]
	mustCreate(t, a, alice, "peer")
	o := put(t, a, alice, "peer", "f", "on a")
	waitFor(t, "b has it", func() bool { return b.holds(o.SHA256) })
	b.forgetContent(o.SHA256)
	c, err := b.ReadContent(context.Background(), o.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(c)
	c.Close()
	if string(got) != "on a" || !b.holds(o.SHA256) {
		t.Errorf("content %q, held %v", got, b.holds(o.SHA256))
	}
	if _, err := a.ReadContent(context.Background(), contentSHA("nobody has this")); err == nil {
		t.Error("read content nobody holds")
	}
}

func TestImportContentChecks(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "configs")
	sum := sha256.Sum256([]byte("right"))
	sha := hex.EncodeToString(sum[:])
	if err := s.ImportContent(strings.NewReader("wrong"), sha); !errors.Is(err, ErrContentMismatch) {
		t.Errorf("mismatched content: %v", err)
	}
	if s.HasContent(sha) {
		t.Error("mismatched content stored")
	}
	if err := s.ImportContent(strings.NewReader("x"), "../../etc"); !errors.Is(err, ErrContentMismatch) {
		t.Errorf("invalid checksum: %v", err)
	}
	// Content no file refers to is not kept.
	if err := s.ImportContent(strings.NewReader("right"), sha); err != nil || s.HasContent(sha) {
		t.Errorf("import of unreferenced content: %v", err)
	}

	// Content a file refers to, but this server lacks, is kept.
	now := hlc.Now()
	s.Merge(nil, []*Object{{BucketId: bid(s, "configs"), Key: "r.txt", Size: 5, SHA256: sha, ETag: "x", UpdatedAt: now}})
	if s.HasContent(sha) {
		t.Fatal("content held before it was imported")
	}
	if err := s.ImportContent(strings.NewReader("right"), sha); err != nil || !s.HasContent(sha) {
		t.Errorf("import: %v", err)
	}
	if got := read(t, s, alice, "configs", "r.txt"); got != "right" {
		t.Errorf("imported content %q", got)
	}
}

func TestBackupWhileWriting(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "configs")

	var written atomic.Int64
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			put(t, s, alice, "configs", fmt.Sprintf("k%05d", i), "v")
			written.Add(1)
		}
	}()
	for range 50 {
		want := written.Load()
		_, objects := export(t, s, nil)
		if int64(len(objects)) < want {
			t.Errorf("backup holds %d files, %d were written before it started", len(objects), want)
			break
		}
	}
	close(stop)
	<-done
}

func TestStoreDirLocked(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir, nil)
	if _, err := Open(Config{Dir: dir}); !errors.Is(err, ErrStoreInUse) {
		t.Errorf("second open of a directory in use: %v", err)
	}
	s.Close()
	s2, err := Open(Config{Dir: dir})
	if err != nil {
		t.Fatalf("open after close: %v", err)
	}
	s2.Close()
}

func TestImportEmptyContent(t *testing.T) {
	s := newStore(t)
	sum := sha256.Sum256(nil)
	sha := hex.EncodeToString(sum[:])
	mustCreate(t, s, alice, "configs")
	put(t, s, alice, "configs", "empty", "")
	if !s.HasContent(sha) {
		t.Error("empty content not held")
	}
	if err := s.ImportContent(bytes.NewReader(nil), sha); err != nil || !s.HasContent(sha) {
		t.Errorf("import empty: %v", err)
	}
}
