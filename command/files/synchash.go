package command_files

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"time"

	"github.com/shamaton/msgpack/v3"
)

// The checksums of local files are kept between runs, by size and
// modification time, so a sync of a large directory does not read every file
// again to find what changed. The cache is per directory, shared by every
// sync of it, and only ever a shortcut: a file whose size or time changed is
// read again.

const (
	hashCacheVersion = 1
	// hashSaveEvery is how often a watching sync writes the cache.
	hashSaveEvery = 30 * time.Second
)

type hashCacheFile struct {
	Version int                       `msgpack:"version"`
	Dir     string                    `msgpack:"dir"`
	Files   map[string]hashCacheEntry `msgpack:"files"` // by local rel
}

type hashCacheEntry struct {
	Size  int64  `msgpack:"size"`
	Mtime int64  `msgpack:"mtime"` // unix nanoseconds
	SHA   []byte `msgpack:"sha"`   // 32 bytes
}

// hashCachePath is where the checksums of a directory's files are kept.
func hashCachePath(dir string) (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(dir))
	return filepath.Join(cfg, "knot", "sync", "hashes-"+hex.EncodeToString(sum[:12])+".msgpack"), nil
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
	var f hashCacheFile
	if msgpack.Unmarshal(data, &f) != nil || f.Version != hashCacheVersion || f.Dir != s.opt.dir {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for rel, e := range f.Files {
		if len(e.SHA) == sha256.Size {
			s.hashes[rel] = hashed{size: e.Size, mtime: time.Unix(0, e.Mtime), sha: hex.EncodeToString(e.SHA)}
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
		if l == nil || l.rel != rel || l.size != h.size || !l.mtime.Equal(h.mtime) {
			continue
		}
		raw, err := hex.DecodeString(h.sha)
		if err != nil {
			continue
		}
		f.Files[rel] = hashCacheEntry{Size: h.size, Mtime: h.mtime.UnixNano(), SHA: raw}
	}
	s.hashDirt = false
	s.hashSaved = time.Now()
	s.mu.Unlock()

	data, err := msgpack.Marshal(&f)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.opt.hashCachePath), 0700); err != nil {
		return
	}
	tmp := s.opt.hashCachePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return
	}
	if err := os.Rename(tmp, s.opt.hashCachePath); err != nil {
		os.Remove(tmp)
	}
}
