package filestore

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/paularlott/knot/internal/log"
)

// BackupData is the file storage part of a backup: every live bucket and
// the live objects in them, with their content addressed by sha256.
type BackupData struct {
	Buckets []*Bucket `json:"buckets"`
	Objects []*Object `json:"objects"`
}

// ReadBackup reads the metadata held in a storage directory. It reads the
// snapshot and journal without opening the directory as a store, so it is
// safe while a server is using it: should the server compact its journal
// during the read, the read starts again.
func ReadBackup(dir string) (*BackupData, error) {
	metaDir := filepath.Join(dir, "meta")
	if _, err := os.Stat(metaDir); err != nil {
		return nil, errors.New("no file storage found in " + dir)
	}
	snapshot := filepath.Join(metaDir, "snapshot.jsonl")
	for attempt := 0; ; attempt++ {
		before, _ := os.Stat(snapshot)
		d, err := readBackup(dir)
		if err != nil {
			return nil, err
		}
		after, _ := os.Stat(snapshot)
		if before == nil || after == nil || os.SameFile(before, after) || attempt == 9 {
			return d, nil
		}
	}
}

func readBackup(dir string) (*BackupData, error) {
	s := &Store{
		dir:      dir,
		logger:   log.WithGroup("files"),
		buckets:  make(map[string]*Bucket),
		objects:  make(map[string]map[string]*Object),
		stats:    make(map[string]*bucketStats),
		owned:    make(map[string]map[string]struct{}),
		blobRefs: make(map[string]int),
		missing:  make(map[string]*Object),
		blobs:    &blobStore{dir: filepath.Join(dir, "blobs"), tmpDir: filepath.Join(dir, "tmp")},
	}
	for _, name := range []string{"snapshot.jsonl", "journal.jsonl"} {
		if err := s.loadFile(filepath.Join(dir, "meta", name)); err != nil {
			return nil, err
		}
	}

	d := &BackupData{}
	for _, b := range s.buckets {
		if !live(b) {
			continue
		}
		d.Buckets = append(d.Buckets, b)
		for _, o := range s.objects[b.Name] {
			if s.liveLocked(o) {
				d.Objects = append(d.Objects, o)
			}
		}
	}
	return d, nil
}

// OpenContent opens the content of sha held in a storage directory, for
// reading while a server may be using it.
func OpenContent(dir, sha string) (*os.File, error) {
	bs := &blobStore{dir: filepath.Join(dir, "blobs")}
	return bs.open(sha)
}

// Restore merges backed up records into the store. They keep their
// timestamps, so where the store, or another server, holds a newer version
// of a bucket or file, that version wins. It returns the number of buckets
// and files that changed.
func (s *Store) Restore(d *BackupData) (int, int) {
	var changedB []*Bucket
	var changedO []*Object
	s.mu.Lock()
	for _, b := range d.Buckets {
		if b != nil && ValidBucketName(b.Name) && s.applyBucketLocked(b) {
			changedB = append(changedB, b)
		}
	}
	for _, o := range d.Objects {
		if o == nil || !ValidBucketName(o.Bucket) || !ValidKey(o.Key) || (!o.IsDeleted && !validSHA(o.SHA256)) {
			continue
		}
		if s.applyObjectLocked(o) {
			changedO = append(changedO, o)
		}
	}
	s.mu.Unlock()
	s.journalAppend(changedB, changedO, true)
	s.flushUnlinks()
	return len(changedB), len(changedO)
}

// ImportContent stores the content of sha read from r, refusing content
// that does not match it.
func (s *Store) ImportContent(r io.Reader, sha string) error {
	if !validSHA(sha) {
		return ErrContentMismatch
	}
	tw, err := s.blobs.writeTemp(r, -1, -1, sha, nil)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.blobs.install(tw.Path(), sha); err != nil {
		tw.discard()
		return err
	}
	s.contentHeldLocked(sha)
	return nil
}

// HasContent reports whether the content of sha is stored here.
func (s *Store) HasContent(sha string) bool {
	return s.blobs.has(sha)
}
