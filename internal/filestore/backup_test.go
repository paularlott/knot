package filestore

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
)

func TestBackupRoundTrip(t *testing.T) {
	src := newStore(t)
	mustCreate(t, src, alice, "configs")
	mustCreate(t, src, bob, "logs")
	mustCreate(t, src, bob, "gone")
	put(t, src, alice, "configs", "a.txt", "alpha")
	put(t, src, alice, "configs", "dir/b.txt", "beta")
	put(t, src, alice, "configs", "same.txt", "alpha") // shares content with a.txt
	put(t, src, alice, "configs", "old.txt", "old")
	if err := src.DeleteObject(alice, "configs", "old.txt"); err != nil {
		t.Fatal(err)
	}
	put(t, src, bob, "logs", "x.log", "log line")
	put(t, src, bob, "gone", "y", "y")
	if err := src.DeleteBucket(bob, "gone", true); err != nil {
		t.Fatal(err)
	}

	// Read while the source store is open, as a backup of a running server.
	d, err := ReadBackup(src.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Buckets) != 2 || len(d.Objects) != 4 {
		t.Fatalf("backup holds %d buckets and %d files, want 2 and 4", len(d.Buckets), len(d.Objects))
	}
	for _, o := range d.Objects {
		if o.Key == "old.txt" || o.Bucket == "gone" {
			t.Errorf("deleted file %s/%s backed up", o.Bucket, o.Key)
		}
	}

	dst := newStore(t)
	if nb, no := dst.Restore(d); nb != 2 || no != 4 {
		t.Errorf("restore changed %d buckets and %d files", nb, no)
	}
	// Restoring again changes nothing.
	if nb, no := dst.Restore(d); nb != 0 || no != 0 {
		t.Errorf("second restore changed %d buckets and %d files", nb, no)
	}

	for _, o := range d.Objects {
		if dst.HasContent(o.SHA256) {
			continue
		}
		f, err := OpenContent(src.dir, o.SHA256)
		if err != nil {
			t.Fatal(err)
		}
		err = dst.ImportContent(f, o.SHA256)
		f.Close()
		if err != nil {
			t.Fatalf("import %s/%s: %v", o.Bucket, o.Key, err)
		}
	}
	if got := read(t, dst, alice, "configs", "dir/b.txt"); got != "beta" {
		t.Errorf("restored content %q", got)
	}
	if got := read(t, dst, alice, "configs", "same.txt"); got != "alpha" {
		t.Errorf("restored content %q", got)
	}
	if got := read(t, dst, bob, "logs", "x.log"); got != "log line" {
		t.Errorf("restored content %q", got)
	}
	if _, _, err := dst.OpenObject(t.Context(), alice, "configs", "old.txt"); err == nil {
		t.Error("deleted file restored")
	}
	dst.mu.RLock()
	missing := len(dst.missing)
	dst.mu.RUnlock()
	if missing != 0 {
		t.Errorf("%d files still missing content after import", missing)
	}
}

func TestRestoreKeepsNewer(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "configs")
	put(t, s, alice, "configs", "a.txt", "v1")
	d, err := ReadBackup(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	put(t, s, alice, "configs", "a.txt", "v2")
	s.Restore(d)
	if got := read(t, s, alice, "configs", "a.txt"); got != "v2" {
		t.Errorf("restore replaced a newer version: %q", got)
	}
}

func TestRestorePersists(t *testing.T) {
	src := newStore(t)
	mustCreate(t, src, alice, "configs")
	put(t, src, alice, "configs", "a.txt", "alpha")
	d, _ := ReadBackup(src.dir)

	dir := t.TempDir()
	dst := openStore(t, dir, nil)
	dst.Restore(d)
	f, _ := OpenContent(src.dir, d.Objects[0].SHA256)
	if err := dst.ImportContent(f, d.Objects[0].SHA256); err != nil {
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

func TestImportContentChecks(t *testing.T) {
	s := newStore(t)
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
	if err := s.ImportContent(strings.NewReader("right"), sha); err != nil || !s.HasContent(sha) {
		t.Errorf("import: %v", err)
	}
	// Unreferenced content is removed by the next cleanup, as any other.
	s.cleanup()
	if s.HasContent(sha) {
		t.Error("unreferenced content kept")
	}
}

func TestReadBackupNoStorage(t *testing.T) {
	if _, err := ReadBackup(t.TempDir()); err == nil {
		t.Error("read a backup of an empty directory")
	}
}

func TestReadBackupDuringCompaction(t *testing.T) {
	s := newStore(t)
	mustCreate(t, s, alice, "configs")

	// Keep writing, compacting after each write, so a read can catch the
	// snapshot from before a compaction and the journal from after it.
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
			if i%2 == 0 {
				s.compact()
			}
		}
	}()
	for range 200 {
		want := written.Load()
		d, err := ReadBackup(s.dir)
		if err != nil {
			t.Fatal(err)
		}
		if int64(len(d.Objects)) < want {
			t.Errorf("backup holds %d files, %d were written before it started", len(d.Objects), want)
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
	if err := s.ImportContent(bytes.NewReader(nil), sha); err != nil || !s.HasContent(sha) {
		t.Errorf("import empty: %v", err)
	}
}
