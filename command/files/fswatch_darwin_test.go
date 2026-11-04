//go:build darwin

package command_files

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// On macOS a sync watches with FSEvents, which reports changes anywhere in
// the tree from one stream.
func TestFSEventsWatcher(t *testing.T) {
	client, _ := newFilesServer(t)
	dir := mustReal(t, t.TempDir())
	os.MkdirAll(filepath.Join(dir, "deep", "er"), 0755)
	var out logBuf
	s := newSyncer(client, syncOptions{dir: dir, log: out.logger()})
	w, err := newDirWatcher(s)
	if err != nil {
		t.Fatal(err)
	}
	defer w.close()
	if _, ok := w.(*fseventsWatcher); !ok {
		t.Fatalf("watching with %T, not FSEvents:\n%s", w, out.String())
	}
	w.add(dir)

	expect := func(what, path string) {
		t.Helper()
		deadline := time.After(5 * time.Second)
		for {
			select {
			case ev := <-w.events():
				if ev.path == path || ev.all {
					return
				}
			case <-deadline:
				t.Fatalf("no event for %s (%s)", what, path)
			}
		}
	}
	time.Sleep(200 * time.Millisecond) // the stream reports from when it started

	f := filepath.Join(dir, "deep", "er", "file.txt")
	os.WriteFile(f, []byte("x"), 0644)
	expect("a file written deep in the tree", f)

	nd := filepath.Join(dir, "newdir")
	os.MkdirAll(filepath.Join(nd, "sub"), 0755)
	os.WriteFile(filepath.Join(nd, "sub", "inner.txt"), []byte("y"), 0644)
	expect("a file in a new folder, with no folder added", filepath.Join(nd, "sub", "inner.txt"))

	moved := filepath.Join(dir, "moved.txt")
	os.Rename(f, moved)
	expect("a rename's new name", moved)

	os.Remove(moved)
	expect("a removal", moved)

	if w.takeLost() {
		t.Error("events reported lost")
	}
}
