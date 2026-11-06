package filestore

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/paularlott/gossip/hlc"
)

// SnapshotManifest is the file a snapshot writes last into the folder it
// makes: a folder without one is a snapshot not yet complete, or one that
// failed and is being taken away.
const SnapshotManifest = ".knot-snapshot.json"

// ErrSnapshotChanged is a snapshot that could not be made because files in
// the folder kept being replaced as it was made, and their content went
// before it could be shared.
var ErrSnapshotChanged = errors.New("files in the folder were replaced as the snapshot was made: try again")

// errSourceGone is a file read for a snapshot whose content went before
// the snapshot's record of it was written.
var errSourceGone = errors.New("snapshot source content gone")

// beforeSnapshotWrite, for tests, runs between a snapshot's read and its
// write.
var beforeSnapshotWrite func()

// snapshotAttempts is how many times a snapshot is tried while files it read
// are replaced and their content dropped before it is written.
const snapshotAttempts = 3

// Snapshot is a snapshot made: the files it holds, their size, and when the
// folder was read.
type Snapshot struct {
	Files   int       `json:"files"`
	Folders int       `json:"folders"`
	Bytes   int64     `json:"bytes"`
	TakenAt time.Time `json:"taken_at"`
	// Where it was taken from, as the manifest records it.
	SourceBucket string `json:"source_bucket"`
	SourcePath   string `json:"source_path"`
}

// SnapshotFolder copies a folder and everything below it, as it was at one
// moment, to a new folder, in the same bucket or another, without copying
// any content: every file is read in one database transaction, so the copy
// holds no file older or newer than the rest, however much is written to
// the folder as it is copied. from "" is the whole bucket.
//
// The destination must not exist. Its files are written in batches, so
// they appear over a moment; the manifest (SnapshotManifest) goes last, and
// a failure takes away what was written. The destination owner is charged
// for every file, as for a copy.
func (s *Store) SnapshotFolder(p *Principal, srcBucket, from, dstBucket, to string) (*Snapshot, error) {
	from, to = strings.Trim(from, "/"), strings.Trim(to, "/")
	if (from != "" && !ValidKey(from)) || !ValidKey(to) {
		return nil, ErrInvalidKey
	}
	if !ValidKey(to + "/" + SnapshotManifest) {
		return nil, ErrInvalidKey
	}
	_, limit, err := s.quotaFor(p, dstBucket, AccessWrite)
	if err != nil {
		return nil, err
	}

	for attempt := 1; ; attempt++ {
		snap, written, err := s.snapshotOnce(p, srcBucket, from, dstBucket, to, limit)
		if errors.Is(err, errSourceGone) {
			if attempt < snapshotAttempts {
				continue
			}
			return nil, ErrSnapshotChanged
		}
		if err != nil {
			return nil, err
		}

		// The manifest, last: the snapshot is whole once it is there.
		manifest, _ := json.MarshalIndent(snap, "", "  ")
		manifest = append(manifest, '\n')
		if _, err := s.PutObject(p, dstBucket, to+"/"+SnapshotManifest, bytes.NewReader(manifest), PutOptions{
			ContentType: "application/json",
			Size:        int64(len(manifest)),
			IfNoneMatch: true,
		}); err != nil {
			s.mu.RLock()
			s.takeBack(written)
			s.mu.RUnlock()
			if errors.Is(err, ErrPrecondition) {
				err = ErrDestinationExists
			}
			return nil, err
		}
		return snap, nil
	}
}

// snapshotOnce reads the folder and writes its copy, returning the records
// written. On a failure it takes back what it wrote.
func (s *Store) snapshotOnce(p *Principal, srcBucket, from, dstBucket, to string, limit int64) (*Snapshot, []*Object, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sb, err := s.bucketForLocked(p, srcBucket, AccessRead)
	if err != nil {
		return nil, nil, err
	}
	db, err := s.bucketForLocked(p, dstBucket, AccessWrite)
	if err != nil {
		return nil, nil, err
	}
	if sb.Id == db.Id && (from == "" || to == from || strings.HasPrefix(to, from+"/")) {
		// A folder cannot be copied into itself.
		return nil, nil, ErrInvalidKey
	}

	// The folder, as it is now, in one read; and the destination, empty.
	srcPrefix := ""
	if from != "" {
		srcPrefix = from + "/"
	}
	snap := &Snapshot{SourceBucket: sb.Name, SourcePath: from}
	var files []*Object
	err = s.db.View(func(txn *badger.Txn) error {
		snap.TakenAt = time.Now().UTC()
		base := objectPrefix(sb.Id)
		it := txn.NewIterator(badger.IteratorOptions{PrefetchValues: true, PrefetchSize: 100, Prefix: append(append([]byte(nil), base...), srcPrefix...)})
		for it.Rewind(); it.Valid(); it.Next() {
			var o *Object
			if err := it.Item().Value(func(v []byte) (err error) { o, err = decodeObject(v); return }); err != nil {
				it.Close()
				return err
			}
			// The source's own manifest, when it is a snapshot, is replaced
			// by this one's.
			if !s.liveLocked(o) || o.Key == srcPrefix+SnapshotManifest {
				continue
			}
			files = append(files, o)
		}
		it.Close()

		if cur, err := getObject(txn, db.Id, to); err != nil {
			return err
		} else if s.liveLocked(cur) {
			return ErrDestinationExists
		}
		dst := txn.NewIterator(badger.IteratorOptions{Prefix: objectKey(db.Id, to+"/")})
		defer dst.Close()
		for dst.Rewind(); dst.Valid(); dst.Next() {
			var o *Object
			if err := dst.Item().Value(func(v []byte) (err error) { o, err = decodeObject(v); return }); err != nil {
				return err
			}
			if s.liveLocked(o) {
				return ErrDestinationExists
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if len(files) == 0 {
		return nil, nil, ErrNoSuchKey
	}

	ops := make([]objectOp, 0, len(files))
	var total int64
	for _, src := range files {
		o := src.clone()
		o.Writer = p.Writer
		o.BucketId = db.Id
		o.Key = to + "/" + src.Key[len(srcPrefix):]
		if !ValidKey(o.Key) {
			return nil, nil, ErrInvalidKey
		}
		o.UpdatedAt = hlc.Now()
		if !s.isMissing(src.SHA256) {
			o.SourceNode = s.nodeId
		}
		if strings.HasSuffix(o.Key, "/") {
			snap.Folders++
		} else {
			snap.Files++
		}
		total += o.Size
		sha := o.SHA256
		ops = append(ops, objectOp{
			obj: o,
			pre: func(txn *badger.Txn, cur *Object) error {
				if s.liveLocked(cur) {
					return ErrDestinationExists
				}
				// The file read may have been replaced since, and its content
				// let go: then there is nothing to share.
				if sha != "" {
					refs, err := getRef(txn, sha)
					if err != nil {
						return err
					}
					if refs == 0 {
						return errSourceGone
					}
				}
				return nil
			},
		})
	}
	snap.Bytes = total

	if beforeSnapshotWrite != nil {
		beforeSnapshotWrite()
	}
	release, err := s.reserveLocked(db.OwnerId, limit, total)
	if err != nil {
		return nil, nil, err
	}
	defer release()

	written, err := s.applyObjects(ops)
	if len(written) > 0 {
		s.commit(nil, written)
	}
	if err != nil {
		s.takeBack(written)
		return nil, nil, err
	}
	return snap, written, nil
}

// takeBack deletes the records a failed snapshot wrote, each only while it
// is still the one written: a file written there since is left. s.mu is
// held for reading.
func (s *Store) takeBack(written []*Object) {
	for try := 0; try < 5 && len(written) > 0; try++ {
		// Those still as written; a check in the transaction catches one
		// replaced between this look and the delete, and the rest are
		// looked at again.
		var ours []*Object
		err := s.db.View(func(txn *badger.Txn) error {
			for _, w := range written {
				cur, err := getObject(txn, w.BucketId, w.Key)
				if err != nil {
					return err
				}
				if cur != nil && !cur.IsDeleted && cur.UpdatedAt == w.UpdatedAt {
					ours = append(ours, w)
				}
			}
			return nil
		})
		if err != nil {
			s.logger.Error("file storage: taking back a failed snapshot", "error", err)
			return
		}
		ops := make([]objectOp, 0, len(ours))
		for _, w := range ours {
			stamp := w.UpdatedAt
			ops = append(ops, objectOp{
				obj: &Object{
					BucketId:   w.BucketId,
					Key:        w.Key,
					ModifiedAt: time.Now().UTC(),
					SourceNode: s.nodeId,
					UpdatedAt:  hlc.Now(),
					IsDeleted:  true,
					Writer:     w.Writer,
				},
				pre: func(txn *badger.Txn, cur *Object) error {
					if cur == nil || cur.UpdatedAt != stamp {
						return errNotOurs
					}
					return nil
				},
			})
		}
		gone, err := s.applyObjects(ops)
		if len(gone) > 0 {
			s.commit(nil, gone)
		}
		if err == nil {
			return
		}
		if !errors.Is(err, errNotOurs) {
			s.logger.Error("file storage: taking back a failed snapshot", "error", err)
			return
		}
		written = ours
	}
}

// errNotOurs is a record replaced since a failed snapshot wrote it: it is
// left as it is.
var errNotOurs = errors.New("record replaced since written")
