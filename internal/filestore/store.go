package filestore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/logger"
)

const (
	// TombstoneTTL is how long deleted buckets and objects are remembered so
	// a server returning from an outage cannot resurrect them. It matches how
	// long the databases keep deleted records (garbageMaxAge in the drivers),
	// so a server offline longer than this is out of step with the whole system.
	TombstoneTTL = 3 * 24 * time.Hour

	// DigestSlots is how many parts each bucket's digest is split into, so
	// anti-entropy repairs only the parts of a bucket that differ.
	DigestSlots = 64

	// broadcastInterval is how long local changes wait to be gossiped, so a
	// burst of writes travels in a few messages rather than one per write.
	broadcastInterval = 50 * time.Millisecond
	// broadcastBatch is the most object records sent in one gossip message;
	// with the record size limits it keeps a message well under the
	// transport's packet limit.
	broadcastBatch = 500
	// broadcastBucketBatch is the same for bucket records, which can carry
	// up to MaxGrants grants.
	broadcastBucketBatch = 100

	// unlinkBatch is how many content files are removed per hold of the lock.
	unlinkBatch = 256
)

// Replicator carries metadata and content between servers. The cluster
// implements it; with no replicator the store works standalone.
type Replicator interface {
	// BroadcastFiles gossips changed records to the cluster.
	BroadcastFiles(buckets []*Bucket, objects []*Object)
	// FileNodes lists the ids of the other alive servers with file storage.
	FileNodes() []string
	// OpenContent streams a blob from another server, starting at offset.
	OpenContent(ctx context.Context, nodeId, sha string, offset int64) (io.ReadCloser, error)
	// OpenContentBatch streams several small blobs from another server over one
	// connection, in the framing WriteContentBatch writes.
	OpenContentBatch(ctx context.Context, nodeId string, shas []string) (io.ReadCloser, error)
}

// QuotaFunc returns the file storage limit in bytes for a user, 0 for no limit.
type QuotaFunc func(userId string) (int64, error)

// BucketLimitFunc returns how many buckets a user may own, 0 for no limit.
type BucketLimitFunc func(userId string) (int, error)

// Config configures a Store.
type Config struct {
	Dir         string
	NodeId      string
	Quota       QuotaFunc
	BucketLimit BucketLimitFunc

	// OwnerState, when set, lets the store notice buckets whose owner has
	// been deleted, and remove them.
	OwnerState OwnerFunc

	// Changed, when set, is told which buckets had files or settings change,
	// whether by this server or another, so a page showing them can update.
	// Changes are gathered for a moment and reported together; a nil list
	// means too many buckets changed to name.
	Changed func(bucketIds []string)

	// NoSync skips the fsync of writes, so a crash or power loss can lose the
	// last moments of writes. It is for tests and tools; a server always
	// makes a write durable before it is reported done.
	NoSync bool
}

// bucketStats is what the store keeps alongside each bucket's records.
type bucketStats struct {
	mu sync.Mutex

	size  int64 // live content
	count int   // live objects

	// records is how many records are held, tombstones included.
	records int

	digest     uint64 // xor of the record hashes of every record held
	slots      [DigestSlots]uint64
	slotCounts [DigestSlots]int32
}

// toggle adds or removes a record's hash from the digests.
func (st *bucketStats) toggle(o *Object) {
	h := recordHash(o)
	st.digest ^= h
	st.slots[keySlot(o.Key)] ^= h
}

// dropRecord forgets a record that has been removed.
func (st *bucketStats) dropRecord(o *Object) {
	st.toggle(o)
	st.slotCounts[keySlot(o.Key)]--
	st.records--
}

func (st *bucketStats) usage() (int64, int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.size, st.count
}

// keySlot is the digest slot a key belongs to.
func keySlot(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() % DigestSlots)
}

// Store is the file storage engine of one server.
type Store struct {
	dir         string
	noSync      bool
	lock        *os.File
	nodeId      string
	quota       QuotaFunc
	bucketLimit BucketLimitFunc
	ownerState  OwnerFunc
	logger      logger.Logger
	db          *badger.DB

	// mu guards the buckets. Object changes hold it shared and rely on the
	// database to order changes to one key; changes to a bucket's identity
	// hold it exclusively, so no object is changed under a bucket that is
	// being replaced.
	mu      sync.RWMutex
	buckets map[string]*Bucket             // by id
	byName  map[string]string              // name of a live bucket -> its id
	owned   map[string]map[string]struct{} // owner id -> ids of their live buckets

	statsMu sync.RWMutex
	stats   map[string]*bucketStats

	missingMu sync.Mutex
	missing   map[string]*Object // content referenced but not held, by sha

	// pending is storage promised to uploads not yet recorded, by owner, so
	// concurrent uploads cannot together pass the owner's quota.
	pendMu  sync.Mutex
	pending map[string]int64

	// blobMu orders installing a content file, with the record that refers to
	// it, against removing files whose last reference went.
	blobMu     sync.RWMutex
	unlinkMu   sync.Mutex
	unlink     []string // content whose last reference went, to remove
	sweepMu    sync.Mutex
	sweepQueue map[string]struct{} // buckets with records to sweep
	sweepKick  chan struct{}
	// sweepPaused holds sweeping back, for tests of resuming one.
	sweepPaused atomic.Bool

	replMu sync.RWMutex
	repl   Replicator

	onChanged func(bucketIds []string)
	chMu      sync.Mutex
	chIds     map[string]struct{}
	chTimer   *time.Timer

	bcMu      sync.Mutex
	bcBuckets []*Bucket
	bcObjects []*Object
	bcKick    chan struct{}
	bcNow     bool // gossip each change at once rather than batched, for tests

	blobs   *blobStore
	fetcher *fetcher

	stop     chan struct{}
	wg       sync.WaitGroup
	bgMu     sync.Mutex // orders starting background work against Close
	closed   bool
	writeSeq atomic.Uint64 // counts changes to records, so a check can tell the store was busy
	damaged  atomic.Bool   // a write failed after memory was updated: statistics need recounting
}

var (
	instanceMu sync.RWMutex
	instance   *Store
)

// Get returns the server's store, nil when file storage is disabled.
func Get() *Store {
	instanceMu.RLock()
	defer instanceMu.RUnlock()
	return instance
}

// SetInstance installs the server's store.
func SetInstance(s *Store) {
	instanceMu.Lock()
	instance = s
	instanceMu.Unlock()
}

// Open opens or creates a store in cfg.Dir.
func Open(cfg Config) (*Store, error) {
	if cfg.Dir == "" {
		return nil, ErrDisabled
	}

	s := &Store{
		dir:         cfg.Dir,
		noSync:      cfg.NoSync,
		nodeId:      cfg.NodeId,
		quota:       cfg.Quota,
		bucketLimit: cfg.BucketLimit,
		ownerState:  cfg.OwnerState,
		onChanged:   cfg.Changed,
		logger:      log.WithGroup("files"),
		buckets:     make(map[string]*Bucket),
		owned:       make(map[string]map[string]struct{}),
		byName:      make(map[string]string),
		stats:       make(map[string]*bucketStats),
		missing:     make(map[string]*Object),
		pending:     make(map[string]int64),
		sweepQueue:  make(map[string]struct{}),
		sweepKick:   make(chan struct{}, 1),
		bcKick:      make(chan struct{}, 1),
		stop:        make(chan struct{}),
	}

	for _, d := range []string{"meta", "tmp", "multipart"} {
		if err := os.MkdirAll(filepath.Join(cfg.Dir, d), 0700); err != nil {
			return nil, err
		}
	}

	var err error
	if s.lock, err = lockDir(cfg.Dir); err != nil {
		return nil, err
	}
	opened := false
	defer func() {
		if !opened {
			if s.db != nil {
				s.db.Close()
			}
			if s.lock != nil {
				s.lock.Close()
			}
		}
	}()
	if s.blobs, err = newBlobStore(filepath.Join(cfg.Dir, "blobs"), filepath.Join(cfg.Dir, "tmp"), cfg.NoSync); err != nil {
		return nil, err
	}

	// Leftover temporary files belong to writes that never committed.
	cleanDir(filepath.Join(cfg.Dir, "tmp"))

	if s.db, err = openDB(filepath.Join(cfg.Dir, "db"), cfg.NoSync); err != nil {
		return nil, err
	}
	if err := s.loadState(); err != nil {
		return nil, err
	}

	var queued []string
	for name := range s.sweepQueue {
		queued = append(queued, name)
	}
	s.fetcher = newFetcher(s)
	s.wg.Add(4)
	go s.maintenance()
	go s.broadcaster()
	go s.sweeper()
	go s.refetcher()
	for _, name := range queued {
		s.kickSweep(name)
	}

	opened = true
	return s, nil
}

// Close stops background work, sends any changes not yet gossiped and
// closes the database.
func (s *Store) Close() {
	s.bgMu.Lock()
	s.closed = true
	s.bgMu.Unlock()
	close(s.stop)
	s.chMu.Lock()
	if s.chTimer != nil {
		s.chTimer.Stop()
		s.chTimer = nil
	}
	s.onChanged = nil
	s.chMu.Unlock()
	s.fetcher.close()
	s.wg.Wait()

	if s.db != nil {
		// A store that lost a write is recounted from its records next time.
		if !s.damaged.Load() {
			if err := s.saveState(); err != nil {
				s.logger.Error("failed to save file storage statistics", "error", err)
			}
		}
		s.db.Close()
	}
	if s.lock != nil {
		s.lock.Close()
	}
}

// SetNodeId sets the server id recorded as the source of locally written content.
func (s *Store) SetNodeId(id string) { s.nodeId = id }

// SetReplicator connects the store to the cluster.
func (s *Store) SetReplicator(r Replicator) {
	s.replMu.Lock()
	s.repl = r
	s.replMu.Unlock()
	if r != nil && s.fetcher != nil {
		go s.queueMissing(false)
	}
}

func (s *Store) replicator() Replicator {
	s.replMu.RLock()
	defer s.replMu.RUnlock()
	return s.repl
}

const (
	// changeDebounce is how long changes are gathered before they are reported.
	changeDebounce = 300 * time.Millisecond
	// changeMaxIds is how many buckets are named in one report.
	changeMaxIds = 100
)

// noteChanged records that buckets or files changed, to be reported to
// Config.Changed once the changes have settled. It costs nothing without one.
func (s *Store) noteChanged(buckets []*Bucket, objects []*Object) {
	if len(buckets) == 0 && len(objects) == 0 {
		return
	}
	s.chMu.Lock()
	defer s.chMu.Unlock()
	if s.onChanged == nil {
		return
	}
	if s.chIds == nil {
		s.chIds = map[string]struct{}{}
	}
	for _, b := range buckets {
		s.chIds[b.Id] = struct{}{}
	}
	for _, o := range objects {
		s.chIds[o.BucketId] = struct{}{}
	}
	if s.chTimer == nil {
		s.chTimer = time.AfterFunc(changeDebounce, s.reportChanged)
	}
}

func (s *Store) reportChanged() {
	s.chMu.Lock()
	fn := s.onChanged
	ids := make([]string, 0, len(s.chIds))
	for id := range s.chIds {
		ids = append(ids, id)
	}
	s.chIds, s.chTimer = nil, nil
	s.chMu.Unlock()
	if fn == nil {
		return
	}
	if len(ids) > changeMaxIds {
		ids = nil
	}
	fn(ids)
}

// broadcast queues changed records to be gossiped by the broadcaster.
func (s *Store) broadcast(buckets []*Bucket, objects []*Object) {
	s.noteChanged(buckets, objects)
	if s.replicator() == nil {
		return
	}
	s.bcMu.Lock()
	s.bcBuckets = append(s.bcBuckets, buckets...)
	s.bcObjects = append(s.bcObjects, objects...)
	// Bucket changes are rare and acted on straight away from other servers,
	// so they go at once; object writes wait to be batched.
	full := len(s.bcObjects) >= broadcastBatch || len(buckets) > 0
	now := s.bcNow
	s.bcMu.Unlock()
	if now {
		s.flushBroadcasts()
	} else if full {
		select {
		case s.bcKick <- struct{}{}:
		default:
		}
	}
}

// broadcaster gossips queued changes every broadcastInterval, or as soon as
// a full message's worth, or a bucket change, is waiting.
func (s *Store) broadcaster() {
	defer s.wg.Done()
	ticker := time.NewTicker(broadcastInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			s.flushBroadcasts()
			return
		case <-ticker.C:
		case <-s.bcKick:
		}
		s.flushBroadcasts()
	}
}

func (s *Store) flushBroadcasts() {
	s.bcMu.Lock()
	buckets, objects := s.bcBuckets, s.bcObjects
	s.bcBuckets, s.bcObjects = nil, nil
	s.bcMu.Unlock()

	r := s.replicator()
	if r == nil {
		return
	}
	// Bucket records go first, and receivers apply a message's buckets before
	// its objects, so objects always land against their bucket.
	for len(buckets) > 0 || len(objects) > 0 {
		nb := min(len(buckets), broadcastBucketBatch)
		no := 0
		if nb == len(buckets) {
			no = min(len(objects), broadcastBatch)
		}
		r.BroadcastFiles(buckets[:nb], objects[:no])
		buckets, objects = buckets[nb:], objects[no:]
	}
}

// ---------------------------------------------------------------------------
// Merge rules
// ---------------------------------------------------------------------------

// bucketNewer reports whether a should replace b.
func bucketNewer(a, b *Bucket) bool {
	if a.UpdatedAt != b.UpdatedAt {
		return a.UpdatedAt.After(b.UpdatedAt)
	}
	if a.IsDeleted != b.IsDeleted {
		return a.IsDeleted
	}
	return a.OwnerId > b.OwnerId
}

// objectNewer reports whether a should replace b: the later update.
func objectNewer(a, b *Object) bool {
	if a.UpdatedAt != b.UpdatedAt {
		return a.UpdatedAt.After(b.UpdatedAt)
	}
	if a.IsDeleted != b.IsDeleted {
		return a.IsDeleted
	}
	return a.SHA256 > b.SHA256
}

func recordHash(o *Object) uint64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s\x00%d\x00%t", o.Key, uint64(o.UpdatedAt), o.IsDeleted)
	return h.Sum64()
}

func live(b *Bucket) bool { return b != nil && !b.IsDeleted }

// liveLocked reports whether o is a visible object of a live bucket.
func (s *Store) liveLocked(o *Object) bool {
	if o == nil || o.IsDeleted {
		return false
	}
	return live(s.buckets[o.BucketId])
}

// statsFor returns a bucket's statistics, creating them.
func (s *Store) statsFor(bucket string) *bucketStats {
	s.statsMu.RLock()
	st := s.stats[bucket]
	s.statsMu.RUnlock()
	if st != nil {
		return st
	}
	s.statsMu.Lock()
	defer s.statsMu.Unlock()
	if st = s.stats[bucket]; st == nil {
		st = &bucketStats{}
		s.stats[bucket] = st
	}
	return st
}

// statsIfAny returns a bucket's statistics, nil if it has none.
func (s *Store) statsIfAny(bucket string) *bucketStats {
	s.statsMu.RLock()
	defer s.statsMu.RUnlock()
	return s.stats[bucket]
}

// ---------------------------------------------------------------------------
// Content
// ---------------------------------------------------------------------------

// holds reports whether the content of sha is stored here.
func (s *Store) holds(sha string) bool {
	if !validSHA(sha) {
		return false
	}
	found := false
	s.db.View(func(txn *badger.Txn) error {
		found, _ = hasKey(txn, shaKey('c', sha))
		return nil
	})
	return found || s.blobs.has(sha)
}

// openContent opens stored content.
func (s *Store) openContent(sha string) (Content, error) {
	if !validSHA(sha) {
		return nil, os.ErrNotExist
	}
	var data []byte
	var found bool
	err := s.db.View(func(txn *badger.Txn) (err error) {
		data, found, err = openInline(txn, sha)
		return
	})
	if err != nil {
		return nil, err
	}
	if found {
		return &memContent{Reader: bytes.NewReader(data)}, nil
	}
	f, err := s.blobs.open(sha)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (s *Store) isMissing(sha string) bool {
	s.missingMu.Lock()
	defer s.missingMu.Unlock()
	_, ok := s.missing[sha]
	return ok
}

// flushUnlinks removes content files left unreferenced. Each file is checked
// again under the lock that content is installed and referenced under, so
// content written again in the meantime is never caught; the work is batched
// so writers are not held up.
func (s *Store) flushUnlinks() {
	s.unlinkMu.Lock()
	shas := s.unlink
	s.unlink = nil
	s.unlinkMu.Unlock()
	if len(shas) == 0 {
		return
	}

	// A large removal, such as a bucket of many files, finishes in the
	// background so the request that caused it returns at once; once the
	// store is closing it runs here instead, so Close never races it.
	if len(shas) > unlinkBatch {
		s.bgMu.Lock()
		if !s.closed {
			s.wg.Add(1)
			s.bgMu.Unlock()
			go func() {
				defer s.wg.Done()
				s.removeUnlinked(shas)
			}()
			return
		}
		s.bgMu.Unlock()
	}
	s.removeUnlinked(shas)
}

func (s *Store) removeUnlinked(shas []string) {
	for len(shas) > 0 {
		n := min(len(shas), unlinkBatch)
		s.blobMu.Lock()
		for _, sha := range shas[:n] {
			if s.refCount(sha) == 0 {
				s.blobs.remove(sha)
			}
		}
		s.blobMu.Unlock()
		shas = shas[n:]
	}
}

// storeContent stores fetched or imported content of sha, if a record still
// references it. Small content goes into the database; a larger one is the
// temporary file at path, which is installed or removed.
func (s *Store) storeContent(sha string, size int64, data []byte, path string, force bool) error {
	if path != "" && size <= inlineMax {
		var err error
		if data, err = os.ReadFile(path); err != nil {
			return err
		}
		os.Remove(path)
		path = ""
	}

	// With no file, the content is the data, which is empty for an empty file.
	if path == "" {
		held := false
		err := s.db.Update(func(txn *badger.Txn) error {
			n, err := getRef(txn, sha)
			if err != nil {
				return err
			}
			if n == 0 {
				return nil // deleted while it was on its way
			}
			held = true
			if err := txn.Set(shaKey('c', sha), data); err != nil {
				return err
			}
			if s.isMissing(sha) {
				return txn.Delete(shaKey('m', sha))
			}
			return nil
		})
		if err != nil {
			return err
		}
		if held {
			s.contentHeld(sha)
		}
		return nil
	}

	s.blobMu.RLock()
	defer s.blobMu.RUnlock()
	if s.refCount(sha) == 0 && !force {
		os.Remove(path)
		return nil
	}
	if err := s.blobs.install(path, sha); err != nil {
		return err
	}
	if s.isMissing(sha) {
		if err := s.db.Update(func(txn *badger.Txn) error { return txn.Delete(shaKey('m', sha)) }); err != nil {
			return err
		}
	}
	s.contentHeld(sha)
	return nil
}

// contentHeld records that content is now stored locally.
func (s *Store) contentHeld(sha string) {
	s.missingMu.Lock()
	delete(s.missing, sha)
	s.missingMu.Unlock()
}

// ---------------------------------------------------------------------------
// Applying records
// ---------------------------------------------------------------------------

// indexLocked keeps the name and owner indexes in step as a bucket record
// changes.
func (s *Store) indexLocked(cur, next *Bucket) {
	if live(cur) {
		if s.byName[cur.Name] == cur.Id {
			delete(s.byName, cur.Name)
		}
		delete(s.owned[cur.OwnerId], cur.Id)
		if len(s.owned[cur.OwnerId]) == 0 {
			delete(s.owned, cur.OwnerId)
		}
	}
	if live(next) {
		s.byName[next.Name] = next.Id
		set := s.owned[next.OwnerId]
		if set == nil {
			set = make(map[string]struct{})
			s.owned[next.OwnerId] = set
		}
		set[next.Id] = struct{}{}
	}
}

// bucketByNameLocked returns the live bucket with a name, nil if none.
func (s *Store) bucketByNameLocked(name string) *Bucket {
	return s.buckets[s.byName[name]]
}

// bucketLoses reports whether bucket a gives way to b when both claim a
// name: the one created later, then the one with the greater id.
func bucketLoses(a, b *Bucket) bool {
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.After(b.CreatedAt)
	}
	return a.Id > b.Id
}

// freeNameLocked returns name with a suffix, -2, -3 and so on, that no live
// bucket holds.
func (s *Store) freeNameLocked(name string) string {
	for i := 2; ; i++ {
		suffix := fmt.Sprintf("-%d", i)
		base := name
		if len(base)+len(suffix) > 63 {
			base = strings.TrimRight(base[:63-len(suffix)], "-.")
		}
		if cand := base + suffix; ValidBucketName(cand) && s.byName[cand] == "" {
			return cand
		}
	}
}

// applyBucketLocked merges a bucket record. It returns the record now held,
// nil if the change was not applied, and any other records it had to change,
// all to be gossiped: when two live buckets claim a name the one created
// later is renamed with a suffix, so no data is lost. The caller holds the
// lock exclusively.
func (s *Store) applyBucketLocked(b *Bucket) (*Bucket, []*Bucket, error) {
	cur := s.buckets[b.Id]
	if cur != nil && !bucketNewer(b, cur) {
		return nil, nil, nil
	}
	next := b.clone()

	var renamed []*Bucket // other buckets changed with it
	var other *Bucket
	if live(next) {
		if o := s.bucketByNameLocked(next.Name); o != nil && o.Id != next.Id {
			if bucketLoses(next, o) {
				next.Name = s.freeNameLocked(next.Name)
				next.UpdatedAt = hlc.Now()
			} else {
				other = o.clone()
				other.Name = s.freeNameLocked(o.Name)
				other.UpdatedAt = hlc.Now()
				renamed = append(renamed, other)
			}
		}
	}

	// With the same state, which objects are visible cannot change: only the
	// record (its name, owner or grants) does.
	sameState := cur != nil && cur.IsDeleted == next.IsDeleted

	// Records held before the bucket is known become visible with it.
	var size int64
	count := 0
	st := s.statsFor(next.Id)
	st.mu.Lock()
	records := st.records
	st.mu.Unlock()
	if !sameState && !next.IsDeleted && records > 0 {
		err := s.db.View(func(txn *badger.Txn) error {
			it := txn.NewIterator(badger.IteratorOptions{PrefetchValues: true, PrefetchSize: 1000, Prefix: objectPrefix(next.Id)})
			defer it.Close()
			for it.Rewind(); it.Valid(); it.Next() {
				var o *Object
				if err := it.Item().Value(func(v []byte) (err error) { o, err = decodeObject(v); return }); err != nil {
					return err
				}
				if !o.IsDeleted {
					size += o.Size
					count++
				}
			}
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}

	sweep := !sameState && next.IsDeleted && records > 0
	err := s.db.Update(func(txn *badger.Txn) error {
		if err := txn.Set(bucketKey(next.Id), encodeRecord(next)); err != nil {
			return err
		}
		if other != nil {
			if err := txn.Set(bucketKey(other.Id), encodeRecord(other)); err != nil {
				return err
			}
		}
		if sweep {
			return txn.Set(queueKey(next.Id), []byte{1})
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	if other != nil {
		prev := s.buckets[other.Id]
		s.indexLocked(prev, nil)
		s.buckets[other.Id] = other
		s.indexLocked(nil, other)
	}
	s.indexLocked(cur, next)
	s.buckets[next.Id] = next
	if !sameState {
		st.mu.Lock()
		st.size, st.count = size, count
		st.mu.Unlock()
	}
	if sweep {
		s.kickSweep(next.Id)
	}
	return next, renamed, nil
}

// objectOp is one change to an object record.
type objectOp struct {
	obj *Object

	// pre, for a change made here rather than received, checks the record
	// being replaced, nil if there is none, inside the transaction.
	pre func(txn *badger.Txn, cur *Object) error

	// content is what is stored with the record: nothing (it arrives by
	// replication, or is shared with another record), the data to keep in the
	// database, or a file already installed.
	content contentKind
	data    []byte
}

type contentKind int

const (
	contentNone contentKind = iota
	contentInline
	contentFile
)

// applied is the outcome of one objectOp.
type applied struct {
	obj      *Object // the record now held, nil if the change was not applied
	cur      *Object // the record it replaced
	curLive  bool
	newLive  bool
	missing  bool     // its content is not held
	held     bool     // its content was supplied
	released []string // content whose last reference went
}

// applyObjects merges object records into the database in one transaction,
// returning the records that changed. The caller holds s.mu, shared or
// exclusive. At most writeChunk ops are applied at once.
func (s *Store) applyObjects(ops []objectOp) ([]*Object, error) {
	var changed []*Object
	for len(ops) > 0 {
		n := min(len(ops), writeChunk)
		got, err := s.applyChunk(ops[:n])
		if err != nil {
			return changed, err
		}
		changed = append(changed, got...)
		ops = ops[n:]
	}
	return changed, nil
}

func (s *Store) applyChunk(ops []objectOp) ([]*Object, error) {
	var results []applied
	for attempt := 0; ; attempt++ {
		results = results[:0]
		err := s.db.Update(func(txn *badger.Txn) error {
			results = results[:0]
			for _, op := range ops {
				a, err := s.applyOne(txn, op)
				if err != nil {
					return err
				}
				results = append(results, a)
			}
			return nil
		})
		if errors.Is(err, badger.ErrConflict) && attempt < 50 {
			continue
		}
		if err != nil {
			return nil, err
		}
		break
	}

	s.writeSeq.Add(1)
	var changed []*Object
	var enqueue []*Object
	for _, a := range results {
		if a.obj == nil {
			continue
		}
		changed = append(changed, a.obj)

		st := s.statsFor(a.obj.BucketId)
		st.mu.Lock()
		if a.cur != nil {
			st.toggle(a.cur)
			if a.curLive {
				st.size -= a.cur.Size
				st.count--
			}
		} else {
			st.records++
			st.slotCounts[keySlot(a.obj.Key)]++
		}
		st.toggle(a.obj)
		if a.newLive {
			st.size += a.obj.Size
			st.count++
		}
		st.mu.Unlock()

		if a.missing {
			s.missingMu.Lock()
			_, already := s.missing[a.obj.SHA256]
			s.missing[a.obj.SHA256] = a.obj
			s.missingMu.Unlock()
			if !already {
				enqueue = append(enqueue, a.obj)
			}
		}
		if a.held {
			s.contentHeld(a.obj.SHA256)
		}
		if len(a.released) > 0 {
			s.missingMu.Lock()
			for _, sha := range a.released {
				delete(s.missing, sha)
			}
			s.missingMu.Unlock()
			s.unlinkMu.Lock()
			s.unlink = append(s.unlink, a.released...)
			s.unlinkMu.Unlock()
		}
	}
	if s.fetcher != nil {
		for _, o := range enqueue {
			s.fetcher.enqueue(o.SHA256, o.Size, o.SourceNode)
		}
	}
	return changed, nil
}

// applyOne merges one object record in txn.
func (s *Store) applyOne(txn *badger.Txn, op objectOp) (applied, error) {
	o := op.obj
	if b := s.buckets[o.BucketId]; b != nil && b.IsDeleted {
		return applied{}, nil
	}

	cur, err := getObject(txn, o.BucketId, o.Key)
	if err != nil {
		return applied{}, err
	}
	if op.pre != nil {
		if err := op.pre(txn, cur); err != nil {
			return applied{}, err
		}
	}
	if cur != nil && !objectNewer(o, cur) {
		return applied{}, nil
	}

	n := o.clone()
	a := applied{obj: n, cur: cur, curLive: s.liveLocked(cur), newLive: s.liveLocked(n)}
	if err := txn.Set(objectKey(o.BucketId, o.Key), encodeRecord(n)); err != nil {
		return applied{}, err
	}
	if cur != nil && cur.IsDeleted {
		if err := txn.Delete(tombKey(cur.UpdatedAt, cur.BucketId, cur.Key)); err != nil {
			return applied{}, err
		}
	}
	if n.IsDeleted {
		if err := txn.Set(tombKey(n.UpdatedAt, n.BucketId, n.Key), []byte{}); err != nil {
			return applied{}, err
		}
	}

	// Take the new reference before dropping the old so content shared by
	// both versions is never removed in between.
	if !n.IsDeleted {
		refs, err := getRef(txn, n.SHA256)
		if err != nil {
			return applied{}, err
		}
		if err := setRef(txn, n.SHA256, refs+1); err != nil {
			return applied{}, err
		}
		wasMissing := s.isMissing(n.SHA256)
		switch op.content {
		case contentInline:
			if refs == 0 || wasMissing {
				if err := txn.Set(shaKey('c', n.SHA256), op.data); err != nil {
					return applied{}, err
				}
			}
			a.held = true
		case contentFile:
			a.held = true
		default:
			if refs == 0 {
				held, err := hasKey(txn, shaKey('c', n.SHA256))
				if err != nil {
					return applied{}, err
				}
				if !held && !s.blobs.has(n.SHA256) {
					if err := txn.Set(shaKey('m', n.SHA256), encodeRecord(n)); err != nil {
						return applied{}, err
					}
					a.missing = true
				}
			}
		}
		if a.held && wasMissing {
			if err := txn.Delete(shaKey('m', n.SHA256)); err != nil {
				return applied{}, err
			}
		}
	}
	if cur != nil && !cur.IsDeleted {
		gone, err := s.release(txn, cur.SHA256)
		if err != nil {
			return applied{}, err
		}
		if gone {
			a.released = append(a.released, cur.SHA256)
		}
	}
	return a, nil
}

// release drops one reference to content, removing it when it was the last;
// a content file is removed by the caller once the transaction commits.
func (s *Store) release(txn *badger.Txn, sha string) (bool, error) {
	n, err := getRef(txn, sha)
	if err != nil {
		return false, err
	}
	if n > 1 {
		return false, setRef(txn, sha, n-1)
	}
	if err := txn.Delete(shaKey('r', sha)); err != nil {
		return false, err
	}
	if err := txn.Delete(shaKey('c', sha)); err != nil {
		return false, err
	}
	// The queue to fetch it goes too, whether or not this server has noted it
	// yet: a record that came and went in quick succession can leave a mark
	// no later change would clear.
	if queued, err := hasKey(txn, shaKey('m', sha)); err != nil {
		return false, err
	} else if queued {
		if err := txn.Delete(shaKey('m', sha)); err != nil {
			return false, err
		}
	}
	return true, nil
}

// ---------------------------------------------------------------------------
// Replication entry points
// ---------------------------------------------------------------------------

// Merge applies bucket and object records received from another server,
// buckets first so objects land against their bucket.
func (s *Store) Merge(buckets []*Bucket, objects []*Object) {
	// Records received are not gossiped on, but a bucket renamed because its
	// name clashed with another's is a new record the others need.
	var changedB []*Bucket
	s.mu.Lock()
	for _, b := range buckets {
		if b == nil || !ValidBucketId(b.Id) || !ValidBucketName(b.Name) {
			continue
		}
		stored, renamed, err := s.applyBucketLocked(b)
		if err != nil {
			s.fail("failed to apply bucket", err)
			continue
		}
		if stored != nil && stored.UpdatedAt != b.UpdatedAt {
			changedB = append(changedB, stored)
		}
		changedB = append(changedB, renamed...)
	}
	s.mu.Unlock()
	if len(changedB) > 0 {
		s.broadcast(changedB, nil)
	}

	var ops []objectOp
	for _, o := range objects {
		if o == nil || !ValidBucketId(o.BucketId) || !ValidKey(o.Key) || (!o.IsDeleted && len(o.SHA256) != 64) {
			continue
		}
		ops = append(ops, objectOp{obj: o})
	}
	s.mu.RLock()
	changedO, err := s.applyObjects(ops)
	s.mu.RUnlock()
	if err != nil {
		s.fail("failed to apply objects", err)
	}
	s.noteChanged(nil, changedO)
	s.flushUnlinks()
}

// fail logs a write that did not reach the database. Memory may now differ
// from it, so the statistics are recounted at the next start.
func (s *Store) fail(what string, err error) {
	s.damaged.Store(true)
	s.logger.Error("file storage: "+what, "error", err)
}

// BucketDigest summarises a bucket for anti-entropy.
type BucketDigest struct {
	Bucket *Bucket `msgpack:"bucket"`
	Digest uint64  `msgpack:"digest"`
	Count  int     `msgpack:"count"`
}

// Digests returns up to limit bucket digests, deleted buckets included, keyed
// by bucket id and taken in id order after the given id, and the id to
// continue after ("" at the end). A limit of 0 returns them all.
func (s *Store) Digests(after string, limit int) (map[string]BucketDigest, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.buckets))
	for name := range s.buckets {
		if name > after {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	next := ""
	if limit > 0 && len(names) > limit {
		names = names[:limit]
		next = names[limit-1]
	}

	out := make(map[string]BucketDigest, len(names))
	for _, name := range names {
		d := BucketDigest{Bucket: s.buckets[name].clone()}
		if st := s.statsIfAny(name); st != nil {
			st.mu.Lock()
			d.Digest = st.digest
			d.Count = st.records
			st.mu.Unlock()
		}
		out[name] = d
	}
	return out, next
}

// SlotDigests returns the digest and record count of each of a bucket's
// digest slots.
func (s *Store) SlotDigests(bucket string) ([]uint64, []int32) {
	digests, counts := make([]uint64, DigestSlots), make([]int32, DigestSlots)
	if st := s.statsIfAny(bucket); st != nil {
		st.mu.Lock()
		copy(digests, st.slots[:])
		copy(counts, st.slotCounts[:])
		st.mu.Unlock()
	}
	return digests, counts
}

// NewerBuckets returns the local bucket records that are newer than, or
// missing from, a peer's digests.
func (s *Store) NewerBuckets(remote map[string]BucketDigest) []*Bucket {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Bucket
	for name, b := range s.buckets {
		if r, ok := remote[name]; !ok || r.Bucket == nil || bucketNewer(b, r.Bucket) {
			out = append(out, b.clone())
		}
	}
	return out
}

// ObjectPage returns up to limit object records of a bucket, tombstones
// included, in key order after the given key, and the key to continue after
// ("" at the end). With slots given only records in those digest slots are
// returned.
func (s *Store) ObjectPage(bucket, after string, limit int, slots []int) ([]*Object, string) {
	limit = max(limit, 1)
	var want [DigestSlots]bool
	for _, sl := range slots {
		if sl >= 0 && sl < DigestSlots {
			want[sl] = true
		}
	}

	base := objectPrefix(bucket)
	var page []*Object
	next := ""
	s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.IteratorOptions{PrefetchValues: true, PrefetchSize: 100, Prefix: base})
		defer it.Close()
		for it.Seek(append(append([]byte(nil), base...), after...)); it.Valid(); it.Next() {
			k := string(it.Item().Key()[len(base):])
			if k == after || (slots != nil && !want[keySlot(k)]) {
				continue
			}
			if len(page) == limit {
				next = page[len(page)-1].Key
				return nil
			}
			var o *Object
			if err := it.Item().Value(func(v []byte) (err error) { o, err = decodeObject(v); return }); err != nil {
				return err
			}
			page = append(page, o)
		}
		return nil
	})
	return page, next
}

// ---------------------------------------------------------------------------
// Sweeping
// ---------------------------------------------------------------------------

// kickSweep schedules a sweep of a bucket's records that can never be seen
// again, because the bucket was deleted or replaced.
func (s *Store) kickSweep(name string) {
	s.sweepMu.Lock()
	s.sweepQueue[name] = struct{}{}
	s.sweepMu.Unlock()
	select {
	case s.sweepKick <- struct{}{}:
	default:
	}
}

func (s *Store) sweeper() {
	defer s.wg.Done()
	for {
		select {
		case <-s.stop:
			return
		case <-s.sweepKick:
		}
		for !s.sweepPaused.Load() {
			s.sweepMu.Lock()
			name := ""
			for n := range s.sweepQueue {
				name = n
				break
			}
			if name != "" {
				delete(s.sweepQueue, name)
			}
			s.sweepMu.Unlock()
			if name == "" {
				break
			}
			if err := s.sweepBucket(name); err != nil {
				s.logger.Error("failed to sweep bucket", "bucket", name, "error", err)
			}
			select {
			case <-s.stop:
				return
			default:
			}
		}
	}
}

// sweepBucket removes the records of a bucket that its current record makes
// unreachable. It works a chunk at a time, so the store carries on meanwhile.
func (s *Store) sweepBucket(name string) error {
	base := objectPrefix(name)
	after := ""
	for {
		select {
		case <-s.stop:
			// Resumed at the next start from the queue key.
			return nil
		default:
		}

		s.mu.RLock()
		b := s.buckets[name]
		var dead []*Object
		last := ""
		more := false
		// Only the records of a deleted bucket can never be seen again.
		if b != nil && b.IsDeleted {
			s.db.View(func(txn *badger.Txn) error {
				it := txn.NewIterator(badger.IteratorOptions{PrefetchValues: true, PrefetchSize: 200, Prefix: base})
				defer it.Close()
				seek := append(append([]byte(nil), base...), after...)
				for it.Seek(seek); it.Valid(); it.Next() {
					k := string(it.Item().Key()[len(base):])
					if k == after && after != "" {
						continue
					}
					if len(dead) == writeChunk {
						more = true
						return nil
					}
					last = k
					var o *Object
					if err := it.Item().Value(func(v []byte) (err error) { o, err = decodeObject(v); return }); err != nil {
						return err
					}
					dead = append(dead, o)
				}
				return nil
			})
		}
		if len(dead) > 0 {
			if err := s.dropRecords(dead); err != nil {
				s.mu.RUnlock()
				return err
			}
		}
		s.mu.RUnlock()
		s.flushUnlinks()

		if !more {
			break
		}
		after = last
		if len(dead) == 0 {
			continue
		}
	}
	return s.db.Update(func(txn *badger.Txn) error { return txn.Delete(queueKey(name)) })
}

// dropRecords removes records from the database, each only if it is still
// the record that was read, releasing what they reference. The caller holds
// s.mu.
func (s *Store) dropRecords(recs []*Object) error {
	var dropped []*Object
	var released []string
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		err = s.dropTxn(recs, &dropped, &released)
		if !errors.Is(err, badger.ErrConflict) {
			break
		}
	}
	if err != nil {
		return err
	}
	s.writeSeq.Add(1)
	for _, o := range dropped {
		st := s.statsFor(o.BucketId)
		st.mu.Lock()
		st.dropRecord(o)
		st.mu.Unlock()
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
	}
	return nil
}

func (s *Store) dropTxn(recs []*Object, droppedOut *[]*Object, releasedOut *[]string) error {
	var dropped []*Object
	var released []string
	err := s.db.Update(func(txn *badger.Txn) error {
		dropped, released = dropped[:0], released[:0]
		for _, o := range recs {
			cur, err := getObject(txn, o.BucketId, o.Key)
			if err != nil {
				return err
			}
			if cur == nil || cur.UpdatedAt != o.UpdatedAt || cur.IsDeleted != o.IsDeleted {
				continue
			}
			if err := txn.Delete(objectKey(o.BucketId, o.Key)); err != nil {
				return err
			}
			if o.IsDeleted {
				if err := txn.Delete(tombKey(o.UpdatedAt, o.BucketId, o.Key)); err != nil {
					return err
				}
			} else {
				gone, err := s.release(txn, o.SHA256)
				if err != nil {
					return err
				}
				if gone {
					released = append(released, o.SHA256)
				}
			}
			dropped = append(dropped, o)
		}
		return nil
	})
	if err != nil {
		return err
	}
	*droppedOut, *releasedOut = dropped, released
	return nil
}

// dropExpired forgets buckets and objects deleted before the tombstone TTL.
func (s *Store) dropExpired() {
	expiry := hlc.FromTime(time.Now().Add(-TombstoneTTL))

	for {
		var recs []*Object
		var stale [][]byte
		s.mu.RLock()
		s.db.View(func(txn *badger.Txn) error {
			it := txn.NewIterator(badger.IteratorOptions{Prefix: []byte{'t'}})
			defer it.Close()
			for it.Rewind(); it.Valid() && len(recs) < writeChunk; it.Next() {
				k := it.Item().KeyCopy(nil)
				if beUint64(k[1:9]) >= uint64(expiry) {
					return nil
				}
				if len(k) < 25 {
					stale = append(stale, k)
					continue
				}
				o, err := getObject(txn, idFromRaw(k[9:25]), string(k[25:]))
				if err != nil {
					return err
				}
				if o == nil || !o.IsDeleted || uint64(o.UpdatedAt) != beUint64(k[1:9]) {
					stale = append(stale, k)
					continue
				}
				recs = append(recs, o)
			}
			return nil
		})
		var err error
		if len(recs) > 0 {
			err = s.dropRecords(recs)
		}
		if len(stale) > 0 && err == nil {
			err = s.db.Update(func(txn *badger.Txn) error {
				for _, k := range stale {
					if err := txn.Delete(k); err != nil {
						return err
					}
				}
				return nil
			})
		}
		s.mu.RUnlock()
		if err != nil {
			s.logger.Error("failed to remove expired records", "error", err)
			return
		}
		if len(recs) == 0 && len(stale) == 0 {
			break
		}
	}

	// A deleted bucket is forgotten once its records are gone.
	s.mu.Lock()
	var gone []string
	for name, b := range s.buckets {
		if b.IsDeleted && b.UpdatedAt.Before(expiry) {
			if st := s.statsIfAny(name); st == nil || func() bool { st.mu.Lock(); defer st.mu.Unlock(); return st.records == 0 }() {
				gone = append(gone, name)
			}
		}
	}
	if len(gone) > 0 {
		err := s.db.Update(func(txn *badger.Txn) error {
			for _, name := range gone {
				if err := txn.Delete(bucketKey(name)); err != nil {
					return err
				}
				if err := txn.Delete(statsKey(name)); err != nil {
					return err
				}
			}
			return nil
		})
		if err == nil {
			s.statsMu.Lock()
			for _, name := range gone {
				delete(s.buckets, name)
				delete(s.stats, name)
			}
			s.statsMu.Unlock()
		}
	}
	s.mu.Unlock()
}

func beUint64(b []byte) uint64 {
	return uint64(b[0])<<56 | uint64(b[1])<<48 | uint64(b[2])<<40 | uint64(b[3])<<32 |
		uint64(b[4])<<24 | uint64(b[5])<<16 | uint64(b[6])<<8 | uint64(b[7])
}

// tmpMaxAge is how long a temporary file may go unwritten before it is
// treated as abandoned; writes in progress keep refreshing its time.
const tmpMaxAge = time.Hour

// sweepInterval is how often the storage directory is swept for garbage.
const sweepInterval = time.Hour

// cleanupStats reports what a sweep removed.
type cleanupStats struct {
	Blobs     int
	TempFiles int
	Uploads   int
}

// cleanup removes everything on disk no longer needed: content files no
// record references, temporary files of abandoned writes and fetches,
// multipart uploads that are stale or whose bucket is gone, and empty
// content directories. It runs shortly after start-up and every sweepInterval.
func (s *Store) cleanup() cleanupStats {
	var stats cleanupStats
	stats.Blobs = s.removeOrphanBlobs()

	if entries, err := os.ReadDir(filepath.Join(s.dir, "tmp")); err == nil {
		for _, e := range entries {
			if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > tmpMaxAge {
				if os.RemoveAll(filepath.Join(s.dir, "tmp", e.Name())) == nil {
					stats.TempFiles++
				}
			}
		}
	}

	stats.Uploads = s.cleanMultipart()
	s.blobs.removeEmptyDirs()
	return stats
}

// removeOrphanBlobs deletes content files no record references, checked in
// batches as flushUnlinks does.
func (s *Store) removeOrphanBlobs() int {
	var orphans []string
	s.blobs.walk(func(sha string) {
		orphans = append(orphans, sha)
	})

	removed := 0
	for len(orphans) > 0 {
		n := min(len(orphans), unlinkBatch)
		s.blobMu.Lock()
		for _, sha := range orphans[:n] {
			if s.refCount(sha) == 0 {
				s.blobs.remove(sha)
				removed++
			}
		}
		s.blobMu.Unlock()
		orphans = orphans[n:]
	}
	return removed
}

func (s *Store) maintenance() {
	defer s.wg.Done()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	// Housekeeping that scans is left until the server is up.
	first := time.NewTimer(10 * time.Second)
	defer first.Stop()
	lastSweep := time.Time{}
	lastGC := time.Now()

	for {
		select {
		case <-s.stop:
			return
		case <-first.C:
		case <-ticker.C:
		}

		// Queue any content still missing, e.g. after a failed fetch.
		s.queueMissing(false)

		if time.Since(lastSweep) >= sweepInterval {
			lastSweep = time.Now()
			s.dropExpired()
			s.reconcileOwners()
			if st := s.cleanup(); st != (cleanupStats{}) {
				s.logger.Info("removed unused files", "content", st.Blobs, "temporary", st.TempFiles, "uploads", st.Uploads)
			}
		}

		// Reclaim the space of removed inline content.
		if time.Since(lastGC) >= 10*time.Minute {
			lastGC = time.Now()
			for s.db.RunValueLogGC(0.5) == nil {
			}
		}
	}
}

// refetcher keeps the content still missing being fetched: a server that has
// fallen far behind has more to fetch than the queue holds, so this feeds it
// as it drains, rather than leaving the rest for the next sweep.
func (s *Store) refetcher() {
	defer s.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
		}
		s.missingMu.Lock()
		n := len(s.missing)
		s.missingMu.Unlock()
		if n > 0 && s.fetcher != nil && !s.fetcher.busy() {
			s.queueMissing(true)
		}
	}
}

// queueMissing schedules a fetch of the content still missing, forgetting
// content that has arrived. With wait it waits for room in the queue; without
// it content that does not fit is left for the next call.
func (s *Store) queueMissing(wait bool) {
	s.missingMu.Lock()
	todo := make([]*Object, 0, len(s.missing))
	for _, o := range s.missing {
		todo = append(todo, o)
	}
	s.missingMu.Unlock()

	for _, o := range todo {
		if s.refCount(o.SHA256) == 0 {
			// Nothing wants it now: the record that did has gone.
			s.contentHeld(o.SHA256)
			s.db.Update(func(txn *badger.Txn) error { return txn.Delete(shaKey('m', o.SHA256)) })
		} else if s.holds(o.SHA256) {
			s.contentHeld(o.SHA256)
			s.db.Update(func(txn *badger.Txn) error { return txn.Delete(shaKey('m', o.SHA256)) })
		} else if s.fetcher != nil {
			if wait {
				s.fetcher.enqueueWait(o.SHA256, o.Size, o.SourceNode)
			} else {
				s.fetcher.enqueue(o.SHA256, o.Size, o.SourceNode)
			}
		}
	}
}

func cleanDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		os.RemoveAll(filepath.Join(dir, e.Name()))
	}
}

// ---------------------------------------------------------------------------
// Read helpers
// ---------------------------------------------------------------------------

func (s *Store) bucketForLocked(p *Principal, name string, need int) (*Bucket, error) {
	b := s.bucketByNameLocked(name)
	if !live(b) {
		return nil, ErrNoSuchBucket
	}
	level := b.AccessFor(p)
	if level == AccessNone {
		// Hide buckets the caller cannot see at all.
		return nil, ErrNoSuchBucket
	}
	if level < need {
		return nil, ErrAccessDenied
	}
	return b, nil
}

// checkBucket verifies p has the needed access to a bucket, returning its owner.
func (s *Store) checkBucket(p *Principal, name string, need int) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, err := s.bucketForLocked(p, name, need)
	if err != nil {
		return "", err
	}
	return b.OwnerId, nil
}

// checkBucketId is checkBucket returning the bucket's id.
func (s *Store) checkBucketId(p *Principal, name string, need int) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, err := s.bucketForLocked(p, name, need)
	if err != nil {
		return "", err
	}
	return b.Id, nil
}

// liveObjectLocked returns a visible object, nil if there is none. The
// caller holds s.mu.
func (s *Store) liveObjectLocked(bucketId, key string) *Object {
	var o *Object
	s.db.View(func(txn *badger.Txn) (err error) {
		o, err = getObject(txn, bucketId, key)
		return
	})
	if !s.liveLocked(o) {
		return nil
	}
	return o
}

// usageLocked sums the content and objects in the buckets userId owns.
func (s *Store) usageLocked(userId string) (int64, int) {
	var size int64
	count := 0
	for name := range s.owned[userId] {
		if st := s.statsIfAny(name); st != nil {
			sz, c := st.usage()
			size += sz
			count += c
		}
	}
	return size, count
}

// The limits below call out to the database, so they are looked up before
// taking the store lock and only compared under it.

func (s *Store) quotaLimit(userId string) (int64, error) {
	if s.quota == nil {
		return 0, nil
	}
	return s.quota(userId)
}

func (s *Store) bucketLimitFor(userId string) (int, error) {
	if s.bucketLimit == nil {
		return 0, nil
	}
	return s.bucketLimit(userId)
}

// quotaFor checks p's access to a bucket and returns its owner's storage
// limit, for comparing under the lock with reserve.
func (s *Store) quotaFor(p *Principal, bucket string, need int) (string, int64, error) {
	owner, err := s.checkBucket(p, bucket, need)
	if err != nil {
		return "", 0, err
	}
	limit, err := s.quotaLimit(owner)
	return owner, limit, err
}

// remainingLocked is how many more bytes userId may store within limit
// when replacing existing bytes, -1 for no limit.
func (s *Store) remainingLocked(userId string, limit, existing int64) int64 {
	if limit <= 0 {
		return -1
	}
	used, _ := s.usageLocked(userId)
	s.pendMu.Lock()
	used += s.pending[userId]
	s.pendMu.Unlock()
	return max(0, limit-used+existing)
}

// roomLocked verifies userId can store delta more bytes within limit.
func (s *Store) roomLocked(userId string, limit, delta int64) error {
	if delta <= 0 || limit <= 0 {
		return nil
	}
	if used, _ := s.usageLocked(userId); used+delta > limit {
		return ErrQuotaExceeded
	}
	return nil
}

// reserveLocked verifies userId can store delta more bytes within limit and
// promises them until the returned function is called, so uploads running
// at once cannot together pass the limit. The caller holds s.mu.
func (s *Store) reserveLocked(userId string, limit, delta int64) (func(), error) {
	if delta <= 0 || limit <= 0 {
		return func() {}, nil
	}
	used, _ := s.usageLocked(userId)
	s.pendMu.Lock()
	defer s.pendMu.Unlock()
	if used+s.pending[userId]+delta > limit {
		return nil, ErrQuotaExceeded
	}
	s.pending[userId] += delta
	return func() {
		s.pendMu.Lock()
		s.pending[userId] -= delta
		if s.pending[userId] <= 0 {
			delete(s.pending, userId)
		}
		s.pendMu.Unlock()
	}, nil
}

// UserUsage is the storage and buckets a user owns.
type UserUsage struct {
	UsedBytes int64
	Objects   int
	Buckets   int
}

// Usage returns the storage and buckets userId owns.
func (s *Store) Usage(userId string) UserUsage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u := UserUsage{Buckets: len(s.owned[userId])}
	u.UsedBytes, u.Objects = s.usageLocked(userId)
	return u
}

func normalizeMeta(meta map[string]string) map[string]string {
	if len(meta) == 0 {
		return nil
	}
	out := make(map[string]string, len(meta))
	for k, v := range meta {
		out[strings.ToLower(k)] = v
	}
	return out
}

// ownerReport is what a look at the owners of buckets found.
type ownerReport struct {
	Deleted []string // owners that were deleted
	Missing []string // owners the user database does not know
	Unknown int      // owners it could not say
}

// checkOwners asks the user database about the owner of every live bucket.
func (s *Store) checkOwners() ownerReport {
	var r ownerReport
	if s.ownerState == nil {
		return r
	}
	s.mu.RLock()
	owners := make([]string, 0, len(s.owned))
	for id := range s.owned {
		owners = append(owners, id)
	}
	s.mu.RUnlock()
	sort.Strings(owners)

	for _, id := range owners {
		switch s.ownerState(id) {
		case OwnerDeleted:
			r.Deleted = append(r.Deleted, id)
		case OwnerMissing:
			r.Missing = append(r.Missing, id)
		case OwnerUnknown:
			r.Unknown++
		}
	}
	return r
}

// reconcileOwners removes the buckets of users that were deleted, which
// should have gone with them. An owner the database does not know is only
// reported: the user may not have replicated here yet. It returns how many
// buckets it deleted.
func (s *Store) reconcileOwners() int {
	r := s.checkOwners()
	deleted := 0
	for _, id := range r.Deleted {
		n := s.DeleteUser(id)
		deleted += n
		s.logger.Warn("removed the file storage buckets of a deleted user", "user_id", id, "buckets", n)
	}
	for _, id := range r.Missing {
		s.logger.Warn("file storage buckets belong to a user that is not in the user database; an administrator can transfer or delete them", "user_id", id)
	}
	return deleted
}
