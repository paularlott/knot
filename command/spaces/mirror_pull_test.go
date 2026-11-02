package command_spaces

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
)

// remoteTree serves files from the stub as if they lived under /srv in the
// space, listing them for Find and serving their content for reads.
func remoteTree(stub *stubClient, mtime time.Time, files map[string]string) {
	stub.files = map[string]remoteFile{}
	dirs := map[string]bool{}
	for rel, content := range files {
		p := "/srv/" + rel
		stub.files[p] = remoteFile{content: content, mtimeNs: mtime.UnixNano(), perm: 0640}
		stub.remoteEntries = append(stub.remoteEntries, apiclient.FindEntry{
			Path: p, Size: int64(len(content)), Mtime: float64(mtime.UnixNano()) / 1e9,
		})
		for d := filepath.Dir(rel); d != "."; d = filepath.Dir(d) {
			dirs[d] = true
		}
	}
	for d := range dirs {
		stub.remoteEntries = append(stub.remoteEntries, apiclient.FindEntry{Path: "/srv/" + d, IsDir: true})
	}
}

func pullOptions(stub *stubClient, root string) *mirrorOptions {
	return &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "myspace",
		localRoot: root, remoteDir: "/srv", parallel: 2, pull: true,
	}
}

func readLocal(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

func TestIsSpacePath(t *testing.T) {
	for arg, want := range map[string]bool{
		"myspace:/srv":  true,
		"my-space:rel":  true,
		"C:\\Users\\me": false,
		"c:/data":       false,
		"./site":        false,
		"./a:b":         false,
		"/tmp/x:y":      false,
		"..\\dir:name":  false,
		"plainfolder":   false,
		"ab:":           true,
	} {
		if got := isSpacePath(arg); got != want {
			t.Errorf("isSpacePath(%q) = %v, want %v", arg, got, want)
		}
	}
}

func TestPullDownloadsTree(t *testing.T) {
	root := filepath.Join(t.TempDir(), "new") // created by the pull
	mtime := time.Now().Add(-time.Hour).Truncate(time.Second)
	stub := newStubClient()
	remoteTree(stub, mtime, map[string]string{
		"index.html":     "<h1>hi</h1>",
		"css/site.css":   "body{}",
		"img/deep/a.bin": "\x00\x01\xff binary",
	})

	stats, err := pullOptions(stub, root).runPull(context.Background())
	if err != nil {
		t.Fatalf("runPull: %v", err)
	}
	if got := readLocal(t, root, "img/deep/a.bin"); got != "\x00\x01\xff binary" {
		t.Errorf("a.bin = %q", got)
	}
	if got := readLocal(t, root, "css/site.css"); got != "body{}" {
		t.Errorf("site.css = %q", got)
	}
	info, err := os.Stat(filepath.Join(root, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(mtime) || info.Mode().Perm() != 0640 {
		t.Errorf("index.html mtime %v perm %v, want %v 0640", info.ModTime(), info.Mode().Perm(), mtime)
	}
	if got := stats.uploaded.Load(); got != 3 {
		t.Errorf("downloaded %d, want 3", got)
	}

	// A second run finds nothing to fetch.
	stub.reads = 0
	stats, err = pullOptions(stub, root).runPull(context.Background())
	if err != nil {
		t.Fatalf("second runPull: %v", err)
	}
	if stats.uploaded.Load() != 0 || stats.skipped.Load() != 3 || stub.reads != 0 {
		t.Errorf("second run: downloaded %d skipped %d reads %d, want 0 3 0", stats.uploaded.Load(), stats.skipped.Load(), stub.reads)
	}
}

func TestPullChunked(t *testing.T) {
	prev := chunkSize
	chunkSize = 4
	defer func() { chunkSize = prev }()

	root := t.TempDir()
	stub := newStubClient()
	content := strings.Repeat("0123456789", 3) + "x"
	remoteTree(stub, time.Now(), map[string]string{"big.txt": content})

	if _, err := pullOptions(stub, root).runPull(context.Background()); err != nil {
		t.Fatalf("runPull: %v", err)
	}
	if got := readLocal(t, root, "big.txt"); got != content {
		t.Errorf("big.txt = %q, want %q", got, content)
	}
	if stub.reads != 8 {
		t.Errorf("reads = %d, want 8 pieces of 4 bytes", stub.reads)
	}
}

func TestPullReplacesChangedAndDeletesExtras(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"keep.txt":          "old",
		"stale.txt":         "gone",
		"olddir/a.txt":      "gone",
		"olddir/sub/b.txt":  "gone",
		"logs/app.log":      "excluded",
		".knot-mirror-1234": "left by an interrupted run",
	})
	stub := newStubClient()
	remoteTree(stub, time.Now(), map[string]string{"keep.txt": "new!"})

	opts := pullOptions(stub, root)
	opts.excludes = []string{"logs/"}
	opts.verbose = true
	stats, err := opts.runPull(context.Background())
	if err != nil {
		t.Fatalf("runPull: %v", err)
	}
	if got := readLocal(t, root, "keep.txt"); got != "new!" {
		t.Errorf("keep.txt = %q", got)
	}
	for _, gone := range []string{"stale.txt", "olddir", ".knot-mirror-1234"} {
		if _, err := os.Lstat(filepath.Join(root, gone)); !os.IsNotExist(err) {
			t.Errorf("%s should be gone", gone)
		}
	}
	if got := readLocal(t, root, "logs/app.log"); got != "excluded" {
		t.Errorf("excluded logs/app.log touched: %q", got)
	}
	// stale.txt and olddir (once, with its contents); the leftover temp
	// file is cleared but isn't part of the mirror.
	if got := stats.deleted.Load(); got != 2 {
		t.Errorf("deleted %d, want 2", got)
	}
}

func TestPullDirectoryReplacesLocalFile(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"conf": "was a file"})
	stub := newStubClient()
	remoteTree(stub, time.Now(), map[string]string{"conf/app.yaml": "a: 1"})

	if _, err := pullOptions(stub, root).runPull(context.Background()); err != nil {
		t.Fatalf("runPull: %v", err)
	}
	if got := readLocal(t, root, "conf/app.yaml"); got != "a: 1" {
		t.Errorf("conf/app.yaml = %q", got)
	}
}

func TestPullSymlinks(t *testing.T) {
	root := t.TempDir()
	stub := newStubClient()
	remoteTree(stub, time.Now(), map[string]string{"real.txt": "r"})
	stub.remoteEntries = append(stub.remoteEntries, apiclient.FindEntry{Path: "/srv/link.txt", LinkTarget: "real.txt"})

	if _, err := pullOptions(stub, root).runPull(context.Background()); err != nil {
		t.Fatalf("runPull: %v", err)
	}
	if target, err := os.Readlink(filepath.Join(root, "link.txt")); err != nil || target != "real.txt" {
		t.Errorf("link.txt → %q, %v", target, err)
	}
}

func TestPullDryRunChangesNothing(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"stale.txt": "s"})
	stub := newStubClient()
	remoteTree(stub, time.Now(), map[string]string{"new.txt": "n"})

	opts := pullOptions(stub, root)
	opts.dryRun = true
	stats, err := opts.runPull(context.Background())
	if err != nil {
		t.Fatalf("runPull: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "new.txt")); !os.IsNotExist(err) {
		t.Error("dry run downloaded new.txt")
	}
	if readLocal(t, root, "stale.txt") != "s" || stub.reads != 0 {
		t.Errorf("dry run touched files: reads %d", stub.reads)
	}
	if stats.uploaded.Load() != 1 || stats.deleted.Load() != 1 {
		t.Errorf("dry run counts: downloaded %d deleted %d, want 1 1", stats.uploaded.Load(), stats.deleted.Load())
	}
}

func TestPullRefusesEmptyRemote(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"precious.txt": "p"})
	stub := newStubClient()

	_, err := pullOptions(stub, root).runPull(context.Background())
	if err == nil || !strings.Contains(err.Error(), "empty or missing") {
		t.Fatalf("err = %v, want a refusal", err)
	}
	if readLocal(t, root, "precious.txt") != "p" {
		t.Error("precious.txt was touched")
	}
}

func TestPushRefusesEmptyLocal(t *testing.T) {
	stub := newStubClient()
	stub.remoteEntries = []apiclient.FindEntry{{Path: "/var/www/index.php", Size: 1}}
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "myspace",
		localRoot: t.TempDir(), remoteDir: "/var/www", parallel: 1,
	}
	_, err := opts.run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "is empty") {
		t.Fatalf("err = %v, want a refusal", err)
	}
	if stub.hasDeleted("/var/www/index.php") {
		t.Error("index.php was deleted")
	}
}
