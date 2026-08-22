// Package agentpackages caches scriptling package zips (knot.zip, libs.zip)
// for the in-space agent service API. The agent never builds zips — it fetches
// them from the knot server and serves the cached bytes to in-space scriptling
// processes. libs.zip is invalidated when the server notifies a library change;
// knot.zip is invalidated whenever the agent establishes a new server session
// (a server restart produces a fresh package).
package agentpackages

import (
	"os"
	"path/filepath"
	"sync"
)

var (
	mu        sync.Mutex
	cacheDir  string
	invalid   = make(map[string]bool)
	persisted = make(map[string][]byte)
)

// Init sets the cache directory (e.g. ~/.knot/cache) and drops any previously
// cached content held in memory.
func Init(dir string) {
	mu.Lock()
	defer mu.Unlock()
	cacheDir = dir
	invalid = make(map[string]bool)
	persisted = make(map[string][]byte)
	_ = os.MkdirAll(dir, 0o700)
}

// Invalidate drops the cached package with the given name (e.g. "libs.zip").
func Invalidate(name string) {
	mu.Lock()
	defer mu.Unlock()
	invalid[name] = true
	_ = os.Remove(path(name))
	delete(persisted, name)
}

// Get returns the cached package bytes, fetching them via the supplied
// function on a cache miss. The fetch is performed at most once per wait
// group; concurrent callers share the result.
func Get(name string, fetch func() ([]byte, error)) ([]byte, error) {
	if b, ok := load(name); ok {
		return b, nil
	}

	mu.Lock()
	if b, ok := persisted[name]; ok && !invalid[name] {
		mu.Unlock()
		return b, nil
	}
	mu.Unlock()

	b, err := fetch()
	if err != nil {
		return nil, err
	}

	mu.Lock()
	defer mu.Unlock()
	invalid[name] = false
	persisted[name] = b
	if cacheDir != "" {
		_ = os.WriteFile(path(name), b, 0o644)
	}
	return b, nil
}

func load(name string) ([]byte, bool) {
	mu.Lock()
	defer mu.Unlock()
	if invalid[name] {
		return nil, false
	}
	if b, ok := persisted[name]; ok {
		return b, true
	}
	if cacheDir == "" {
		return nil, false
	}
	b, err := os.ReadFile(path(name))
	if err != nil {
		return nil, false
	}
	persisted[name] = b
	return b, true
}

func path(name string) string {
	return filepath.Join(cacheDir, name)
}
