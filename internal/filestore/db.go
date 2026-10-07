package filestore

import (
	"encoding/binary"
	"encoding/hex"
	"errors"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/badger/v4/options"
	"github.com/google/uuid"
	"github.com/paularlott/gossip/hlc"
	"github.com/shamaton/msgpack/v3"
)

// Metadata lives in an embedded Badger database in the storage directory.
// Content of at most inlineMax bytes is stored in it beside the metadata;
// larger content is a file under blobs/. The keys are:
//
//	b<id>                           bucket record
//	o<id><key>                      object record, tombstones included
//	r<sha>                          number of records referencing content
//	c<sha>                          content stored inline
//	m<sha>                          content referenced but not held, with the record
//	t<updated><id><key>             index of tombstones, oldest first
//	q<id>                           deleted bucket with records left to sweep
//	s<id>                           bucket statistics saved at a clean close
//	zclean                          present when the statistics are current
//
// A bucket id is its UUID as 16 raw bytes, and a sha is 32 raw bytes, so the
// prefix of an object key is fixed in length.
//
// Records are ordered by key, so listing a folder is a scan of the keys with
// its prefix.
const (
	// inlineMax is the largest content stored in the database.
	inlineMax = 64 << 10

	// writeChunk is how many records one transaction changes at most.
	writeChunk = 500
)

var cleanKey = []byte("zclean")

// rawId is a bucket id as 16 bytes.
func rawId(id string) []byte {
	u, _ := uuid.Parse(id)
	return u[:]
}

// idFromRaw is the bucket id of 16 bytes.
func idFromRaw(b []byte) string {
	u, err := uuid.FromBytes(b)
	if err != nil {
		return ""
	}
	return u.String()
}

func bucketKey(id string) []byte { return append([]byte{'b'}, rawId(id)...) }
func queueKey(id string) []byte  { return append([]byte{'q'}, rawId(id)...) }
func statsKey(id string) []byte  { return append([]byte{'s'}, rawId(id)...) }

func objectPrefix(bucketId string) []byte { return append([]byte{'o'}, rawId(bucketId)...) }

func objectKey(bucketId, key string) []byte {
	return append(objectPrefix(bucketId), key...)
}

func tombKey(ts hlc.Timestamp, bucketId, key string) []byte {
	k := binary.BigEndian.AppendUint64([]byte{'t'}, uint64(ts))
	return append(append(k, rawId(bucketId)...), key...)
}

// shaKey is the key of kind ('r', 'c' or 'm') for a content checksum.
func shaKey(kind byte, sha string) []byte {
	raw, _ := hex.DecodeString(sha)
	return append([]byte{kind}, raw...)
}

func encodeRecord(v any) []byte {
	data, _ := msgpack.Marshal(v)
	return data
}

func decodeObject(data []byte) (*Object, error) {
	o := &Object{}
	if err := msgpack.Unmarshal(data, o); err != nil {
		return nil, err
	}
	return o, nil
}

func decodeBucket(data []byte) (*Bucket, error) {
	b := &Bucket{}
	if err := msgpack.Unmarshal(data, b); err != nil {
		return nil, err
	}
	return b, nil
}

func openDB(dir string, noSync bool) (*badger.DB, error) {
	opts := badger.DefaultOptions(dir).
		WithLogger(nil).
		WithSyncWrites(!noSync).
		WithCompression(options.None).
		WithBlockCacheSize(128 << 20).
		WithValueLogFileSize(256 << 20).
		// Records stay in the tree; inline content goes to the value log.
		WithValueThreshold(1 << 10)
	return badger.Open(opts)
}

// getObject reads an object record, nil when there is none.
func getObject(txn *badger.Txn, bucketId, key string) (*Object, error) {
	item, err := txn.Get(objectKey(bucketId, key))
	if errors.Is(err, badger.ErrKeyNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var o *Object
	err = item.Value(func(v []byte) error {
		var derr error
		o, derr = decodeObject(v)
		return derr
	})
	return o, err
}

func hasKey(txn *badger.Txn, key []byte) (bool, error) {
	_, err := txn.Get(key)
	if errors.Is(err, badger.ErrKeyNotFound) {
		return false, nil
	}
	return err == nil, err
}

func getRef(txn *badger.Txn, sha string) (uint32, error) {
	item, err := txn.Get(shaKey('r', sha))
	if errors.Is(err, badger.ErrKeyNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var n uint32
	err = item.Value(func(v []byte) error { n = binary.LittleEndian.Uint32(v); return nil })
	return n, err
}

func setRef(txn *badger.Txn, sha string, n uint32) error {
	var v [4]byte
	binary.LittleEndian.PutUint32(v[:], n)
	return txn.Set(shaKey('r', sha), v[:])
}

// refCount is the number of records referencing sha.
func (s *Store) refCount(sha string) uint32 {
	var n uint32
	s.db.View(func(txn *badger.Txn) error {
		n, _ = getRef(txn, sha)
		return nil
	})
	return n
}

// ---------------------------------------------------------------------------
// Loading
// ---------------------------------------------------------------------------

// persistedStats is a bucket's statistics as saved at a clean close.
type persistedStats struct {
	Size       int64               `msgpack:"size"`
	Count      int                 `msgpack:"count"`
	Records    int                 `msgpack:"records"`
	Digest     uint64              `msgpack:"digest"`
	Slots      [DigestSlots]uint64 `msgpack:"slots"`
	SlotCounts [DigestSlots]int32  `msgpack:"slot_counts"`
}

// loadState reads the small parts of the database into memory: the buckets,
// content still to fetch, buckets with records to sweep, and the statistics
// of each bucket, recounted from the records if the last shutdown was not
// clean.
func (s *Store) loadState() error {
	var queued []string
	clean := false
	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.IteratorOptions{PrefetchValues: true, PrefetchSize: 100, Prefix: []byte{'b'}})
		for it.Rewind(); it.Valid(); it.Next() {
			var b *Bucket
			if err := it.Item().Value(func(v []byte) (err error) { b, err = decodeBucket(v); return }); err != nil {
				it.Close()
				return err
			}
			s.buckets[b.Id] = b
			s.indexLocked(nil, b)
		}
		it.Close()

		it = txn.NewIterator(badger.IteratorOptions{PrefetchValues: true, PrefetchSize: 100, Prefix: []byte{'m'}})
		for it.Rewind(); it.Valid(); it.Next() {
			var o *Object
			if err := it.Item().Value(func(v []byte) (err error) { o, err = decodeObject(v); return }); err != nil {
				it.Close()
				return err
			}
			s.missing[o.SHA256] = o
		}
		it.Close()

		it = txn.NewIterator(badger.IteratorOptions{Prefix: []byte{'q'}})
		for it.Rewind(); it.Valid(); it.Next() {
			queued = append(queued, idFromRaw(it.Item().Key()[1:]))
		}
		it.Close()

		var err error
		clean, err = hasKey(txn, cleanKey)
		return err
	})
	if err != nil {
		return err
	}
	for _, name := range queued {
		s.sweepQueue[name] = struct{}{}
	}

	if clean {
		err = s.db.View(func(txn *badger.Txn) error {
			for id := range s.buckets {
				item, err := txn.Get(statsKey(id))
				if errors.Is(err, badger.ErrKeyNotFound) {
					continue
				}
				if err != nil {
					return err
				}
				var ps persistedStats
				if err := item.Value(func(v []byte) error { return msgpack.Unmarshal(v, &ps) }); err != nil {
					return err
				}
				st := s.statsFor(id)
				st.size, st.count, st.records = ps.Size, ps.Count, ps.Records
				st.digest, st.slots, st.slotCounts = ps.Digest, ps.Slots, ps.SlotCounts
			}
			return nil
		})
	} else {
		err = s.rebuildStats()
	}
	if err != nil {
		return err
	}
	// From here an unclean exit must be noticed.
	return s.db.Update(func(txn *badger.Txn) error { return txn.Delete(cleanKey) })
}

// rebuildStats recounts every bucket's statistics from its records.
func (s *Store) rebuildStats() error {
	return s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.IteratorOptions{PrefetchValues: true, PrefetchSize: 1000, Prefix: []byte{'o'}})
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			var o *Object
			if err := it.Item().Value(func(v []byte) (err error) { o, err = decodeObject(v); return }); err != nil {
				return err
			}
			st := s.statsFor(o.BucketId)
			st.records++
			st.slotCounts[keySlot(o.Key)]++
			st.toggle(o)
			if s.liveLocked(o) {
				st.size += o.Size
				st.count++
			}
		}
		return nil
	})
}

// saveState records the statistics, then that they are current.
func (s *Store) saveState() error {
	return s.db.Update(func(txn *badger.Txn) error {
		s.statsMu.RLock()
		defer s.statsMu.RUnlock()
		for id, st := range s.stats {
			st.mu.Lock()
			ps := persistedStats{Size: st.size, Count: st.count, Records: st.records, Digest: st.digest, Slots: st.slots, SlotCounts: st.slotCounts}
			st.mu.Unlock()
			if err := txn.Set(statsKey(id), encodeRecord(&ps)); err != nil {
				return err
			}
		}
		return txn.Set(cleanKey, []byte{1})
	})
}

// openInline opens content kept in the database.
func openInline(txn *badger.Txn, sha string) ([]byte, bool, error) {
	item, err := txn.Get(shaKey('c', sha))
	if errors.Is(err, badger.ErrKeyNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	data, err := item.ValueCopy(nil)
	return data, err == nil, err
}
