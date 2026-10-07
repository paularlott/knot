package filestore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"

	badger "github.com/dgraph-io/badger/v4"
)

// What a check can find.
const (
	FsckMissingContent = "missing-content"      // a file's content is not held here
	FsckCorruptContent = "corrupt-content"      // held content that is the wrong size or checksum
	FsckRefCount       = "reference-count"      // a content reference count that is wrong
	FsckOrphanRef      = "orphan-reference"     // a reference count with no file behind it
	FsckOrphanInline   = "orphan-content"       // inline content no file refers to
	FsckOrphanFile     = "orphan-content-file"  // a content file no file refers to
	FsckMissingMarker  = "missing-marker"       // content missing but not queued to be fetched
	FsckStaleMarker    = "stale-marker"         // content queued to be fetched that is held or unwanted
	FsckTombstone      = "tombstone-index"      // the index of deletion records out of step
	FsckStats          = "statistics"           // a bucket's size, count or digest that is wrong
	FsckDeadRecords    = "deleted-bucket-files" // records of a deleted bucket not yet removed
	FsckUnknownBucket  = "unknown-bucket-files" // records of a bucket this server has no record of
	FsckOwnerDeleted   = "owner-deleted"        // a bucket whose owner was deleted
	FsckOwnerMissing   = "owner-missing"        // a bucket whose owner is not in the user database
)

// maxFsckIssues is how many individual findings a report lists; the counts
// are complete.
const maxFsckIssues = 200

// FsckOptions says what a check does beyond looking.
type FsckOptions struct {
	// Repair fixes what it can: content is fetched from the cluster, counts
	// and indexes are rebuilt, buckets of deleted users are removed.
	Repair bool `json:"repair"`
	// Deep reads every stored content and checks its checksum.
	Deep bool `json:"deep"`
}

// FsckIssue is one finding.
type FsckIssue struct {
	Kind   string `json:"kind"`
	Bucket string `json:"bucket,omitempty"`
	Key    string `json:"key,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// FsckReport is what a check found and fixed.
type FsckReport struct {
	Buckets  int            `json:"buckets"`
	Files    int            `json:"files"`
	Found    map[string]int `json:"found"`
	Repaired map[string]int `json:"repaired"`
	Issues   []FsckIssue    `json:"issues"`
	Notes    []string       `json:"notes,omitempty"`
}

// Unresolved is how many findings remain after any repair.
func (r *FsckReport) Unresolved() int {
	n := 0
	for kind, found := range r.Found {
		n += found - r.Repaired[kind]
	}
	return n
}

func (r *FsckReport) add(kind, bucket, key, detail string) {
	r.Found[kind]++
	if len(r.Issues) < maxFsckIssues {
		r.Issues = append(r.Issues, FsckIssue{Kind: kind, Bucket: bucket, Key: key, Detail: detail})
	}
}

func (r *FsckReport) note(format string, args ...any) {
	r.Notes = append(r.Notes, fmt.Sprintf(format, args...))
}

// fsckRef is one file that uses some content.
type fsckRef struct {
	bucketId string
	key      string
	size     int64
	source   string
	files    int // how many files use it
}

// fsckScan is what a pass over the database finds. Content is keyed by its
// checksum as 32 raw bytes.
type fsckScan struct {
	stats     map[string]*bucketStats // by bucket id, recounted from the records
	refs      map[string]*fsckRef     // content the files use
	have      map[string]uint32       // reference counts held
	inline    map[string]int64        // inline content, with its size
	markers   map[string]bool         // content queued to be fetched
	tombs     map[string]bool         // deletion index entries expected
	haveTombs map[string]bool         // and found
}

// fsckScanLocked reads every record in one consistent snapshot. The caller
// holds s.mu, shared or exclusive, so the buckets do not change under it.
func (s *Store) fsckScanLocked() (*fsckScan, error) {
	sc := &fsckScan{
		stats:     make(map[string]*bucketStats),
		refs:      make(map[string]*fsckRef),
		have:      make(map[string]uint32),
		inline:    make(map[string]int64),
		markers:   make(map[string]bool),
		tombs:     make(map[string]bool),
		haveTombs: make(map[string]bool),
	}
	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.IteratorOptions{PrefetchValues: true, PrefetchSize: 1000, Prefix: []byte{'o'}})
		for it.Rewind(); it.Valid(); it.Next() {
			var o *Object
			if err := it.Item().Value(func(v []byte) (err error) { o, err = decodeObject(v); return }); err != nil {
				it.Close()
				return err
			}
			st := sc.stats[o.BucketId]
			if st == nil {
				st = &bucketStats{}
				sc.stats[o.BucketId] = st
			}
			st.records++
			st.slotCounts[keySlot(o.Key)]++
			st.toggle(o)
			if o.IsDeleted {
				sc.tombs[string(tombKey(o.UpdatedAt, o.BucketId, o.Key))] = true
				continue
			}
			if s.liveLocked(o) {
				st.size += o.Size
				st.count++
			}
			raw := string(rawSha(o.SHA256))
			if r := sc.refs[raw]; r != nil {
				r.files++
			} else {
				sc.refs[raw] = &fsckRef{bucketId: o.BucketId, key: o.Key, size: o.Size, source: o.SourceNode, files: 1}
			}
		}
		it.Close()

		for _, kind := range []byte{'r', 'c', 'm', 't'} {
			it = txn.NewIterator(badger.IteratorOptions{PrefetchValues: kind == 'r', Prefix: []byte{kind}})
			for it.Rewind(); it.Valid(); it.Next() {
				item := it.Item()
				switch kind {
				case 'r':
					var n uint32
					if err := item.Value(func(v []byte) error {
						if len(v) == 4 {
							n = uint32(v[0]) | uint32(v[1])<<8 | uint32(v[2])<<16 | uint32(v[3])<<24
						}
						return nil
					}); err != nil {
						it.Close()
						return err
					}
					sc.have[string(item.Key()[1:])] = n
				case 'c':
					sc.inline[string(item.Key()[1:])] = int64(item.ValueSize())
				case 'm':
					sc.markers[string(item.Key()[1:])] = true
				case 't':
					sc.haveTombs[string(item.KeyCopy(nil))] = true
				}
			}
			it.Close()
		}
		return nil
	})
	return sc, err
}

func rawSha(sha string) []byte {
	raw, _ := hex.DecodeString(sha)
	return raw
}

// Fsck checks the store for damage: content that is missing or wrong, counts
// and indexes that disagree with the records, and buckets of users that are
// gone. With Repair it fixes what it can, fetching content from the other
// servers when there are any. It can run while the store is in use; fixing
// counts and indexes pauses writes for as long as a pass over the records
// takes.
func (s *Store) Fsck(ctx context.Context, opt FsckOptions) (*FsckReport, error) {
	rep := &FsckReport{Found: make(map[string]int), Repaired: make(map[string]int)}

	s.mu.RLock()
	buckets := make(map[string]*Bucket, len(s.buckets))
	for id, b := range s.buckets {
		buckets[id] = b
	}
	seq := s.writeSeq.Load()
	sc, err := s.fsckScanLocked()
	busy := s.writeSeq.Load() != seq
	// The statistics in memory, as they stood with the scan.
	memStats := make(map[string]persistedStats)
	for id := range buckets {
		if st := s.statsIfAny(id); st != nil {
			st.mu.Lock()
			memStats[id] = persistedStats{Size: st.size, Count: st.count, Records: st.records, Digest: st.digest}
			st.mu.Unlock()
		}
	}
	s.mu.RUnlock()
	if err != nil {
		return rep, err
	}

	for _, b := range buckets {
		if live(b) {
			rep.Buckets++
		}
	}
	for _, st := range sc.stats {
		rep.Files += st.count
	}

	name := func(id string) string {
		if b := buckets[id]; b != nil {
			return b.Name
		}
		return id
	}

	// Content.
	var missing, corrupt []string
	shas := make([]string, 0, len(sc.refs))
	for raw := range sc.refs {
		shas = append(shas, raw)
	}
	sort.Strings(shas)
	for _, raw := range shas {
		ref := sc.refs[raw]
		hexSha := hex.EncodeToString([]byte(raw))
		size, inline := sc.inline[raw]
		held := inline
		if !inline {
			if info, err := os.Stat(s.blobs.path(hexSha)); err == nil {
				held, size = true, info.Size()
			}
		}
		where := fmt.Sprintf("content %s used by %d files", hexSha[:12], ref.files)
		switch {
		case !held:
			missing = append(missing, raw)
			rep.add(FsckMissingContent, name(ref.bucketId), ref.key, where)
		case size != ref.size:
			corrupt = append(corrupt, raw)
			rep.add(FsckCorruptContent, name(ref.bucketId), ref.key, fmt.Sprintf("%s is %d bytes, the file says %d", where, size, ref.size))
		case opt.Deep:
			if ok, err := s.contentSumMatches(hexSha); err == nil && !ok {
				corrupt = append(corrupt, raw)
				rep.add(FsckCorruptContent, name(ref.bucketId), ref.key, where+" does not match its checksum")
			}
		}
	}

	// Reference counts, queued content and the deletion index.
	structural := 0
	for raw, ref := range sc.refs {
		if got := sc.have[raw]; got != uint32(ref.files) {
			structural++
			rep.add(FsckRefCount, name(ref.bucketId), ref.key, fmt.Sprintf("content %s has %d files and a count of %d", hex.EncodeToString([]byte(raw))[:12], ref.files, got))
		}
	}
	for raw := range sc.have {
		if sc.refs[raw] == nil {
			structural++
			rep.add(FsckOrphanRef, "", "", "count for content "+hex.EncodeToString([]byte(raw))[:12]+" that no file uses")
		}
	}
	for raw := range sc.inline {
		if sc.refs[raw] == nil {
			structural++
			rep.add(FsckOrphanInline, "", "", "content "+hex.EncodeToString([]byte(raw))[:12]+" that no file uses")
		}
	}
	missingSet := make(map[string]bool, len(missing)+len(corrupt))
	for _, raw := range missing {
		missingSet[raw] = true
		if !sc.markers[raw] {
			structural++
			rep.add(FsckMissingMarker, "", "", "content "+hex.EncodeToString([]byte(raw))[:12]+" is not queued to be fetched")
		}
	}
	for raw := range sc.markers {
		if !missingSet[raw] {
			structural++
			rep.add(FsckStaleMarker, "", "", "content "+hex.EncodeToString([]byte(raw))[:12]+" is queued but held or unwanted")
		}
	}
	for k := range sc.tombs {
		if !sc.haveTombs[k] {
			structural++
			rep.add(FsckTombstone, "", "", "a deletion record is not in the expiry index")
		}
	}
	for k := range sc.haveTombs {
		if !sc.tombs[k] {
			structural++
			rep.add(FsckTombstone, "", "", "the expiry index lists a record that is gone")
		}
	}

	// Buckets: records of deleted or unknown ones, and statistics.
	var deadBuckets []string
	for id, st := range sc.stats {
		b := buckets[id]
		switch {
		case b == nil:
			rep.add(FsckUnknownBucket, id, "", fmt.Sprintf("%d records of a bucket this server has no record of; they are kept in case it arrives", st.records))
		case b.IsDeleted:
			deadBuckets = append(deadBuckets, id)
			rep.add(FsckDeadRecords, b.Name, "", fmt.Sprintf("%d records of a deleted bucket", st.records))
		}
	}
	if busy {
		rep.note("files were written during the check, so statistics were not compared")
	} else {
		for id, b := range buckets {
			want := sc.stats[id]
			if want == nil {
				want = &bucketStats{}
			}
			got := memStats[id]
			if got.Size != want.size || got.Count != want.count || got.Records != want.records || got.Digest != want.digest {
				structural++
				rep.add(FsckStats, b.Name, "", fmt.Sprintf("held as %d files, %d bytes; the records say %d files, %d bytes", got.Count, got.Size, want.count, want.size))
			}
		}
	}

	// Content files nothing uses.
	orphanFiles := 0
	s.blobs.walk(func(sha string) {
		if sc.refs[string(rawSha(sha))] == nil {
			orphanFiles++
		}
	})
	if orphanFiles > 0 {
		rep.Found[FsckOrphanFile] += orphanFiles
		if len(rep.Issues) < maxFsckIssues {
			rep.Issues = append(rep.Issues, FsckIssue{Kind: FsckOrphanFile, Detail: fmt.Sprintf("%d content files that no file uses", orphanFiles)})
		}
	}

	// Owners.
	owners := s.checkOwners()
	for _, id := range owners.Deleted {
		rep.add(FsckOwnerDeleted, "", "", "buckets owned by deleted user "+id)
	}
	for _, id := range owners.Missing {
		rep.add(FsckOwnerMissing, "", "", "buckets owned by user "+id+", who is not in the user database")
	}
	if owners.Unknown > 0 {
		rep.note("the user database could not say whether %d bucket owners exist", owners.Unknown)
	}
	if s.ownerState == nil {
		rep.note("bucket owners were not checked: no user database is connected")
	}

	if !opt.Repair {
		return rep, ctx.Err()
	}

	// Repairs, in an order that leaves the next one a true picture.
	if len(owners.Deleted) > 0 {
		s.reconcileOwners()
		rep.Repaired[FsckOwnerDeleted] += len(owners.Deleted)
	}
	for _, id := range deadBuckets {
		s.kickSweep(id)
		rep.Repaired[FsckDeadRecords]++
	}
	if structural > 0 {
		if err := s.fsckRepairStructure(rep); err != nil {
			return rep, err
		}
	}
	if orphanFiles > 0 {
		n := s.removeOrphanBlobs()
		rep.Repaired[FsckOrphanFile] += n
	}

	// Content comes from the cluster.
	if len(corrupt) > 0 {
		for _, raw := range corrupt {
			s.discardContent(hex.EncodeToString([]byte(raw)))
		}
	}
	todo := append(append([]string(nil), missing...), corrupt...)
	if len(todo) > 0 {
		if s.replicator() == nil {
			rep.note("content cannot be fetched: this server has no cluster to fetch from")
			return rep, ctx.Err()
		}
		fetched := s.fsckFetch(ctx, sc, todo)
		for _, raw := range todo {
			if fetched[raw] {
				if missingSet[raw] {
					rep.Repaired[FsckMissingContent]++
				} else {
					rep.Repaired[FsckCorruptContent]++
				}
			}
		}
		if lost := len(todo) - len(fetched); lost > 0 {
			rep.note("%d contents could not be fetched from any other server; delete the files that use them, or restore the content from a backup", lost)
		}
	}
	return rep, ctx.Err()
}

// fsckFetch fetches content from the other servers, a few at a time,
// returning what arrived.
func (s *Store) fsckFetch(ctx context.Context, sc *fsckScan, todo []string) map[string]bool {
	var mu sync.Mutex
	fetched := make(map[string]bool)
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, raw := range todo {
		select {
		case <-ctx.Done():
			wg.Wait()
			return fetched
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(raw string) {
			defer wg.Done()
			defer func() { <-sem }()
			ref := sc.refs[raw]
			sha := hex.EncodeToString([]byte(raw))
			if err := s.fetcher.wait(ctx, sha, ref.size, ref.source); err == nil && s.holds(sha) {
				mu.Lock()
				fetched[raw] = true
				mu.Unlock()
			}
		}(raw)
	}
	wg.Wait()
	return fetched
}

// discardContent removes held content that is wrong, so it can be fetched
// again.
func (s *Store) discardContent(sha string) {
	s.db.Update(func(txn *badger.Txn) error { return txn.Delete(shaKey('c', sha)) })
	s.blobMu.Lock()
	s.blobs.remove(sha)
	s.blobMu.Unlock()
}

// contentSumMatches reads stored content and checks its checksum.
func (s *Store) contentSumMatches(sha string) (bool, error) {
	c, err := s.openContent(sha)
	if err != nil {
		return false, err
	}
	defer c.Close()
	h := sha256.New()
	if _, err := io.Copy(h, c); err != nil {
		return false, err
	}
	return hex.EncodeToString(h.Sum(nil)) == sha, nil
}

// fsckRepairStructure rebuilds the reference counts, the queue of content to
// fetch, the deletion index and the statistics from the records. It holds
// the store exclusively, so the records cannot change under it.
func (s *Store) fsckRepairStructure(rep *FsckReport) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sc, err := s.fsckScanLocked()
	if err != nil {
		return err
	}

	var ops []func(txn *badger.Txn) error
	fixed := func(kind string) { rep.Repaired[kind]++ }
	rawKey := func(kind byte, raw string) []byte { return append([]byte{kind}, raw...) }

	for raw, ref := range sc.refs {
		raw, ref := raw, ref
		if sc.have[raw] != uint32(ref.files) {
			ops = append(ops, func(txn *badger.Txn) error {
				var v [4]byte
				n := uint32(ref.files)
				v[0], v[1], v[2], v[3] = byte(n), byte(n>>8), byte(n>>16), byte(n>>24)
				return txn.Set(rawKey('r', raw), v[:])
			})
			fixed(FsckRefCount)
		}
	}
	for raw := range sc.have {
		raw := raw
		if sc.refs[raw] == nil {
			ops = append(ops, func(txn *badger.Txn) error { return txn.Delete(rawKey('r', raw)) })
			fixed(FsckOrphanRef)
		}
	}
	for raw := range sc.inline {
		raw := raw
		if sc.refs[raw] == nil {
			ops = append(ops, func(txn *badger.Txn) error { return txn.Delete(rawKey('c', raw)) })
			fixed(FsckOrphanInline)
		}
	}

	// Queue content that is missing, and forget what no longer needs it.
	wanted := make(map[string]bool)
	for raw, ref := range sc.refs {
		raw, ref := raw, ref
		_, inline := sc.inline[raw]
		if inline || s.blobs.has(hex.EncodeToString([]byte(raw))) {
			continue
		}
		wanted[raw] = true
		if !sc.markers[raw] {
			ops = append(ops, func(txn *badger.Txn) error {
				o, err := getObject(txn, ref.bucketId, ref.key)
				if err != nil || o == nil {
					return err
				}
				return txn.Set(rawKey('m', raw), encodeRecord(o))
			})
			fixed(FsckMissingMarker)
		}
	}
	for raw := range sc.markers {
		raw := raw
		if !wanted[raw] {
			ops = append(ops, func(txn *badger.Txn) error { return txn.Delete(rawKey('m', raw)) })
			fixed(FsckStaleMarker)
		}
	}

	for k := range sc.tombs {
		k := k
		if !sc.haveTombs[k] {
			ops = append(ops, func(txn *badger.Txn) error { return txn.Set([]byte(k), []byte{}) })
			fixed(FsckTombstone)
		}
	}
	for k := range sc.haveTombs {
		k := k
		if !sc.tombs[k] {
			ops = append(ops, func(txn *badger.Txn) error { return txn.Delete([]byte(k)) })
			fixed(FsckTombstone)
		}
	}

	for len(ops) > 0 {
		n := min(len(ops), writeChunk)
		chunk := ops[:n]
		if err := s.db.Update(func(txn *badger.Txn) error {
			for _, op := range chunk {
				if err := op(txn); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
		ops = ops[n:]
	}

	// The queue in memory follows the database.
	s.missingMu.Lock()
	for raw := range s.missing {
		if !wanted[string(rawSha(raw))] {
			delete(s.missing, raw)
		}
	}
	s.missingMu.Unlock()
	if err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.IteratorOptions{PrefetchValues: true, Prefix: []byte{'m'}})
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			var o *Object
			if err := it.Item().Value(func(v []byte) (err error) { o, err = decodeObject(v); return }); err != nil {
				return err
			}
			s.missingMu.Lock()
			s.missing[o.SHA256] = o
			s.missingMu.Unlock()
		}
		return nil
	}); err != nil {
		return err
	}

	// Statistics.
	for id := range s.buckets {
		want := sc.stats[id]
		if want == nil {
			want = &bucketStats{}
		}
		st := s.statsFor(id)
		st.mu.Lock()
		if st.size != want.size || st.count != want.count || st.records != want.records || st.digest != want.digest || st.slots != want.slots || st.slotCounts != want.slotCounts {
			if st.size != want.size || st.count != want.count || st.records != want.records || st.digest != want.digest {
				fixed(FsckStats)
			}
			st.size, st.count, st.records = want.size, want.count, want.records
			st.digest, st.slots, st.slotCounts = want.digest, want.slots, want.slotCounts
		}
		st.mu.Unlock()
	}
	return nil
}
