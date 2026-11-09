package command_files

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// --hash-file puts the checksum cache where it is asked to; one inside the
// directory synced is not synced itself, nor are its temporary files.
func TestHashFileInsideSyncedDir(t *testing.T) {
	client, _ := newFilesServer(t)
	ctx := context.Background()
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.txt"), "alpha")
	hashFile := filepath.Join(src, ".hashes")
	if err := runSyncRequest(ctx, client, src, "sync-test:hf", syncRequest{hashPath: hashFile}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hashFile); err != nil {
		t.Fatalf("no cache at --hash-file: %v", err)
	}
	// Again, with the cache there from the start.
	writeFile(t, filepath.Join(src, "b.txt"), "beta")
	if err := runSyncRequest(ctx, client, src, "sync-test:hf", syncRequest{hashPath: hashFile}); err != nil {
		t.Fatal(err)
	}
	for _, k := range remoteKeys(t, client) {
		if strings.Contains(k, ".hashes") {
			t.Errorf("the cache was synced: %s", k)
		}
	}
	if keys := strings.Join(remoteKeys(t, client), " "); !strings.Contains(keys, "hf/a.txt") || !strings.Contains(keys, "hf/b.txt") {
		t.Errorf("the files were not synced: %s", keys)
	}
}

// --hash-file=~/x is under the home directory, as the shell would make it.
func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	if got := expandHome("~/.notes-hashes"); got != filepath.Join(home, ".notes-hashes") {
		t.Errorf("expandHome = %q", got)
	}
	if got := expandHome("/abs/x"); got != "/abs/x" {
		t.Errorf("expandHome changed an absolute path: %q", got)
	}
}

// Caches and states unused for long, and temporary files left behind, are
// removed from the sync folder; others, and anything else there, are kept.
func TestSweepSyncDir(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	files := map[string]time.Duration{
		"hashes-old.bin":       staleAfter + time.Hour,
		"hashes-new.bin":       time.Hour,
		"state-old.bin":        staleAfter + time.Hour,
		"state-new.bin":        time.Hour,
		"hashes-x.bin.tmp-123": tempStaleAfter + time.Hour,
		"hashes-y.bin.tmp-456": time.Minute,
		"something-else.txt":   staleAfter + time.Hour,
	}
	for name, age := range files {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		at := now.Add(-age)
		os.Chtimes(p, at, at)
	}
	sweepSyncDir(dir, now)
	for name, want := range map[string]bool{
		"hashes-old.bin": false, "hashes-new.bin": true,
		"state-old.bin": false, "state-new.bin": true,
		"hashes-x.bin.tmp-123": false, "hashes-y.bin.tmp-456": true,
		"something-else.txt": true,
	} {
		_, err := os.Stat(filepath.Join(dir, name))
		if got := err == nil; got != want {
			t.Errorf("%s kept = %v, want %v", name, got, want)
		}
	}
}

// A sync marks the cache it uses, so one in use is never swept, however
// long nothing changed.
func TestSyncMarksCacheUsed(t *testing.T) {
	client, _ := newFilesServer(t)
	ctx := context.Background()
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.txt"), "alpha")
	hashFile := filepath.Join(t.TempDir(), "hashes")
	if err := runSyncRequest(ctx, client, src, "sync-test:mu", syncRequest{hashPath: hashFile}); err != nil {
		t.Fatal(err)
	}
	long := time.Now().Add(-staleAfter - time.Hour)
	os.Chtimes(hashFile, long, long)
	if err := runSyncRequest(ctx, client, src, "sync-test:mu", syncRequest{hashPath: hashFile}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(hashFile)
	if err != nil || time.Since(info.ModTime()) > time.Hour {
		t.Errorf("a cache used was not marked used: %v %v", info.ModTime(), err)
	}
}

// Two syncs writing the same cache at once each replace it whole, through
// temporary files of their own, and leave none behind.
func TestWriteAtomicConcurrent(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cache")
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := writeAtomic(p, []byte(strings.Repeat(string(rune('a'+i)), 4096))); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	data, err := os.ReadFile(p)
	if err != nil || len(data) != 4096 || strings.Count(string(data), string(data[0])) != 4096 {
		t.Errorf("the cache is not one write whole: %d bytes, %v", len(data), err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("temporary files left: %d entries", len(entries))
	}
}
