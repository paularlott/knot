package command_files

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// A file changed by a tool that puts its modification time back, at the
// same size, is uploaded: its inode change time moved, so the cached
// checksum is not trusted. On a platform without change times only size and
// time can be compared, as rsync's quick check does.
func TestSyncNoticesEditWithTimeRestored(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("change times are read on macOS and Linux")
	}
	client, _ := newFilesServer(t)
	ctx := context.Background()
	src := t.TempDir()
	path := filepath.Join(src, "a.txt")
	old := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)

	writeFile(t, path, "alpha")
	os.Chtimes(path, old, old)
	if err := syncUp(ctx, client, src, "sync-test", "", false, false); err != nil {
		t.Fatal(err)
	}
	// As cp -p or tar x would: new content, same size, the time put back.
	time.Sleep(20 * time.Millisecond)
	writeFile(t, path, "omega")
	os.Chtimes(path, old, old)
	if err := syncUp(ctx, client, src, "sync-test", "", false, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := remoteContent(t, client, "sync-test", "a.txt"); got != "omega" {
		t.Errorf("after an edit with its time restored the bucket holds %q, want %q", got, "omega")
	}
}

func TestStampMatches(t *testing.T) {
	known := fileStamp{ino: 7, ctime: 100}
	for _, c := range []struct {
		recorded, now fileStamp
		want          bool
	}{
		{known, known, true},
		{known, fileStamp{ino: 7, ctime: 101}, false},
		{known, fileStamp{ino: 8, ctime: 100}, false},
		{fileStamp{}, known, false},      // recorded by an earlier knot: read once more
		{known, fileStamp{}, true},       // a platform without stamps
		{fileStamp{}, fileStamp{}, true}, // likewise
	} {
		if got := stampMatches(c.recorded, c.now); got != c.want {
			t.Errorf("stampMatches(%+v, %+v) = %v, want %v", c.recorded, c.now, got, c.want)
		}
	}
}
