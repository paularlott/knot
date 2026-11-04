package command_files

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/fsnotify/fsnotify"
)

// checksumsRead is how many files a sync read for their checksum, from its
// debug log.
func checksumsRead(out string) int {
	n := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, `"msg":"files read for their checksum"`) {
			var c int
			if i := strings.Index(line, `"count":`); i >= 0 {
				fmt.Sscanf(line[i+len(`"count":`):], "%d", &c)
			}
			n += c
		}
	}
	return n
}

// A one-way sync reads a file for its checksum once: later runs take it from
// the cache while the file's size and time are unchanged.
func TestChecksumsKeptBetweenRuns(t *testing.T) {
	client, _ := newFilesServer(t)
	dir := t.TempDir()
	for i := 0; i < 20; i++ {
		writeFile(t, filepath.Join(dir, fmt.Sprintf("d%d", i%4), fmt.Sprintf("f%d.txt", i)), fmt.Sprintf("content %d", i))
	}
	run := func() int {
		t.Helper()
		var out logBuf
		if err := runEngine(context.Background(), client, syncOptions{mode: modeUp, dir: dir, bucket: "sync-test", prefix: "hc/", log: out.logger()}, nil); err != nil {
			t.Fatalf("sync: %v\n%s", err, out.String())
		}
		return checksumsRead(out.String())
	}
	if n := run(); n != 20 {
		t.Fatalf("first run read %d files, want 20", n)
	}
	if n := run(); n != 0 {
		t.Errorf("second run read %d files, want none: the cache was not used", n)
	}
	writeFile(t, filepath.Join(dir, "d1", "f1.txt"), "changed content")
	if n := run(); n != 1 {
		t.Errorf("after one change, read %d files, want 1", n)
	}
	path, _ := hashCachePath(mustReal(t, dir))
	if _, err := os.Stat(path); err != nil {
		t.Errorf("no cache file: %v", err)
	}
}

func mustReal(t *testing.T, dir string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

// A folder made or removed while watching is looked at on its own, without
// walking the whole directory.
func TestWatchNewFolderOnly(t *testing.T) {
	client, _ := newFilesServer(t)
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "first.txt"), "1")
	var out logBuf
	startWatch(t, client, src, "sync-test:nf", syncRequest{del: true, log: out.logger()})
	has := func(key string) func() bool {
		return func() bool { _, ok := remoteContent(t, client, "sync-test", key); return ok }
	}
	waitUntil(t, "initial pass", has("nf/first.txt"))

	// A folder that arrives already filled, as a move or an unpack makes.
	staging := t.TempDir()
	for i := 0; i < 5; i++ {
		writeFile(t, filepath.Join(staging, "pkg", "deep", fmt.Sprintf("f%d.txt", i)), "x")
	}
	if err := os.Rename(filepath.Join(staging, "pkg"), filepath.Join(src, "pkg")); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "files of a folder moved in", has("nf/pkg/deep/f4.txt"))

	os.RemoveAll(filepath.Join(src, "pkg"))
	waitUntil(t, "a folder removed", func() bool { return !has("nf/pkg/deep/f0.txt")() && !has("nf/pkg/deep/f4.txt")() })

	if logged(out.String(), "looking at every local file") {
		t.Errorf("a new or removed folder walked the whole directory:\n%s", out.String())
	}
}

// Folders that cannot be watched are reported once, with the reason, not a
// warning for each.
func TestWatchFailuresReportedOnce(t *testing.T) {
	client, _ := newFilesServer(t)
	dir := t.TempDir()
	for _, d := range []string{"a", "b/c", "d"} {
		os.MkdirAll(filepath.Join(dir, d), 0755)
	}
	var out logBuf
	s := newSyncer(client, syncOptions{dir: dir, log: out.logger()})
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	fsw.Close() // every Add fails
	s.watchTree(fsw, dir)
	s.watchTree(fsw, dir)
	text := out.String()
	if strings.Count(text, "cannot watch every folder") != 1 || !logged(text, "cannot watch every folder", `"unwatched":5`, `"of":5`) {
		t.Errorf("watch failures not reported once:\n%s", text)
	}
}

func TestWatchLimitHint(t *testing.T) {
	if h := watchLimitHint(syscall.EMFILE, 10); !strings.Contains(h, "ulimit -n") {
		t.Errorf("open file limit hint: %q", h)
	}
}
