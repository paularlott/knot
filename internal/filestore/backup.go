package filestore

import (
	"context"
	"io"
	"sort"

	badger "github.com/dgraph-io/badger/v4"
)

// A backup of file storage is a stream of bucket records and a stream of
// object records, plus the content they refer to, read through the running
// server. Restoring merges records back in: they keep their timestamps, so
// where the store, or another server, holds a newer version of a bucket or
// file, that version wins.

// liveBuckets returns the live buckets that pass keep, by id.
func (s *Store) liveBuckets(keep func(*Bucket) bool) map[string]*Bucket {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]*Bucket, len(s.buckets))
	for id, b := range s.buckets {
		if live(b) && (keep == nil || keep(b)) {
			out[id] = b.clone()
		}
	}
	return out
}

// StreamBuckets calls emit with every live bucket that passes keep (nil for
// all), in id order.
func (s *Store) StreamBuckets(keep func(*Bucket) bool, emit func(*Bucket) error) error {
	buckets := s.liveBuckets(keep)
	ids := make([]string, 0, len(buckets))
	for id := range buckets {
		ids = append(ids, id)
	}
	sortStrings(ids)
	for _, id := range ids {
		if err := emit(buckets[id]); err != nil {
			return err
		}
	}
	return nil
}

// StreamObjects calls emit with every live file in the live buckets that pass
// keep (nil for all), bucket by bucket in key order, from one consistent
// snapshot, so a large store streams without being held in memory.
func (s *Store) StreamObjects(keep func(*Bucket) bool, emit func(*Object) error) error {
	buckets := s.liveBuckets(keep)
	return s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.IteratorOptions{PrefetchValues: true, PrefetchSize: 1000, Prefix: []byte{'o'}})
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			var o *Object
			if err := it.Item().Value(func(v []byte) (err error) { o, err = decodeObject(v); return }); err != nil {
				return err
			}
			if o.IsDeleted || buckets[o.BucketId] == nil {
				continue
			}
			if err := emit(o); err != nil {
				return err
			}
		}
		return nil
	})
}

// RestoreBuckets merges backed up bucket records into the store, returning
// how many changed it.
func (s *Store) RestoreBuckets(buckets []*Bucket) (int, error) {
	var changed []*Bucket
	var firstErr error
	s.mu.Lock()
	for _, b := range buckets {
		if b == nil || !ValidBucketId(b.Id) || !ValidBucketName(b.Name) {
			continue
		}
		stored, renamed, err := s.applyBucketLocked(b)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if stored != nil {
			changed = append(changed, stored)
			changed = append(changed, renamed...)
		}
	}
	s.mu.Unlock()
	s.broadcast(changed, nil)
	return len(changed), firstErr
}

// RestoreObjects merges backed up file records into the store, returning how
// many changed it. Content is restored separately.
func (s *Store) RestoreObjects(objects []*Object) (int, error) {
	var ops []objectOp
	for _, o := range objects {
		if o == nil || !ValidBucketId(o.BucketId) || !ValidKey(o.Key) || (!o.IsDeleted && !validSHA(o.SHA256)) {
			continue
		}
		ops = append(ops, objectOp{obj: o})
	}
	s.mu.RLock()
	changed, err := s.applyObjects(ops)
	s.mu.RUnlock()
	s.noteChanged(nil, changed)
	s.flushUnlinks()
	return len(changed), err
}

// ImportContent stores the content of sha read from r, refusing content
// that does not match it. Content no record references is not kept.
func (s *Store) ImportContent(r io.Reader, sha string) error {
	if !validSHA(sha) {
		return ErrContentMismatch
	}
	st, err := s.blobs.stage(r, -1, -1, sha, nil)
	if err != nil {
		return err
	}
	if st.inline() {
		return s.storeContent(sha, st.size, st.data, "", false)
	}
	if err := s.storeContent(sha, st.size, nil, st.tw.Path(), true); err != nil {
		st.discard()
		return err
	}
	return nil
}

// HasContent reports whether the content of sha is stored here.
func (s *Store) HasContent(sha string) bool {
	return s.holds(sha)
}

// MissingContent returns which of shas this server does not hold.
func (s *Store) MissingContent(shas []string) []string {
	var out []string
	for _, sha := range shas {
		if validSHA(sha) && !s.holds(sha) {
			out = append(out, sha)
		}
	}
	return out
}

// ReadContent opens the content of sha for a backup, fetching it from
// another server first if this one does not hold it.
func (s *Store) ReadContent(ctx context.Context, sha string) (Content, error) {
	if c, err := s.openContent(sha); err == nil {
		return c, nil
	}
	// The size is not known here; anything over the inline limit goes through
	// a temporary file, which is right for content of any size.
	if err := s.fetcher.wait(ctx, sha, inlineMax+1, ""); err != nil {
		return nil, err
	}
	c, err := s.openContent(sha)
	if err != nil {
		return nil, ErrUnavailable
	}
	return c, nil
}

func sortStrings(s []string) { sort.Strings(s) }
