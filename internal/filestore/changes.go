package filestore

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	badger "github.com/dgraph-io/badger/v4"
)

// The change feed lists what changed in a bucket since a cursor, so a client
// following a bucket fetches only what changed rather than listing it again.
//
// Each record applied on this server is given the next number of a local
// sequence, and indexed under it:
//
//	u<id><seq>                      key of the record changed at seq
//	v<id><key>                      seq the record was last changed at
//	h<id>                           highest seq of the bucket's records forgotten
//	zseq                            the sequence, saved at a clean close
//	zinst                           this database's change feed instance
//
// The sequence is local, not the records' HLC: a record gossiped from another
// server can arrive after records with later timestamps, and a cursor of
// timestamps would pass it by. A cursor names the instance it was issued by,
// so after the index is rebuilt (an unclean stop, or another server behind a
// load balancer) a client is told to start again rather than missing changes.
//
// Content whose record is deleted or replaced is held for a while, so a file
// that was renamed or changed back can be written again without its content
// being sent:
//
//	d<id><sha>                      content released in a bucket: expiry, size, etag
//	e<expiry><id><sha>              index of the above, soonest first
//
// The hold is a reference like a record's, so the content stays until the
// hold expires. It is per bucket, so content can only be claimed again in the
// bucket that held it.

// contentGrace is how long content released by a delete or overwrite is held
// for reuse; zero holds nothing. A variable for tests.
var contentGrace = time.Hour

const (
	// reuseMargin is how long a hold must still have to run for the content
	// to be claimed, so it is never swept from under a write that claimed it.
	reuseMargin = 5 * time.Minute

	// MaxChanges is the most changes returned at once.
	MaxChanges = 1000
)

var (
	seqKey  = []byte("zseq")
	instKey = []byte("zinst")

	// ErrContentNotHeld is a reuse of content the bucket does not hold: the
	// client sends the content instead.
	ErrContentNotHeld = errors.New("the content is not held for reuse in this bucket: send it")
	// ErrInvalidCursor is a cursor this store did not issue.
	ErrInvalidCursor = errors.New("invalid change cursor")
)

func changeKey(bucketId string, seq uint64) []byte {
	return binary.BigEndian.AppendUint64(append([]byte{'u'}, rawId(bucketId)...), seq)
}

func changeSeqKey(bucketId, key string) []byte {
	return append(append([]byte{'v'}, rawId(bucketId)...), key...)
}

func horizonKey(bucketId string) []byte { return append([]byte{'h'}, rawId(bucketId)...) }

func holdKey(bucketId, sha string) []byte {
	return append(append([]byte{'d'}, rawId(bucketId)...), rawSha(sha)...)
}

func holdIndexKey(expiry int64, bucketId, sha string) []byte {
	k := binary.BigEndian.AppendUint64([]byte{'e'}, uint64(expiry))
	return append(append(k, rawId(bucketId)...), rawSha(sha)...)
}

func getUint64(txn *badger.Txn, key []byte) (uint64, bool, error) {
	item, err := txn.Get(key)
	if errors.Is(err, badger.ErrKeyNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	var n uint64
	err = item.Value(func(v []byte) error {
		if len(v) >= 8 {
			n = binary.BigEndian.Uint64(v)
		}
		return nil
	})
	return n, true, err
}

func putUint64(txn *badger.Txn, key []byte, n uint64) error {
	return txn.Set(key, binary.BigEndian.AppendUint64(nil, n))
}

// changeLog numbers changes. A number is handed out before the transaction
// that uses it and is pending until that transaction ends, so a reader never
// passes a number whose change may still commit.
type changeLog struct {
	mu       sync.Mutex
	next     uint64 // last number handed out
	pending  map[uint64]int
	instance string
}

func (c *changeLog) alloc(n int) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pending == nil {
		c.pending = make(map[uint64]int)
	}
	first := c.next + 1
	c.next += uint64(n)
	c.pending[first] = n
	return first
}

func (c *changeLog) done(first uint64) {
	c.mu.Lock()
	delete(c.pending, first)
	c.mu.Unlock()
}

// watermark is the highest number below which every change has committed.
func (c *changeLog) watermark() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	w := c.next
	for first := range c.pending {
		if first-1 < w {
			w = first - 1
		}
	}
	return w
}

func (c *changeLog) last() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.next
}

// openChanges loads the change sequence, rebuilding the index when it may not
// match the records: on a database from before the feed, or after an unclean
// stop, when the saved sequence may be behind the index.
func (s *Store) openChanges(clean bool) error {
	var inst string
	var seq uint64
	var haveSeq bool
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(instKey)
		if err == nil {
			err = item.Value(func(v []byte) error { inst = string(v); return nil })
		}
		if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		seq, haveSeq, err = getUint64(txn, seqKey)
		return err
	})
	if err != nil {
		return err
	}
	if clean && inst != "" && haveSeq {
		s.changes.instance, s.changes.next = inst, seq
		// From here an unclean exit must rebuild the index.
		return s.db.Update(func(txn *badger.Txn) error { return txn.Delete(seqKey) })
	}
	return s.rebuildChanges()
}

// rebuildChanges indexes every record afresh under a new instance, so cursors
// issued before are refused.
func (s *Store) rebuildChanges() error {
	for _, prefix := range []byte{'u', 'v', 'h'} {
		if err := s.db.DropPrefix([]byte{prefix}); err != nil {
			return err
		}
	}
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return err
	}
	inst := hex.EncodeToString(raw[:])

	var seq uint64
	txn := s.db.NewTransaction(true)
	defer func() { txn.Discard() }()
	count := 0
	err := s.db.View(func(view *badger.Txn) error {
		it := view.NewIterator(badger.IteratorOptions{Prefix: []byte{'o'}})
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			k := it.Item().Key()
			if len(k) < 17 {
				continue
			}
			bucketId, key := idFromRaw(k[1:17]), string(k[17:])
			seq++
			for _, set := range [][2][]byte{
				{changeKey(bucketId, seq), []byte(key)},
				{changeSeqKey(bucketId, key), binary.BigEndian.AppendUint64(nil, seq)},
			} {
				if err := txn.Set(set[0], set[1]); errors.Is(err, badger.ErrTxnTooBig) {
					if err := txn.Commit(); err != nil {
						return err
					}
					txn = s.db.NewTransaction(true)
					if err := txn.Set(set[0], set[1]); err != nil {
						return err
					}
				} else if err != nil {
					return err
				}
			}
			count++
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := txn.Set(instKey, []byte(inst)); err != nil {
		return err
	}
	if err := txn.Commit(); err != nil {
		return err
	}
	s.changes.instance, s.changes.next = inst, seq
	if count > 0 {
		s.logger.Info("indexed file changes", "records", count)
	}
	return nil
}

// saveChanges records the sequence at a clean close.
func (s *Store) saveChanges(txn *badger.Txn) error {
	return putUint64(txn, seqKey, s.changes.last())
}

// indexChange records in txn that a record changed at seq.
func indexChange(txn *badger.Txn, bucketId, key string, seq uint64) error {
	prev, ok, err := getUint64(txn, changeSeqKey(bucketId, key))
	if err != nil {
		return err
	}
	if ok {
		if err := txn.Delete(changeKey(bucketId, prev)); err != nil {
			return err
		}
	}
	if err := txn.Set(changeKey(bucketId, seq), []byte(key)); err != nil {
		return err
	}
	return putUint64(txn, changeSeqKey(bucketId, key), seq)
}

// unindexChange forgets a record that is removed from the database. A client
// that has not seen it yet can no longer be told of it, so the bucket's
// horizon moves past it.
func unindexChange(txn *badger.Txn, bucketId, key string) error {
	prev, ok, err := getUint64(txn, changeSeqKey(bucketId, key))
	if err != nil || !ok {
		return err
	}
	if err := txn.Delete(changeKey(bucketId, prev)); err != nil {
		return err
	}
	if err := txn.Delete(changeSeqKey(bucketId, key)); err != nil {
		return err
	}
	horizon, _, err := getUint64(txn, horizonKey(bucketId))
	if err != nil {
		return err
	}
	if prev > horizon {
		return putUint64(txn, horizonKey(bucketId), prev)
	}
	return nil
}

// ChangeList is a page of the change feed.
type ChangeList struct {
	Changes []*Object
	// Cursor continues the feed after these changes.
	Cursor string
	// More is set when more changes are waiting: ask again at once.
	More bool
	// Reset is set when the cursor can no longer be followed (the index was
	// rebuilt, the cursor came from another server, or deletions it had not
	// seen were forgotten): start again without a cursor.
	Reset bool
}

func (s *Store) cursor(seq uint64) string {
	return s.changes.instance + "." + strconv.FormatUint(seq, 10)
}

// parseCursor returns the sequence of a cursor issued by this store; ok is
// false for one issued by another instance.
func (s *Store) parseCursor(c string) (seq uint64, ok bool, err error) {
	inst, num, found := strings.Cut(c, ".")
	if !found {
		return 0, false, ErrInvalidCursor
	}
	seq, err = strconv.ParseUint(num, 10, 64)
	if err != nil {
		return 0, false, ErrInvalidCursor
	}
	return seq, inst == s.changes.instance, nil
}

// CursorNow asks ListChanges for no changes, only a cursor to follow the
// bucket from now on.
const CursorNow = "now"

// ListChanges returns what changed under prefix in a bucket since cursor, in
// the order the changes were made here, each file once, as it is now. With no
// cursor it returns every file, without deletions, and a cursor to follow on
// from; with CursorNow, only a cursor to follow on from.
func (s *Store) ListChanges(p *Principal, bucket, prefix, cursor string, max int) (*ChangeList, error) {
	if max <= 0 || max > MaxChanges {
		max = MaxChanges
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, err := s.bucketForLocked(p, bucket, AccessRead)
	if err != nil {
		return nil, err
	}

	var since uint64
	if cursor != "" && cursor != CursorNow {
		seq, ok, err := s.parseCursor(cursor)
		if err != nil {
			return nil, err
		}
		if !ok {
			return &ChangeList{Changes: []*Object{}, Reset: true}, nil
		}
		since = seq
	}
	upTo := s.changes.watermark()
	if cursor == CursorNow {
		return &ChangeList{Changes: []*Object{}, Cursor: s.cursor(upTo)}, nil
	}
	if since > s.changes.last() {
		// Issued by this instance, but ahead of it: never handed out.
		return nil, ErrInvalidCursor
	}

	res := &ChangeList{Changes: []*Object{}}
	last := since
	err = s.db.View(func(txn *badger.Txn) error {
		if cursor != "" {
			horizon, _, err := getUint64(txn, horizonKey(b.Id))
			if err != nil {
				return err
			}
			if horizon > since {
				res.Reset = true
				return nil
			}
		}
		pfx := append([]byte{'u'}, rawId(b.Id)...)
		it := txn.NewIterator(badger.IteratorOptions{PrefetchValues: true, PrefetchSize: 100, Prefix: pfx})
		defer it.Close()
		for it.Seek(changeKey(b.Id, since+1)); it.Valid(); it.Next() {
			seq := binary.BigEndian.Uint64(it.Item().Key()[len(pfx):])
			if seq > upTo {
				break
			}
			if len(res.Changes) == max {
				res.More = true
				break
			}
			last = seq
			var key string
			if err := it.Item().Value(func(v []byte) error { key = string(v); return nil }); err != nil {
				return err
			}
			if !strings.HasPrefix(key, prefix) {
				continue
			}
			o, err := getObject(txn, b.Id, key)
			if err != nil {
				return err
			}
			if o == nil || (cursor == "" && o.IsDeleted) {
				continue
			}
			res.Changes = append(res.Changes, o.clone())
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if res.Reset {
		return &ChangeList{Changes: []*Object{}, Reset: true}, nil
	}
	if !res.More {
		last = max64(last, upTo)
	}
	res.Cursor = s.cursor(last)
	return res, nil
}

func max64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

// ---------------------------------------------------------------------------
// Holding released content
// ---------------------------------------------------------------------------

type hold struct {
	expiry int64
	size   int64
	etag   string
}

func getHold(txn *badger.Txn, bucketId, sha string) (*hold, error) {
	item, err := txn.Get(holdKey(bucketId, sha))
	if errors.Is(err, badger.ErrKeyNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var h *hold
	err = item.Value(func(v []byte) error {
		if len(v) < 16 {
			return fmt.Errorf("short content hold")
		}
		h = &hold{
			expiry: int64(binary.BigEndian.Uint64(v[:8])),
			size:   int64(binary.BigEndian.Uint64(v[8:16])),
			etag:   string(v[16:]),
		}
		return nil
	})
	return h, err
}

func setHold(txn *badger.Txn, bucketId, sha string, h *hold) error {
	v := binary.BigEndian.AppendUint64(nil, uint64(h.expiry))
	v = binary.BigEndian.AppendUint64(v, uint64(h.size))
	v = append(v, h.etag...)
	if err := txn.Set(holdKey(bucketId, sha), v); err != nil {
		return err
	}
	return txn.Set(holdIndexKey(h.expiry, bucketId, sha), nil)
}

// holdContent keeps the content of a record that is going, for reuse in its
// bucket. The hold takes a reference, so it must be taken before the record's
// own is released. Content this server does not hold is not held.
func (s *Store) holdContent(txn *badger.Txn, o *Object) error {
	if o.SHA256 == "" || contentGrace <= 0 {
		return nil
	}
	expiry := time.Now().Add(contentGrace).UnixNano()
	h, err := getHold(txn, o.BucketId, o.SHA256)
	if err != nil {
		return err
	}
	if h != nil {
		// Held already: run the hold on from now.
		if err := txn.Delete(holdIndexKey(h.expiry, o.BucketId, o.SHA256)); err != nil {
			return err
		}
		h.expiry = expiry
		return setHold(txn, o.BucketId, o.SHA256, h)
	}
	inline, err := hasKey(txn, shaKey('c', o.SHA256))
	if err != nil {
		return err
	}
	if !inline && !s.blobs.has(o.SHA256) {
		return nil
	}
	refs, err := getRef(txn, o.SHA256)
	if err != nil {
		return err
	}
	if err := setRef(txn, o.SHA256, refs+1); err != nil {
		return err
	}
	return setHold(txn, o.BucketId, o.SHA256, &hold{expiry: expiry, size: o.Size, etag: o.ETag})
}

// expireHolds releases the holds that have run out.
func (s *Store) expireHolds() {
	now := uint64(time.Now().UnixNano())
	for {
		var keys [][]byte
		s.db.View(func(txn *badger.Txn) error {
			it := txn.NewIterator(badger.IteratorOptions{Prefix: []byte{'e'}})
			defer it.Close()
			for it.Rewind(); it.Valid() && len(keys) < writeChunk; it.Next() {
				k := it.Item().KeyCopy(nil)
				if len(k) < 9 || beUint64(k[1:9]) > now {
					break
				}
				keys = append(keys, k)
			}
			return nil
		})
		if len(keys) == 0 {
			return
		}
		var released []string
		err := s.db.Update(func(txn *badger.Txn) error {
			released = released[:0]
			for _, k := range keys {
				if err := txn.Delete(k); err != nil {
					return err
				}
				if len(k) != 1+8+16+32 {
					continue
				}
				bucketId, sha := idFromRaw(k[9:25]), hex.EncodeToString(k[25:])
				h, err := getHold(txn, bucketId, sha)
				if err != nil {
					return err
				}
				if h == nil || uint64(h.expiry) != beUint64(k[1:9]) {
					continue // renewed, or already gone
				}
				if err := txn.Delete(holdKey(bucketId, sha)); err != nil {
					return err
				}
				gone, err := s.release(txn, sha)
				if err != nil {
					return err
				}
				if gone {
					released = append(released, sha)
				}
			}
			return nil
		})
		if err != nil {
			s.logger.Error("failed to release held content", "error", err)
			return
		}
		if len(released) > 0 {
			s.missingMu.Lock()
			for _, sha := range released {
				delete(s.missing, sha)
			}
			s.missingMu.Unlock()
			s.unlinkMu.Lock()
			s.unlink = append(s.unlink, released...)
			s.unlinkMu.Unlock()
			s.flushUnlinks()
		}
	}
}

// countHolds adds the references holds take to refs, by raw sha, for fsck.
func countHolds(txn *badger.Txn, add func(rawSha string)) {
	it := txn.NewIterator(badger.IteratorOptions{Prefix: []byte{'d'}})
	defer it.Close()
	for it.Rewind(); it.Valid(); it.Next() {
		k := it.Item().Key()
		if len(k) == 1+16+32 {
			add(string(k[17:]))
		}
	}
}

// ReuseObject writes a file whose content this bucket held until recently,
// a file renamed or changed back, without the content being sent. It fails
// with ErrContentNotHeld when the content is not held, or not for long enough
// to be sure of it: the client then sends it.
func (s *Store) ReuseObject(p *Principal, bucket, key, sha string, opts PutOptions) (*Object, error) {
	if !ValidKey(key) {
		return nil, ErrInvalidKey
	}
	if !validSHA(sha) {
		return nil, ErrContentMismatch
	}
	if err := checkMeta(opts.ContentType, opts.Meta); err != nil {
		return nil, err
	}
	_, limit, err := s.quotaFor(p, bucket, AccessWrite)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	b, err := s.bucketForLocked(p, bucket, AccessWrite)
	if err != nil {
		return nil, err
	}
	var h *hold
	if err := s.db.View(func(txn *badger.Txn) (err error) { h, err = getHold(txn, b.Id, sha); return }); err != nil {
		return nil, err
	}
	if h == nil || time.Until(time.Unix(0, h.expiry)) < reuseMargin {
		return nil, ErrContentNotHeld
	}
	release, err := s.reserveLocked(b.OwnerId, limit, h.size-s.existingSizeLocked(b.Id, key))
	if err != nil {
		return nil, err
	}
	defer release()

	o := s.newObjectLocked(b, key, sha, h.etag, h.size, opts)
	changed, err := s.applyObjects([]objectOp{{
		obj: o,
		pre: func(txn *badger.Txn, cur *Object) error {
			// Still held: the hold's reference keeps the content until the
			// new record has taken its own.
			now, err := getHold(txn, b.Id, sha)
			if err != nil {
				return err
			}
			if now == nil {
				return ErrContentNotHeld
			}
			if !s.liveLocked(cur) {
				cur = nil
			}
			return checkPreconditions(cur, opts)
		},
	}})
	if err != nil {
		return nil, err
	}
	if len(changed) == 0 {
		return o.clone(), nil
	}
	s.commit(nil, []*Object{o})
	return o.clone(), nil
}
