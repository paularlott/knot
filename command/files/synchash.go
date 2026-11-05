package command_files

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The checksums of local files are kept between runs, by size, modification
// time and stamp, so a sync of a large directory does not read every file
// again to find what changed. The cache is per directory, shared by every
// sync of it, and only ever a shortcut: a file whose size, time or stamp
// changed is read again, and a cache that is gone costs one reading of
// every file.
//
// It is kept in knot's sync folder in the user's configuration directory
// (syncDir), or where --hash-file says. In the sync folder, caches and
// two-way states not used for staleAfter are removed (sweepSyncDir): a
// directory no longer synced does not leave its checksums behind for good.

const (
	// hashSaveEvery is how often a watching sync writes the cache.
	hashSaveEvery = 30 * time.Second
	// staleAfter is how long a cache or two-way state in the sync folder
	// is kept unused. Every sync that uses one marks it used.
	staleAfter = 90 * 24 * time.Hour
	// tempStaleAfter is how long a temporary file left by a sync that was
	// stopped while writing is kept.
	tempStaleAfter = 24 * time.Hour
)

// syncDir is knot's sync folder: where caches and two-way states are kept
// unless the sync is told otherwise.
func syncDir() (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, "knot", "sync"), nil
}

// expandHome expands a leading ~ to the user's home directory, for a path
// given as --flag=~/x, which the shell leaves alone.
func expandHome(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") && !strings.HasPrefix(p, "~"+string(filepath.Separator)) {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, p[1:])
}

// markUsed records that a sync used a cache or state, so it is not swept as
// stale.
func markUsed(path string) {
	now := time.Now()
	os.Chtimes(path, now, now)
}

// sweepSyncDir removes what has gone stale in the sync folder: caches and
// two-way states unused for staleAfter, and temporary files older than
// tempStaleAfter. Files kept elsewhere, by --hash-file or --state-file, are
// the user's to manage. A two-way state removed only makes the next sync of
// its folder a first one, which deletes nothing.
func sweepSyncDir(dir string, now time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		var age time.Duration
		switch {
		case strings.Contains(name, ".tmp"):
			age = tempStaleAfter
		case strings.HasPrefix(name, "upload-") && strings.HasSuffix(name, ".json"):
			age = uploadStaleAfter // the server has let the parts go
		case (strings.HasPrefix(name, "hashes-") || strings.HasPrefix(name, "state-")) && strings.HasSuffix(name, ".bin"):
			age = staleAfter
		default:
			continue
		}
		info, err := e.Info()
		if err != nil || now.Sub(info.ModTime()) < age {
			continue
		}
		os.Remove(filepath.Join(dir, name))
	}
}

// writeAtomic replaces path with data at once, through a temporary file of
// its own, so neither a crash nor another sync writing the same file at the
// same moment leaves part of one.
func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// hashCacheMagic and hashCacheVersion mark a checksum cache (see
// recordFile).
const (
	hashCacheMagic   = "KNSH"
	hashCacheVersion = 1
)

type hashCacheEntry struct {
	Size  int64
	Mtime int64 // unix nanoseconds
	SHA   [sha256.Size]byte
	// The inode number and change time, where the platform has them (see
	// fileStamp).
	Ino   uint64
	Ctime int64
}

var hashCodec = recordCodec[hashCacheEntry]{
	put: func(b []byte, _ string, e hashCacheEntry) []byte {
		b = binary.AppendVarint(b, e.Size)
		b = binary.AppendVarint(b, e.Mtime)
		b = append(b, e.SHA[:]...)
		b = binary.AppendUvarint(b, e.Ino)
		return binary.AppendVarint(b, e.Ctime)
	},
	get: func(r *recReader, _ string) hashCacheEntry {
		var e hashCacheEntry
		e.Size = r.varint()
		e.Mtime = r.varint()
		copy(e.SHA[:], r.take(sha256.Size))
		e.Ino = r.uvarint()
		e.Ctime = r.varint()
		return e
	},
}

// hashCachePath is where the checksums of a directory's files are kept.
func hashCachePath(dir string) (string, error) {
	sd, err := syncDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(dir))
	return filepath.Join(sd, "hashes-"+hex.EncodeToString(sum[:12])+".bin"), nil
}

// loadHashes reads the checksums kept by an earlier run. A missing,
// damaged or unreadable cache is no cache, or as much of one as was read; a
// cache of another directory is replaced. A file there that is not a cache
// at all is left alone, and the sync refused.
func (s *syncer) loadHashes() error {
	if s.opt.hashCachePath == "" {
		p, err := hashCachePath(s.opt.dir)
		if err != nil {
			return nil
		}
		s.opt.hashCachePath = p
	}
	s.hashFile = newRecordFile(s.opt.hashCachePath, hashCacheMagic, hashCacheVersion, s.opt.dir, hashCodec)
	held, err := s.hashFile.load()
	switch {
	case errors.Is(err, errRecordForeign):
		return fmt.Errorf("checksum cache: %w", describeRecordErr(s.opt.hashCachePath, err))
	case err != nil && !errors.Is(err, errRecordVersion):
		s.log.Debug("checksum cache not read", "file", s.opt.hashCachePath, "error", err)
	}
	if s.hashFile.lost {
		s.log.Debug("checksum cache damaged: the files after the damage are read again", "file", s.opt.hashCachePath, "kept", len(held))
	}
	if !s.opt.dryRun {
		markUsed(s.opt.hashCachePath)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for rel, e := range held {
		s.hashes[rel] = hashed{size: e.Size, mtime: time.Unix(0, e.Mtime), stamp: fileStamp{ino: e.Ino, ctime: e.Ctime}, sha: hex.EncodeToString(e.SHA[:])}
	}
	return nil
}

// cacheEntry is what the cache keeps of rel: its checksum while the file is
// as it was when read.
func (s *syncer) cacheEntry(rel string) (hashCacheEntry, bool) {
	h, ok := s.hashes[rel]
	if !ok {
		return hashCacheEntry{}, false
	}
	l := s.local[s.identity(rel)]
	if l == nil || l.rel != rel || l.size != h.size || !l.mtime.Equal(h.mtime) || !stampMatches(h.stamp, l.stamp) {
		return hashCacheEntry{}, false
	}
	e := hashCacheEntry{Size: h.size, Mtime: h.mtime.UnixNano(), Ino: h.stamp.ino, Ctime: h.stamp.ctime}
	if n, err := hex.Decode(e.SHA[:], []byte(h.sha)); err != nil || n != sha256.Size {
		return hashCacheEntry{}, false
	}
	return e, true
}

// saveHashes writes what changed in the checksums: at once with force, else
// no more often than hashSaveEvery. Only files still there, as they were
// when read, are kept, so the cache does not grow with files long gone.
func (s *syncer) saveHashes(force bool) {
	if s.opt.dryRun || s.hashFile == nil {
		return
	}
	s.mu.Lock()
	if !s.hashDirt || (!force && time.Since(s.hashSaved) < hashSaveEvery) {
		s.mu.Unlock()
		return
	}
	s.hashFile.update(func(yield func(string, hashCacheEntry) bool) {
		for rel := range s.hashes {
			if e, ok := s.cacheEntry(rel); ok && !yield(rel, e) {
				return
			}
		}
	}, s.cacheEntry)
	s.hashDirt = false
	s.hashSaved = time.Now()
	s.mu.Unlock()

	if err := s.hashFile.save(); err != nil {
		s.log.Debug("checksum cache not written", "file", s.opt.hashCachePath, "error", err)
	}
}
