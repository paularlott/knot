package dirwatch

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Folders that cannot be watched are reported together, once per Add, not
// one by one; excluded folders are not watched at all.
func TestNotifyUnwatchedReported(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"a", "b/c", "d", "skip/me"} {
		os.MkdirAll(filepath.Join(dir, d), 0755)
	}
	calls, gotFailed, gotTotal := 0, 0, 0
	w, err := NewNotify(Options{
		Root:     dir,
		Excluded: func(rel string, isDir bool) bool { return rel == "skip" },
		Unwatched: func(failed, total int, firstErr error) {
			calls++
			gotFailed, gotTotal = failed, total
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	w.Close() // every Add fails
	w.Add(dir)
	if calls != 1 || gotFailed != 5 || gotTotal != 5 {
		t.Errorf("Unwatched called %d times with %d of %d, want once with 5 of 5", calls, gotFailed, gotTotal)
	}
}

// A file written in a folder added later is reported.
func TestNotifyEvents(t *testing.T) {
	dir := t.TempDir()
	w, err := NewNotify(Options{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	w.Add(dir)
	sub := filepath.Join(dir, "sub")
	os.Mkdir(sub, 0755)
	w.Add(sub)
	f := filepath.Join(sub, "f.txt")
	os.WriteFile(f, []byte("x"), 0644)
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-w.Events():
			if ev.Path == f {
				return
			}
		case <-deadline:
			t.Fatal("no event for a file in an added folder")
		}
	}
}

func TestLimitHintOpenFiles(t *testing.T) {
	if h := LimitHint(syscall.EMFILE, 10); !strings.Contains(h, "ulimit -n") {
		t.Errorf("open file limit hint: %q", h)
	}
}
