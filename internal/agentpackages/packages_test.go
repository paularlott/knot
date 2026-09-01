package agentpackages

import (
	"os"
	"path/filepath"
	"testing"
)

// TestInitClearsStaleCache verifies that Init wipes anything a previous agent
// run left in the cache directory, so every agent start serves packages
// fetched fresh from the server.
func TestInitClearsStaleCache(t *testing.T) {
	dir := t.TempDir()

	stale := filepath.Join(dir, "knot.zip")
	if err := os.WriteFile(stale, []byte("old-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	staleDir := filepath.Join(dir, "junk")
	if err := os.MkdirAll(filepath.Join(staleDir, "inner"), 0o755); err != nil {
		t.Fatal(err)
	}

	Init(dir)

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale knot.zip survived Init: %v", err)
	}
	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Errorf("stale directory survived Init: %v", err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Errorf("cache dir not empty after Init: %v %d entries", err, len(entries))
	}

	// And a normal Get after the purge persists into the now-clean dir.
	data, err := Get("libs.zip", func() ([]byte, error) { return []byte("new-bytes"), nil })
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new-bytes" {
		t.Fatalf("unexpected data: %s", data)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "libs.zip")); err != nil || string(b) != "new-bytes" {
		t.Errorf("fetched package not persisted: %v %q", err, string(b))
	}

	// Re-Init clears it again.
	Init(dir)
	if _, err := os.Stat(filepath.Join(dir, "libs.zip")); !os.IsNotExist(err) {
		t.Errorf("persisted package survived re-Init: %v", err)
	}
}
