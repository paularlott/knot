package filestore

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/logger"
)

const (
	// TombstoneTTL is how long deleted buckets and objects are remembered so
	// a server returning from an outage cannot resurrect them.
	TombstoneTTL = 30 * 24 * time.Hour

	// compactMinEntries is the journal size below which compaction is skipped.
	compactMinEntries = 10000

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

	// NoSync skips the fsync of uploaded content and of each journal write,
	// so a crash or power loss can lose the last moments of writes, and in
	// the worst case leave a file whose content is incomplete. The default,
	// false, makes a write durable before it is reported done.
	NoSync bool
}

// bucketStats is what the store keeps alongside each bucket's records.
type bucketStats struct {
	size  int64 // live content
	count int   // live objects

	// keys holds the key of every record held, tombstones included, sorted,
	// so listings and anti-entropy pages seek rather than sort.
	keys []string

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

func (st *bucketStats) addKey(key string) {
	i := sort.SearchStrings(st.keys, key)
	st.keys = append(st.keys, "")
	copy(st.keys[i+1:], st.keys[i:])
	st.keys[i] = key
	st.slotCounts[keySlot(key)]++
}

// dropRecord removes a record from a bucket's map and digests; callers
// rebuild the key index once they are done with rebuildKeys.
func (st *bucketStats) dropRecord(objs map[string]*Object, o *Object) {
	st.toggle(o)
	st.slotCounts[keySlot(o.Key)]--
	delete(objs, o.Key)
}

func (st *bucketStats) rebuildKeys(objs map[string]*Object) {
	st.keys = st.keys[:0]
	for k := range objs {
		st.keys = append(st.keys, k)
	}
	sort.Strings(st.keys)
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
	logger      logger.Logger

	mu       sync.RWMutex
	buckets  map[string]*Bucket
	objects  map[string]map[string]*Object
	stats    map[string]*bucketStats
	owned    map[string]map[string]struct{} // owner id -> names of their live buckets
	blobRefs map[string]int
	missing  map[string]*Object // content referenced but not held, by sha
	unlink   []string           // content whose last reference went, to remove

	journalMu      sync.Mutex
	journal        *os.File
	journalEntries int

	replMu sync.RWMutex
	repl   Replicator

	bcMu      sync.Mutex
	bcBuckets []*Bucket
	bcObjects []*Object
	bcKick    chan struct{}
	bcNow     bool // gossip each change at once rather than batched, for tests

	blobs   *blobStore
	fetcher *fetcher

	stop   chan struct{}
	wg     sync.WaitGroup
	bgMu   sync.Mutex // orders starting background work against Close
	closed bool
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
		logger:      log.WithGroup("files"),
		buckets:     make(map[string]*Bucket),
		objects:     make(map[string]map[string]*Object),
		stats:       make(map[string]*bucketStats),
		owned:       make(map[string]map[string]struct{}),
		blobRefs:    make(map[string]int),
		missing:     make(map[string]*Object),
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
		if !opened && s.lock != nil {
			s.lock.Close()
		}
	}()
	if s.blobs, err = newBlobStore(filepath.Join(cfg.Dir, "blobs"), filepath.Join(cfg.Dir, "tmp"), cfg.NoSync); err != nil {
		return nil, err
	}

	// Leftover temporary files belong to writes that never committed.
	cleanDir(filepath.Join(cfg.Dir, "tmp"))

	if err := s.load(); err != nil {
		return nil, err
	}
	if err := s.compact(); err != nil {
		return nil, err
	}
	if st := s.cleanup(); st != (cleanupStats{}) {
		s.logger.Info("removed unused files", "content", st.Blobs, "temporary", st.TempFiles, "uploads", st.Uploads)
	}

	s.fetcher = newFetcher(s)
	s.wg.Add(2)
	go s.maintenance()
	go s.broadcaster()

	opened = true
	return s, nil
}

// Close stops background work, sends any changes not yet gossiped and
// closes the journal.
func (s *Store) Close() {
	s.bgMu.Lock()
	s.closed = true
	s.bgMu.Unlock()
	close(s.stop)
	s.fetcher.close()
	s.wg.Wait()

	s.journalMu.Lock()
	if s.journal != nil {
		s.journal.Close()
		s.journal = nil
	}
	s.journalMu.Unlock()

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
		go s.queueMissing()
	}
}

func (s *Store) replicator() Replicator {
	s.replMu.RLock()
	defer s.replMu.RUnlock()
	return s.repl
}

// broadcast queues changed records to be gossiped by the broadcaster.
func (s *Store) broadcast(buckets []*Bucket, objects []*Object) {
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

// objectNewer reports whether a should replace b: a later bucket generation
// always wins, then the later update.
func objectNewer(a, b *Object) bool {
	if a.Generation != b.Generation {
		return a.Generation.After(b.Generation)
	}
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
	fmt.Fprintf(h, "%s\x00%d\x00%d\x00%t", o.Key, uint64(o.Generation), uint64(o.UpdatedAt), o.IsDeleted)
	return h.Sum64()
}

func live(b *Bucket) bool { return b != nil && !b.IsDeleted }

// liveLocked reports whether o is a visible object of a live bucket.
func (s *Store) liveLocked(o *Object) bool {
	if o == nil || o.IsDeleted {
		return false
	}
	b := s.buckets[o.Bucket]
	return live(b) && b.Generation == o.Generation
}

func (s *Store) statsLocked(bucket string) *bucketStats {
	st := s.stats[bucket]
	if st == nil {
		st = &bucketStats{}
		s.stats[bucket] = st
	}
	return st
}

// refLocked adjusts a content reference count. Content whose last reference
// goes is removed by flushUnlinks once the caller has released the lock.
func (s *Store) refLocked(sha string, delta int) {
	n := s.blobRefs[sha] + delta
	if n <= 0 {
		delete(s.blobRefs, sha)
		delete(s.missing, sha)
		s.unlink = append(s.unlink, sha)
		return
	}
	s.blobRefs[sha] = n
}

// flushUnlinks removes content left unreferenced. Each file is checked again
// under the read lock, which excludes the write lock that content is
// installed and referenced under, so content written again in the meantime
// is never caught; the work is batched so writers are not held up.
func (s *Store) flushUnlinks() {
	s.mu.Lock()
	shas := s.unlink
	s.unlink = nil
	s.mu.Unlock()

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
		s.mu.RLock()
		for _, sha := range shas[:n] {
			if s.blobRefs[sha] == 0 {
				s.blobs.remove(sha)
			}
		}
		s.mu.RUnlock()
		shas = shas[n:]
	}
}

// needContentLocked takes a reference on a live object's content and starts
// fetching it if this server does not hold it.
func (s *Store) needContentLocked(o *Object) {
	s.blobRefs[o.SHA256]++
	if _, ok := s.missing[o.SHA256]; ok {
		return
	}
	if !s.blobs.has(o.SHA256) {
		s.missing[o.SHA256] = o
		if s.fetcher != nil {
			s.fetcher.enqueue(o.SHA256, o.Size, o.SourceNode)
		}
	}
}

// contentHeldLocked records that content is now stored locally.
func (s *Store) contentHeldLocked(sha string) {
	delete(s.missing, sha)
}

// ownedLocked keeps the owner index in step as a bucket record changes.
func (s *Store) ownedLocked(cur, next *Bucket) {
	if live(cur) {
		delete(s.owned[cur.OwnerId], cur.Name)
		if len(s.owned[cur.OwnerId]) == 0 {
			delete(s.owned, cur.OwnerId)
		}
	}
	if live(next) {
		set := s.owned[next.OwnerId]
		if set == nil {
			set = make(map[string]struct{})
			s.owned[next.OwnerId] = set
		}
		set[next.Name] = struct{}{}
	}
}

// applyBucketLocked merges a bucket record, returning true if it changed state.
func (s *Store) applyBucketLocked(b *Bucket) bool {
	cur := s.buckets[b.Name]
	if cur != nil && !bucketNewer(b, cur) {
		return false
	}
	next := b.clone()

	// With the same generation and state, which objects are visible cannot
	// change: only the record (its owner or grants) does.
	if cur != nil && cur.Generation == b.Generation && cur.IsDeleted == b.IsDeleted {
		s.ownedLocked(cur, next)
		s.buckets[b.Name] = next
		return true
	}

	// Objects live under the old record may change visibility.
	objs := s.objects[b.Name]
	var oldLive []*Object
	for _, o := range objs {
		if s.liveLocked(o) {
			oldLive = append(oldLive, o)
		}
	}

	s.ownedLocked(cur, next)
	s.buckets[b.Name] = next

	st := s.statsLocked(b.Name)
	st.size, st.count = 0, 0
	dropped := false
	for _, o := range objs {
		// Records from an older generation, or of a deleted bucket, can
		// never become live again.
		if s.deadGenerationLocked(next, o.Generation) {
			st.dropRecord(objs, o)
			dropped = true
			continue
		}
		if s.liveLocked(o) {
			st.size += o.Size
			st.count++
			s.needContentLocked(o)
		}
	}
	if dropped {
		st.rebuildKeys(objs)
	}

	// Release the old references only after taking the new ones.
	for _, o := range oldLive {
		s.refLocked(o.SHA256, -1)
	}
	return true
}

// deadGenerationLocked reports whether objects of generation gen in bucket b
// can never be live: an earlier generation, or the deleted one.
func (s *Store) deadGenerationLocked(b *Bucket, gen hlc.Timestamp) bool {
	return gen.Before(b.Generation) || (b.IsDeleted && !gen.After(b.Generation))
}

// applyObjectLocked merges an object record, returning true if it changed state.
func (s *Store) applyObjectLocked(o *Object) bool {
	if b := s.buckets[o.Bucket]; b != nil && s.deadGenerationLocked(b, o.Generation) {
		return false
	}

	objs := s.objects[o.Bucket]
	if objs == nil {
		objs = make(map[string]*Object)
		s.objects[o.Bucket] = objs
	}

	cur := objs[o.Key]
	if cur != nil && !objectNewer(o, cur) {
		return false
	}

	st := s.statsLocked(o.Bucket)
	if cur != nil {
		st.toggle(cur)
		if s.liveLocked(cur) {
			st.size -= cur.Size
			st.count--
		}
	} else {
		st.addKey(o.Key)
	}

	n := o.clone()
	objs[o.Key] = n
	st.toggle(n)

	// Take the new reference before dropping the old so content shared by
	// both versions is never removed in between.
	if s.liveLocked(n) {
		st.size += n.Size
		st.count++
		s.needContentLocked(n)
	}
	if cur != nil && s.liveLocked(cur) {
		s.refLocked(cur.SHA256, -1)
	}
	return true
}

// ---------------------------------------------------------------------------
// Replication entry points
// ---------------------------------------------------------------------------

// Merge applies bucket and object records received from another server,
// buckets first so objects land against the right generation.
func (s *Store) Merge(buckets []*Bucket, objects []*Object) {
	var changedB []*Bucket
	var changedO []*Object
	s.mu.Lock()
	for _, b := range buckets {
		if b != nil && ValidBucketName(b.Name) && s.applyBucketLocked(b) {
			changedB = append(changedB, b)
		}
	}
	for _, o := range objects {
		if o == nil || !ValidBucketName(o.Bucket) || !ValidKey(o.Key) || (!o.IsDeleted && len(o.SHA256) != 64) {
			continue
		}
		if s.applyObjectLocked(o) {
			changedO = append(changedO, o)
		}
	}
	s.mu.Unlock()
	s.journalAppend(changedB, changedO, false)
	s.flushUnlinks()
}

// BucketDigest summarises a bucket for anti-entropy.
type BucketDigest struct {
	Bucket *Bucket `msgpack:"bucket"`
	Digest uint64  `msgpack:"digest"`
	Count  int     `msgpack:"count"`
}

// Digests returns up to limit bucket digests, deleted buckets included, in
// name order after the given name, and the name to continue after ("" at
// the end). A limit of 0 returns them all.
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
		if st := s.stats[name]; st != nil {
			d.Digest = st.digest
			d.Count = len(st.keys)
		}
		out[name] = d
	}
	return out, next
}

// SlotDigests returns the digest and record count of each of a bucket's
// digest slots.
func (s *Store) SlotDigests(bucket string) ([]uint64, []int32) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	digests, counts := make([]uint64, DigestSlots), make([]int32, DigestSlots)
	if st := s.stats[bucket]; st != nil {
		copy(digests, st.slots[:])
		copy(counts, st.slotCounts[:])
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
	s.mu.RLock()
	defer s.mu.RUnlock()

	st := s.stats[bucket]
	if st == nil {
		return nil, ""
	}
	limit = max(limit, 1)
	var want [DigestSlots]bool
	for _, sl := range slots {
		if sl >= 0 && sl < DigestSlots {
			want[sl] = true
		}
	}

	objs := s.objects[bucket]
	var page []*Object
	for i := sort.SearchStrings(st.keys, after); i < len(st.keys); i++ {
		k := st.keys[i]
		if k == after || (slots != nil && !want[keySlot(k)]) {
			continue
		}
		if len(page) == limit {
			return page, page[len(page)-1].Key
		}
		page = append(page, objs[k].clone())
	}
	return page, ""
}

// ---------------------------------------------------------------------------
// Persistence
// ---------------------------------------------------------------------------

type journalRecord struct {
	Bucket *Bucket `json:"b,omitempty"`
	Object *Object `json:"o,omitempty"`
}

func (s *Store) load() error {
	for _, name := range []string{"snapshot.jsonl", "journal.jsonl"} {
		if err := s.loadFile(filepath.Join(s.dir, "meta", name)); err != nil {
			return err
		}
	}
	s.flushUnlinks()
	return nil
}

func (s *Store) loadFile(path string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	s.mu.Lock()
	defer s.mu.Unlock()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var rec journalRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			// A torn final line from a crash; everything before it is good.
			s.logger.Warn("skipping unreadable journal line", "file", path, "error", err)
			continue
		}
		if rec.Bucket != nil {
			s.applyBucketLocked(rec.Bucket)
		}
		if rec.Object != nil {
			s.applyObjectLocked(rec.Object)
		}
	}
	return scanner.Err()
}

func (s *Store) journalAppend(buckets []*Bucket, objects []*Object, sync bool) {
	if len(buckets) == 0 && len(objects) == 0 {
		return
	}

	var buf []byte
	for _, b := range buckets {
		line, _ := json.Marshal(journalRecord{Bucket: b})
		buf = append(append(buf, line...), '\n')
	}
	for _, o := range objects {
		line, _ := json.Marshal(journalRecord{Object: o})
		buf = append(append(buf, line...), '\n')
	}

	s.journalMu.Lock()
	defer s.journalMu.Unlock()

	if s.journal == nil {
		return
	}
	if _, err := s.journal.Write(buf); err != nil {
		s.logger.Error("failed to write journal", "error", err)
		return
	}
	if sync && !s.noSync {
		s.journal.Sync()
	}
	s.journalEntries += len(buckets) + len(objects)
}

// compact rewrites the snapshot from memory, dropping expired tombstones, and
// starts a fresh journal. The journal lock is held throughout, so a change
// made after the records are collected is written to the new journal.
func (s *Store) compact() error {
	s.journalMu.Lock()
	defer s.journalMu.Unlock()

	s.mu.Lock()
	s.dropExpiredLocked(hlc.FromTime(time.Now().Add(-TombstoneTTL)))
	s.mu.Unlock()

	// Records are replaced, never changed in place, so the collected pointers
	// can be encoded after the lock is released.
	s.mu.RLock()
	buckets := make([]*Bucket, 0, len(s.buckets))
	for _, b := range s.buckets {
		buckets = append(buckets, b)
	}
	var objects []*Object
	for _, objs := range s.objects {
		for _, o := range objs {
			objects = append(objects, o)
		}
	}
	s.mu.RUnlock()

	metaDir := filepath.Join(s.dir, "meta")
	tmpPath := filepath.Join(metaDir, "snapshot.tmp")
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	w := bufio.NewWriterSize(f, 256*1024)
	enc := json.NewEncoder(w)
	for _, b := range buckets {
		enc.Encode(journalRecord{Bucket: b})
	}
	for _, o := range objects {
		enc.Encode(journalRecord{Object: o})
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	f.Close()

	if err := os.Rename(tmpPath, filepath.Join(metaDir, "snapshot.jsonl")); err != nil {
		return err
	}

	if s.journal != nil {
		s.journal.Close()
	}
	s.journal, err = os.OpenFile(filepath.Join(metaDir, "journal.jsonl"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY|os.O_APPEND, 0600)
	s.journalEntries = 0
	return err
}

// dropExpiredLocked forgets buckets and objects deleted before expiry.
func (s *Store) dropExpiredLocked(expiry hlc.Timestamp) {
	for name, b := range s.buckets {
		if b.IsDeleted && b.UpdatedAt.Before(expiry) {
			delete(s.buckets, name)
			delete(s.objects, name)
			delete(s.stats, name)
		}
	}
	for name, objs := range s.objects {
		st := s.statsLocked(name)
		dropped := false
		for _, o := range objs {
			if o.IsDeleted && o.UpdatedAt.Before(expiry) {
				st.dropRecord(objs, o)
				dropped = true
			}
		}
		if dropped {
			st.rebuildKeys(objs)
		}
	}
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

// cleanup removes everything on disk no longer needed: content no live
// object references, temporary files of abandoned writes and fetches,
// multipart uploads that are stale or whose bucket is gone, and empty
// content directories. It runs at start-up and every sweepInterval.
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

// removeOrphanBlobs deletes content no live object references, checked in
// batches under the read lock as flushUnlinks does.
func (s *Store) removeOrphanBlobs() int {
	var orphans []string
	s.blobs.walk(func(sha string) {
		orphans = append(orphans, sha)
	})

	removed := 0
	for len(orphans) > 0 {
		n := min(len(orphans), unlinkBatch)
		s.mu.RLock()
		for _, sha := range orphans[:n] {
			if s.blobRefs[sha] == 0 {
				s.blobs.remove(sha)
				removed++
			}
		}
		s.mu.RUnlock()
		orphans = orphans[n:]
	}
	return removed
}

func (s *Store) maintenance() {
	defer s.wg.Done()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	lastSweep := time.Now()

	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
		}

		// Queue any content still missing, e.g. after a failed fetch.
		s.queueMissing()

		s.mu.RLock()
		records := 0
		for _, st := range s.stats {
			records += len(st.keys)
		}
		s.mu.RUnlock()

		s.journalMu.Lock()
		entries := s.journalEntries
		s.journalMu.Unlock()
		if entries > compactMinEntries && entries > 2*records {
			if err := s.compact(); err != nil {
				s.logger.Error("failed to compact metadata", "error", err)
			}
		}

		if time.Since(lastSweep) >= sweepInterval {
			lastSweep = time.Now()
			if st := s.cleanup(); st != (cleanupStats{}) {
				s.logger.Info("removed unused files", "content", st.Blobs, "temporary", st.TempFiles, "uploads", st.Uploads)
			}
		}
	}
}

// queueMissing schedules a fetch of the content still missing, forgetting
// content that has arrived.
func (s *Store) queueMissing() {
	s.mu.RLock()
	todo := make([]*Object, 0, len(s.missing))
	for _, o := range s.missing {
		todo = append(todo, o)
	}
	s.mu.RUnlock()

	for _, o := range todo {
		if s.blobs.has(o.SHA256) {
			s.mu.Lock()
			s.contentHeldLocked(o.SHA256)
			s.mu.Unlock()
		} else if s.fetcher != nil {
			s.fetcher.enqueue(o.SHA256, o.Size, o.SourceNode)
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
	b := s.buckets[name]
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

func (s *Store) liveObjectLocked(bucket, key string) *Object {
	o := s.objects[bucket][key]
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
		if st := s.stats[name]; st != nil {
			size += st.size
			count += st.count
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
// limit, for comparing under the lock with roomLocked.
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
