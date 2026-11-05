package command_files

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
)

// A two-way sync keeps, between runs, the state both sides last agreed on:
// for each file, its content in the bucket and its size and modification
// time here. Without it a file deleted on one side looks the same as one
// created on the other. It lives in knot's sync folder (syncDir) or where
// --state-file says, as a record file (recordFile): a pass that changes a
// few files appends a few records.
//
// A state that is lost costs nothing but a first run: nothing is deleted,
// files that differ are kept both ways. One damaged keeps the files read
// before the damage, and those after it are synced as on a first run. One
// that has gone back to an earlier pass, its last changes lost to a power
// cut, is safe the same way: a file the same on both sides is agreed on
// whatever the state says, one gone from both is forgotten, and one the
// state does not know is copied to the side without it.

// stateMagic and stateVersion mark a two-way state.
const (
	stateMagic   = "KNSS"
	stateVersion = 1
)

type baseEntry struct {
	Key        string // the file's key in the bucket
	SHA        string // the content both sides held
	Size       int64  // its size
	LocalSize  int64  // the local file's size then
	LocalMtime int64  // and its modification time, unix nanoseconds
	Mode       uint32 // the permission bits both held, 0 when not known
	// The local file's inode number and change time then, where the
	// platform has them (see fileStamp).
	LocalIno   uint64
	LocalCtime int64
}

// matchesLocal reports whether the local file is as it was when both sides
// last agreed, without reading it.
func (b *baseEntry) matchesLocal(l *localFile) bool {
	return b.LocalSize == l.size && b.LocalMtime == l.mtime.UnixNano() &&
		stampMatches(fileStamp{ino: b.LocalIno, ctime: b.LocalCtime}, l.stamp)
}

// The flags of a state record: the key is the prefix and the path (as it
// nearly always is), and the checksum is kept as its 32 bytes rather than
// as text.
const (
	baseKeyIsPath byte = 1 << iota
	baseRawSHA
)

// baseCodec writes and reads state entries of a bucket folder with prefix.
func baseCodec(prefix string) recordCodec[baseEntry] {
	return recordCodec[baseEntry]{
		put: func(b []byte, path string, e baseEntry) []byte {
			var flags byte
			if e.Key == prefix+path {
				flags |= baseKeyIsPath
			}
			var raw [sha256.Size]byte
			if n, err := hex.Decode(raw[:], []byte(e.SHA)); err == nil && n == sha256.Size && hex.EncodeToString(raw[:]) == e.SHA {
				flags |= baseRawSHA
			}
			b = append(b, flags)
			if flags&baseKeyIsPath == 0 {
				b = binary.AppendUvarint(b, uint64(len(e.Key)))
				b = append(b, e.Key...)
			}
			if flags&baseRawSHA != 0 {
				b = append(b, raw[:]...)
			} else {
				b = binary.AppendUvarint(b, uint64(len(e.SHA)))
				b = append(b, e.SHA...)
			}
			b = binary.AppendVarint(b, e.Size)
			b = binary.AppendVarint(b, e.LocalSize)
			b = binary.AppendVarint(b, e.LocalMtime)
			b = binary.AppendUvarint(b, uint64(e.Mode))
			b = binary.AppendUvarint(b, e.LocalIno)
			return binary.AppendVarint(b, e.LocalCtime)
		},
		get: func(r *recReader, path string) baseEntry {
			var e baseEntry
			flags := r.byte()
			if flags&^(baseKeyIsPath|baseRawSHA) != 0 {
				r.bad = true
				return e
			}
			if flags&baseKeyIsPath != 0 {
				e.Key = prefix + path
			} else {
				e.Key = r.str()
			}
			if flags&baseRawSHA != 0 {
				e.SHA = hex.EncodeToString(r.take(sha256.Size))
			} else {
				e.SHA = r.str()
			}
			e.Size = r.varint()
			e.LocalSize = r.varint()
			e.LocalMtime = r.varint()
			mode := r.uvarint()
			if mode > 0xffffffff {
				r.bad = true
			}
			e.Mode = uint32(mode)
			e.LocalIno = r.uvarint()
			e.LocalCtime = r.varint()
			return e
		},
	}
}

// stateIdentity is what a state is of, in its header.
func stateIdentity(bucket, prefix, dir string) string {
	return bucket + "\x00" + prefix + "\x00" + dir
}

// defaultStatePath is where the state of a two-way sync of a bucket folder
// and a directory is kept.
func defaultStatePath(server, bucket, prefix, dir string) (string, error) {
	sd, err := syncDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(server + "\x00" + bucket + "\x00" + prefix + "\x00" + dir))
	return filepath.Join(sd, "state-"+hex.EncodeToString(sum[:12])+".bin"), nil
}

// loadBase reads the state of an earlier run. A missing file is a first run:
// nothing is deleted on it, and files that differ are kept both ways. A
// damaged one is a first run for the files lost to the damage.
func (s *syncer) loadBase(bucketName string) error {
	if s.opt.statePath == "" {
		p, err := defaultStatePath(s.client.GetBaseURL(), bucketName, s.opt.prefix, s.opt.dir)
		if err != nil {
			return err
		}
		s.opt.statePath = p
	}
	path := s.opt.statePath
	s.baseFile = newRecordFile(path, stateMagic, stateVersion, stateIdentity(bucketName, s.opt.prefix, s.opt.dir), baseCodec(s.opt.prefix))
	held, err := s.baseFile.load()
	if err != nil {
		return fmt.Errorf("sync state: %w", describeRecordErr(path, err))
	}
	if other := s.baseFile.otherIdentity; other != "" {
		if p := strings.SplitN(other, "\x00", 3); len(p) == 3 {
			return fmt.Errorf("sync state %s belongs to %s and %s:%s", path, p[2], p[0], p[1])
		}
		return fmt.Errorf("sync state %s belongs to another sync", path)
	}
	if s.baseFile.lost {
		s.log.Warn("sync state damaged: the files after the damage are synced as on a first run, deleting nothing and keeping files that differ both ways", "file", path, "kept", len(held))
	}
	if !s.opt.dryRun {
		markUsed(path)
	}
	for id, e := range held {
		s.base[id] = &e
	}
	return nil
}

// saveBase writes what changed in the state since it was last written.
func (s *syncer) saveBase() error {
	if s.opt.mode != modeTwoWay || s.opt.dryRun || s.baseFile == nil {
		return nil
	}
	s.mu.Lock()
	if s.baseDirt {
		s.baseFile.update(func(yield func(string, baseEntry) bool) {
			for id, b := range s.base {
				if !yield(id, *b) {
					return
				}
			}
		}, func(id string) (baseEntry, bool) {
			b, ok := s.base[id]
			if !ok {
				return baseEntry{}, false
			}
			return *b, true
		})
		s.baseDirt = false
	}
	s.mu.Unlock()
	return s.baseFile.save()
}
