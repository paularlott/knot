package nfsserver

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/paularlott/knot/internal/log"
)

// diskCache keeps whole files on local disk, by the sha256 of their content,
// so reading a file again, or changing it, needs nothing from the server but
// its description. Content addressed, a kept file is never stale: a file
// that changes has another sha256. The least recently used files go once the
// cache is over its size.
type diskCache struct {
	dir     string
	max     int64 // bytes
	maxFile int64 // the largest file kept

	mu      sync.Mutex
	entries map[string]*cacheEntry
	total   int64
	filling map[string]chan struct{} // sha256s being fetched
}

type cacheEntry struct {
	size int64
	used time.Time
}

// openDiskCache opens, or makes, a cache in dir holding at most max bytes,
// finding what an earlier run left in it.
func openDiskCache(dir string, max int64) (*diskCache, error) {
	if err := os.MkdirAll(filepath.Join(dir, "files"), 0700); err != nil {
		return nil, err
	}
	c := &diskCache{dir: dir, max: max, maxFile: max / 4, entries: map[string]*cacheEntry{}, filling: map[string]chan struct{}{}}
	root := filepath.Join(dir, "files")
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if len(name) != 64 {
			os.Remove(p) // a partial fetch
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		c.entries[name] = &cacheEntry{size: info.Size(), used: info.ModTime()}
		c.total += info.Size()
		return nil
	})
	c.mu.Lock()
	c.evictLocked()
	c.mu.Unlock()
	return c, nil
}

func (c *diskCache) path(sha string) string {
	return filepath.Join(c.dir, "files", sha[:2], sha)
}

// stagingDir is where staged files are written, beside the cache so a saved
// one can be moved into it.
func (c *diskCache) stagingDir() string { return filepath.Join(c.dir, "staging") }

// open returns the kept file with this content, nil when there is none.
func (c *diskCache) open(sha string) *os.File {
	if c == nil || len(sha) != 64 {
		return nil
	}
	c.mu.Lock()
	e, ok := c.entries[sha]
	if ok {
		e.used = time.Now()
	}
	c.mu.Unlock()
	if !ok {
		return nil
	}
	f, err := os.Open(c.path(sha))
	if err != nil {
		c.mu.Lock()
		if c.entries[sha] == e {
			delete(c.entries, sha)
			c.total -= e.size
		}
		c.mu.Unlock()
		return nil
	}
	return f
}

// keeps reports whether a file of this size would be kept.
func (c *diskCache) keeps(size int64) bool { return c != nil && size <= c.maxFile }

// adopt moves a file whose content is sha into the cache, removing it
// otherwise; it reports whether the cache took it.
func (c *diskCache) adopt(src, sha string, size int64) bool {
	if !c.keeps(size) || len(sha) != 64 {
		os.Remove(src)
		return false
	}
	dst := c.path(sha)
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		os.Remove(src)
		return false
	}
	if err := os.Rename(src, dst); err != nil {
		os.Remove(src)
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if old, ok := c.entries[sha]; ok {
		c.total -= old.size
	}
	c.entries[sha] = &cacheEntry{size: size, used: time.Now()}
	c.total += size
	c.evictLocked()
	return true
}

// fill fetches a file into the cache with get, unless it is already there or
// being fetched, and checks what arrives is the content expected.
func (c *diskCache) fill(sha string, size int64, get func() (io.ReadCloser, error)) {
	if !c.keeps(size) || len(sha) != 64 {
		return
	}
	c.mu.Lock()
	_, have := c.entries[sha]
	_, busy := c.filling[sha]
	if have || busy {
		c.mu.Unlock()
		return
	}
	done := make(chan struct{})
	c.filling[sha] = done
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.filling, sha)
		c.mu.Unlock()
		close(done)
	}()

	r, err := get()
	if err != nil {
		return
	}
	defer r.Close()
	if err := os.MkdirAll(filepath.Join(c.dir, "files", sha[:2]), 0700); err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Join(c.dir, "files", sha[:2]), "fill-")
	if err != nil {
		return
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), r)
	tmp.Close()
	if err != nil || n != size || hex.EncodeToString(h.Sum(nil)) != sha {
		os.Remove(tmp.Name())
		if err == nil {
			log.WithGroup("nfs").Warn("cache: fetched content does not match its checksum", "sha256", sha)
		}
		return
	}
	c.adopt(tmp.Name(), sha, size)
}

// evictLocked removes the least recently used files until the cache is
// within its size; c.mu is held.
func (c *diskCache) evictLocked() {
	if c.total <= c.max {
		return
	}
	shas := make([]string, 0, len(c.entries))
	for sha := range c.entries {
		shas = append(shas, sha)
	}
	sort.Slice(shas, func(i, j int) bool { return c.entries[shas[i]].used.Before(c.entries[shas[j]].used) })
	for _, sha := range shas {
		if c.total <= c.max {
			break
		}
		e := c.entries[sha]
		os.Remove(c.path(sha))
		delete(c.entries, sha)
		c.total -= e.size
	}
}
