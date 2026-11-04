package command_files

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shamaton/msgpack/v3"
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
	hashCacheVersion = 1
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
		case strings.HasPrefix(name, "hashes-") && strings.HasSuffix(name, ".msgpack"),
			strings.HasSuffix(name, ".json"):
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

type hashCacheFile struct {
	Version int                       `msgpack:"version"`
	Dir     string                    `msgpack:"dir"`
	Files   map[string]hashCacheEntry `msgpack:"files"` // by local rel
}

type hashCacheEntry struct {
	Size  int64  `msgpack:"size"`
	Mtime int64  `msgpack:"mtime"` // unix nanoseconds
	SHA   []byte `msgpack:"sha"`   // 32 bytes
	// The inode number and change time, where the platform has them (see
	// fileStamp); a cache written without them has each file read again
	// once.
	Ino   uint64 `msgpack:"ino,omitempty"`
	Ctime int64  `msgpack:"ctime,omitempty"`
}

// hashCachePath is where the checksums of a directory's files are kept.
func hashCachePath(dir string) (string, error) {
	sd, err := syncDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(dir))
	return filepath.Join(sd, "hashes-"+hex.EncodeToString(sum[:12])+".msgpack"), nil
}

// loadHashes reads the checksums kept by an earlier run. A missing or
// unreadable cache is no cache.
func (s *syncer) loadHashes() {
	if s.opt.hashCachePath == "" {
		p, err := hashCachePath(s.opt.dir)
		if err != nil {
			return
		}
		s.opt.hashCachePath = p
	}
	data, err := os.ReadFile(s.opt.hashCachePath)
	if err != nil {
		return
	}
	if !s.opt.dryRun {
		markUsed(s.opt.hashCachePath)
	}
	var f hashCacheFile
	if msgpack.Unmarshal(data, &f) != nil || f.Version != hashCacheVersion || f.Dir != s.opt.dir {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for rel, e := range f.Files {
		if len(e.SHA) == sha256.Size {
			s.hashes[rel] = hashed{size: e.Size, mtime: time.Unix(0, e.Mtime), stamp: fileStamp{ino: e.Ino, ctime: e.Ctime}, sha: hex.EncodeToString(e.SHA)}
		}
	}
}

// saveHashes writes the checksums when they changed: at once with force, else
// no more often than hashSaveEvery. Only files still there are kept, so the
// cache does not grow with files long gone.
func (s *syncer) saveHashes(force bool) {
	if s.opt.dryRun || s.opt.hashCachePath == "" {
		return
	}
	s.mu.Lock()
	if !s.hashDirt || (!force && time.Since(s.hashSaved) < hashSaveEvery) {
		s.mu.Unlock()
		return
	}
	f := hashCacheFile{Version: hashCacheVersion, Dir: s.opt.dir, Files: make(map[string]hashCacheEntry, len(s.hashes))}
	for rel, h := range s.hashes {
		l := s.local[s.identity(rel)]
		if l == nil || l.rel != rel || l.size != h.size || !l.mtime.Equal(h.mtime) || !stampMatches(h.stamp, l.stamp) {
			continue
		}
		raw, err := hex.DecodeString(h.sha)
		if err != nil {
			continue
		}
		f.Files[rel] = hashCacheEntry{Size: h.size, Mtime: h.mtime.UnixNano(), SHA: raw, Ino: h.stamp.ino, Ctime: h.stamp.ctime}
	}
	s.hashDirt = false
	s.hashSaved = time.Now()
	s.mu.Unlock()

	data, err := msgpack.Marshal(&f)
	if err != nil {
		return
	}
	writeAtomic(s.opt.hashCachePath, data)
}
