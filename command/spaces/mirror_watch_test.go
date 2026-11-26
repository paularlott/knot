package command_spaces

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/util/dirwatch"
)

// --- helpers ----------------------------------------------------------------

// newTestWatcher makes a watcher on opts.localRoot and records what is there
// as sent, as the initial mirror would leave it.
func newTestWatcher(t *testing.T, opts *mirrorOptions) *mirrorWatcher {
	t.Helper()
	w, err := newMirrorWatcher(opts)
	if err != nil {
		t.Fatalf("create watcher: %v", err)
	}
	t.Cleanup(w.close)
	learnLocal(w)
	return w
}

// learnLocal records what is here as given to the space, as after a full
// mirror that sent everything.
func learnLocal(w *mirrorWatcher) {
	clear(w.known)
	w.walk(w.root, func(rel, abs string, info os.FileInfo) {
		target := ""
		if info.Mode()&os.ModeSymlink != 0 {
			target, _ = os.Readlink(abs)
		}
		if e := entryOf(info, target); e.kind != kindNone {
			w.known[rel] = e
		}
	})
}

// startTestWatcher runs the watch loop on localRoot in a goroutine, as after
// the initial mirror. Returns a stop func. The caller makes filesystem
// changes and waits for the debounce + flush to pick them up.
func startTestWatcher(t *testing.T, opts *mirrorOptions) func() {
	t.Helper()
	w := newTestWatcher(t, opts)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.loop(context.Background(), ctx, func() {})
	}()
	// The stream reports from when it started (FSEvents).
	time.Sleep(100 * time.Millisecond)
	return func() {
		cancel()
		<-done
	}
}

// waitFor repeatedly calls check until it returns true or the deadline passes.
func waitFor(t *testing.T, what string, timeout time.Duration, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s: timed out after %v", what, timeout)
}

// --- syncPath unit tests (no fsnotify timing) -------------------------------

func TestWatchSyncPathUploadsFile(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0644)

	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "s",
		localRoot: root, remoteDir: "dst",
		parallel: 1,
	}
	w := newTestWatcher(t, opts)

	w.syncPath(context.Background(), "a.txt")

	if !stub.hasUploaded("dst/a.txt") {
		t.Error("expected a.txt to be uploaded")
	}
}

func TestWatchSyncPathDeletesMissingFile(t *testing.T) {
	root := t.TempDir()

	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "s",
		localRoot: root, remoteDir: "dst",
		parallel: 1,
	}
	w := newTestWatcher(t, opts)
	w.known["gone.txt"] = sentEntry{kind: kindFile}

	// Path sent before, gone locally → should be deleted from remote.
	w.syncPath(context.Background(), "gone.txt")

	if !stub.hasDeleted("dst/gone.txt") {
		t.Error("expected gone.txt to be deleted from remote")
	}
}

// A path that came and went without being sent, such as an editor's temp
// file, costs no delete.
func TestWatchSyncPathSkipsDeleteOfUnsent(t *testing.T) {
	root := t.TempDir()
	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "s",
		localRoot: root, remoteDir: "dst",
		parallel: 1,
	}
	w := newTestWatcher(t, opts)

	w.syncPath(context.Background(), ".file.txt.tmp123")

	if len(stub.ops) != 0 {
		t.Errorf("expected no calls for a path never sent, got %v", stub.ops)
	}
}

func TestWatchSyncPathUploadsDirContents(t *testing.T) {
	root := t.TempDir()
	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "s",
		localRoot: root, remoteDir: "dst",
		parallel: 1,
	}
	w := newTestWatcher(t, opts)

	dir := filepath.Join(root, "sub")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "x.txt"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "y.txt"), []byte("yy"), 0644)

	w.syncPath(context.Background(), "sub")

	if !stub.hasUploaded("dst/sub/x.txt") {
		t.Error("expected sub/x.txt to be uploaded")
	}
	if !stub.hasUploaded("dst/sub/y.txt") {
		t.Error("expected sub/y.txt to be uploaded")
	}
}

func TestWatchSyncPathSymlink(t *testing.T) {
	root := t.TempDir()
	os.Symlink("/target", filepath.Join(root, "link.txt"))

	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "s",
		localRoot: root, remoteDir: "dst",
		parallel: 1,
	}
	w := newTestWatcher(t, opts)

	w.syncPath(context.Background(), "link.txt")

	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.symlinks["dst/link.txt"] != "/target" {
		t.Errorf("expected symlink dst/link.txt -> /target, got %q", stub.symlinks["dst/link.txt"])
	}
}

func TestWatchSyncPathRespectsExcludes(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "secret.key"), []byte("nope"), 0644)
	os.WriteFile(filepath.Join(root, "app.js"), []byte("ok"), 0644)

	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "s",
		localRoot: root, remoteDir: "dst",
		excludes: []string{"*.key"},
		parallel: 1,
	}
	w := newTestWatcher(t, opts)

	w.syncPath(context.Background(), "secret.key")
	w.syncPath(context.Background(), "app.js")

	if stub.hasUploaded("dst/secret.key") {
		t.Error("excluded file should not be uploaded")
	}
	if !stub.hasUploaded("dst/app.js") {
		t.Error("non-excluded file should be uploaded")
	}
}

// A folder event compares the whole folder with what was sent: what changed
// is uploaded, what went is deleted, what is unchanged is left alone.
func TestWatchSyncPathReconcilesFolder(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"sub/same.txt": "same", "sub/old.txt": "old", "sub/deep/gone.txt": "g"})
	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "s",
		localRoot: root, remoteDir: "dst",
		parallel: 1,
	}
	w := newTestWatcher(t, opts)

	os.Remove(filepath.Join(root, "sub", "old.txt"))
	os.RemoveAll(filepath.Join(root, "sub", "deep"))
	os.WriteFile(filepath.Join(root, "sub", "new.txt"), []byte("new"), 0644)

	w.syncPath(context.Background(), "sub")

	if !stub.hasUploaded("dst/sub/new.txt") {
		t.Error("new file in the folder not uploaded")
	}
	if stub.hasUploaded("dst/sub/same.txt") {
		t.Error("unchanged file re-uploaded")
	}
	if !stub.hasDeleted("dst/sub/old.txt") || !stub.hasDeleted("dst/sub/deep") {
		t.Errorf("removed paths not deleted: %v", stub.ops)
	}
	if stub.hasDeleted("dst/sub/deep/gone.txt") {
		t.Error("file in a deleted folder deleted on its own as well")
	}
}

// A symlink replaced by a file is removed before the file is written, so the
// file is not written through the old link.
func TestWatchSyncPathKindChange(t *testing.T) {
	root := t.TempDir()
	os.Symlink("/etc/hosts", filepath.Join(root, "conf"))
	os.WriteFile(filepath.Join(root, "dir"), []byte("was a folder"), 0644)
	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "s",
		localRoot: root, remoteDir: "dst",
		parallel: 1,
	}
	w := newTestWatcher(t, opts)
	w.known["dir"] = sentEntry{kind: kindDir}
	w.known["dir/inner.txt"] = sentEntry{kind: kindFile}

	os.Remove(filepath.Join(root, "conf"))
	os.WriteFile(filepath.Join(root, "conf"), []byte("real file"), 0644)
	w.syncPath(context.Background(), "conf")
	w.syncPath(context.Background(), "dir")

	want := []string{"delete dst/conf", "write dst/conf", "delete dst/dir", "write dst/dir"}
	if strings.Join(stub.ops, ",") != strings.Join(want, ",") {
		t.Errorf("ops = %v, want %v", stub.ops, want)
	}
	if _, ok := w.known["dir/inner.txt"]; ok {
		t.Error("what was below a replaced folder is still known")
	}
}

// A change that cannot be sent stays to be tried again, and is sent when
// the space is back.
func TestWatchFlushRetriesFailures(t *testing.T) {
	root := t.TempDir()
	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "s",
		localRoot: root, remoteDir: "dst",
		parallel: 1,
	}
	w := newTestWatcher(t, opts)
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0644)
	stub.failWrites = 1

	w.pending["a.txt"] = true
	if w.flush(context.Background()) {
		t.Fatal("flush reported success with a failed upload")
	}
	if !w.failed["a.txt"] {
		t.Fatal("failed path not kept to retry")
	}
	if !w.flush(context.Background()) {
		t.Fatal("retry flush failed")
	}
	if !stub.hasUploaded("dst/a.txt") || len(w.failed) != 0 {
		t.Error("failed path not sent on retry")
	}
}

func TestWatchFlushDrainsPending(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0644)

	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "s",
		localRoot: root, remoteDir: "dst",
		parallel: 1,
	}
	w := newTestWatcher(t, opts)

	w.pending["a.txt"] = true
	w.flush(context.Background())

	if len(w.pending) != 0 {
		t.Errorf("expected pending drained, got %d", len(w.pending))
	}
	if !stub.hasUploaded("dst/a.txt") {
		t.Error("expected a.txt uploaded after flush")
	}
}

func TestWatchOnEvent(t *testing.T) {
	root := t.TempDir()
	w := newTestWatcher(t, &mirrorOptions{localRoot: root, remoteDir: "dst", excludes: []string{"*.log"}})

	w.onEvent(dirwatch.Event{Path: filepath.Join(w.root, "f.txt")})
	w.onEvent(dirwatch.Event{Path: filepath.Join(w.root, "f.txt")})
	w.onEvent(dirwatch.Event{Path: filepath.Join(w.root, "app.log")})
	if len(w.pending) != 1 || !w.pending["f.txt"] {
		t.Errorf("pending = %v, want just f.txt", w.pending)
	}
	if !w.onEvent(dirwatch.Event{All: true}) {
		t.Error("lost events did not ask for a full mirror")
	}
	if !w.onEvent(dirwatch.Event{Path: w.root, Below: true}) {
		t.Error("lost events below the root did not ask for a full mirror")
	}
}

// --- integration tests (real file system events) -------------------------------

func TestWatchIntegrationNewFile(t *testing.T) {
	root := t.TempDir()
	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "s",
		localRoot: root, remoteDir: "dst",
		parallel: 1,
	}

	stop := startTestWatcher(t, opts)
	defer stop()

	os.WriteFile(filepath.Join(root, "new.txt"), []byte("hello"), 0644)

	waitFor(t, "upload new file", 2*time.Second, func() bool {
		return stub.hasUploaded("dst/new.txt")
	})
}

func TestWatchIntegrationDeleteFile(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "old.txt"), []byte("old"), 0644)

	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "s",
		localRoot: root, remoteDir: "dst",
		parallel: 1,
	}

	stop := startTestWatcher(t, opts)
	defer stop()

	os.Remove(filepath.Join(root, "old.txt"))

	waitFor(t, "delete old file", 2*time.Second, func() bool {
		return stub.hasDeleted("dst/old.txt")
	})
}

func TestWatchIntegrationModifyFile(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "edit.txt"), []byte("v1"), 0644)

	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "s",
		localRoot: root, remoteDir: "dst",
		parallel: 1,
	}

	stop := startTestWatcher(t, opts)
	defer stop()

	// Wait for the initial file to be picked up (it will be detected on seed
	// as existing, but only gets synced if there's a Create event).
	// Re-write to trigger a Write event.
	time.Sleep(100 * time.Millisecond)
	os.WriteFile(filepath.Join(root, "edit.txt"), []byte("v2"), 0644)

	waitFor(t, "upload modified file", 2*time.Second, func() bool {
		stub.mu.Lock()
		defer stub.mu.Unlock()
		f, ok := stub.uploaded["dst/edit.txt"]
		return ok && f.content == "v2"
	})
}

func TestWatchIntegrationNewDirectory(t *testing.T) {
	root := t.TempDir()
	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "s",
		localRoot: root, remoteDir: "dst",
		parallel: 1,
	}

	stop := startTestWatcher(t, opts)
	defer stop()

	newDir := filepath.Join(root, "pkg")
	os.MkdirAll(newDir, 0755)
	os.WriteFile(filepath.Join(newDir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(newDir, "b.txt"), []byte("b"), 0644)

	waitFor(t, "upload new directory contents", 2*time.Second, func() bool {
		return stub.hasUploaded("dst/pkg/a.txt") && stub.hasUploaded("dst/pkg/b.txt")
	})
}

func TestWatchIntegrationExcludedFile(t *testing.T) {
	root := t.TempDir()
	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "s",
		localRoot: root, remoteDir: "dst",
		excludes: []string{"node_modules"},
		parallel: 1,
	}

	stop := startTestWatcher(t, opts)
	defer stop()

	os.MkdirAll(filepath.Join(root, "node_modules"), 0755)
	os.WriteFile(filepath.Join(root, "node_modules", "lib.js"), []byte("ignored"), 0644)
	os.WriteFile(filepath.Join(root, "app.js"), []byte("ok"), 0644)

	waitFor(t, "upload non-excluded file", 2*time.Second, func() bool {
		return stub.hasUploaded("dst/app.js")
	})

	// Give a little extra time to ensure the excluded file is NOT uploaded.
	time.Sleep(200 * time.Millisecond)
	if stub.hasUploaded("dst/node_modules/lib.js") {
		t.Error("excluded file should not be uploaded")
	}
}

func TestWatchIntegrationNestedDirectory(t *testing.T) {
	root := t.TempDir()
	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "s",
		localRoot: root, remoteDir: "dst",
		parallel: 1,
	}

	stop := startTestWatcher(t, opts)
	defer stop()

	// Create a/b/c/file.txt — exercises recursive directory watching + upload.
	deep := filepath.Join(root, "a", "b", "c")
	os.MkdirAll(deep, 0755)
	os.WriteFile(filepath.Join(deep, "file.txt"), []byte("deep"), 0644)

	waitFor(t, "upload deeply nested file", 2*time.Second, func() bool {
		return stub.hasUploaded("dst/a/b/c/file.txt")
	})
}

// A folder moved in already filled, as an unpack or a move makes, has all
// it holds uploaded; moved out again, it is deleted.
func TestWatchIntegrationFolderMovedInAndOut(t *testing.T) {
	root := t.TempDir()
	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "s",
		localRoot: root, remoteDir: "dst",
		parallel: 1,
	}
	stop := startTestWatcher(t, opts)
	defer stop()

	staging := t.TempDir()
	writeTree(t, staging, map[string]string{"pkg/deep/a.txt": "a", "pkg/deep/b.txt": "b"})
	if err := os.Rename(filepath.Join(staging, "pkg"), filepath.Join(root, "pkg")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "files of a folder moved in", 3*time.Second, func() bool {
		return stub.hasUploaded("dst/pkg/deep/a.txt") && stub.hasUploaded("dst/pkg/deep/b.txt")
	})

	if err := os.Rename(filepath.Join(root, "pkg"), filepath.Join(staging, "out")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "a folder moved out", 3*time.Second, func() bool {
		return stub.hasDeleted("dst/pkg")
	})
}

func TestWatchSkipSocket(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "real.txt"), []byte("ok"), 0644)

	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "s",
		localRoot: root, remoteDir: "dst",
		parallel: 1,
	}
	w := newTestWatcher(t, opts)

	// Create a socket file — syncPath should skip it without error.
	sockPath := filepath.Join(root, "daemon.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Skipf("can't create socket: %v", err)
	}
	defer ln.Close()

	w.syncPath(context.Background(), "daemon.sock")

	if stub.hasUploaded("dst/daemon.sock") {
		t.Error("socket file should not be uploaded")
	}
}

func TestWatchDefaultExcludesSwP(t *testing.T) {
	root := t.TempDir()
	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "s",
		localRoot: root, remoteDir: "dst",
		parallel: 1,
		excludes: append([]string{}, watchDefaultExcludes...), // simulate runWatch having added them
	}

	stop := startTestWatcher(t, opts)
	defer stop()

	os.WriteFile(filepath.Join(root, "app.js"), []byte("ok"), 0644)
	os.WriteFile(filepath.Join(root, ".app.js.swp"), []byte("swap"), 0644)

	waitFor(t, "upload non-swap file", 2*time.Second, func() bool {
		return stub.hasUploaded("dst/app.js")
	})
	time.Sleep(200 * time.Millisecond)
	if stub.hasUploaded("dst/.app.js.swp") {
		t.Error("vim swap file should not be uploaded")
	}
}

func TestWatchSyncPathMakesFolders(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("folder modes are not mirrored from Windows")
	}
	root := t.TempDir()
	stub := newStubClient()
	opts := &mirrorOptions{client: stub, spaceID: "sp", spaceName: "s", localRoot: root, remoteDir: "dst", parallel: 1}
	w := newTestWatcher(t, opts)

	os.Mkdir(filepath.Join(root, "empty"), 0755)
	w.syncPath(context.Background(), "empty")
	if stub.dirs["dst/empty"] != 0755 {
		t.Fatalf("new folder not made: %v", stub.dirs)
	}
	// Unchanged: not made again.
	w.syncPath(context.Background(), "empty")
	if n := strings.Count(strings.Join(stub.ops, "|"), "mkdir dst/empty"); n != 1 {
		t.Errorf("made %d times", n)
	}
	// Mode changed: given it.
	os.Chmod(filepath.Join(root, "empty"), 0700)
	w.syncPath(context.Background(), "empty")
	if stub.dirs["dst/empty"] != 0700 {
		t.Errorf("mode not sent: %o", stub.dirs["dst/empty"])
	}
}

// What the space is known to have comes from the full mirror's walk, so a
// change made after the walk passed is still sent when its event comes.
func TestWatchChangeDuringFullMirror(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"a.txt": "a", "gone.txt": "g"})
	stub := newStubClient()
	opts := &mirrorOptions{client: stub, spaceID: "sp", spaceName: "s", localRoot: root, remoteDir: "dst", parallel: 1}
	w, err := newMirrorWatcher(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.close)
	if _, err := w.mirror(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Made after the walk, as while the uploads run.
	os.Remove(filepath.Join(root, "gone.txt"))
	os.MkdirAll(filepath.Join(root, "new"), 0755)
	os.WriteFile(filepath.Join(root, "new", "b.txt"), []byte("b"), 0644)
	w.pending["gone.txt"], w.pending["new"], w.pending["new/b.txt"] = true, true, true
	w.flush(context.Background())
	if !stub.hasDeleted("dst/gone.txt") || !stub.hasUploaded("dst/new/b.txt") {
		t.Errorf("deleted gone.txt %v, uploaded new/b.txt %v: ops %v", stub.hasDeleted("dst/gone.txt"), stub.hasUploaded("dst/new/b.txt"), stub.ops)
	}
}

// A path a full mirror couldn't send is retried on its own, and a live
// change is sent after the debounce without waiting for it.
func TestWatchFailedPathDoesNotHoldUpLiveChanges(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"a.txt": "a"})
	stub := newStubClient()
	stub.failWrites = 1000
	opts := &mirrorOptions{client: stub, spaceID: "sp", spaceName: "s", localRoot: root, remoteDir: "dst", parallel: 1}
	w, err := newMirrorWatcher(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.close)
	if _, err := w.mirror(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !w.failed["a.txt"] || w.knowsAt("a.txt") {
		t.Fatalf("failed %v, known %v: want a.txt to try again", w.failed, w.known)
	}
	stub.mu.Lock()
	stub.failWrites = 0
	stub.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); w.loop(context.Background(), ctx, func() {}) }()
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	os.WriteFile(filepath.Join(root, "live.txt"), []byte("live"), 0644)
	waitFor(t, "live.txt", 3*time.Second, func() bool { return stub.hasUploaded("dst/live.txt") })
	took := time.Since(start)
	waitFor(t, "a.txt retried", 5*time.Second, func() bool { return stub.hasUploaded("dst/a.txt") })
	cancel()
	<-done
	if took > debounceInterval+700*time.Millisecond {
		t.Errorf("live change took %s", took)
	}
}

// A file whose mode alone changed has its mode set, not its content sent.
func TestWatchModeOnlyChangeSetsMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not mirrored from Windows")
	}
	root := t.TempDir()
	writeTree(t, root, map[string]string{"run.sh": "echo hi"})
	stub := newStubClient()
	opts := &mirrorOptions{client: stub, spaceID: "sp", spaceName: "s", localRoot: root, remoteDir: "dst", parallel: 1}
	w := newTestWatcher(t, opts)
	os.Chmod(filepath.Join(root, "run.sh"), 0755)
	w.syncPath(context.Background(), "run.sh")
	if strings.Join(stub.ops, "|") != "chmod dst/run.sh" || stub.uploaded["dst/run.sh"].perm != 0755 {
		t.Errorf("ops %v, perm %o: want only the mode set", stub.ops, stub.uploaded["dst/run.sh"].perm)
	}
	// Content changed too: sent.
	os.WriteFile(filepath.Join(root, "run.sh"), []byte("echo bye"), 0755)
	w.syncPath(context.Background(), "run.sh")
	if !strings.Contains(strings.Join(stub.ops, "|"), "write dst/run.sh") {
		t.Errorf("content change not sent: %v", stub.ops)
	}
}

// In watch mode a symlink a full mirror couldn't remove is removed by the
// retry before the file is written, and what was below a blocked folder is
// sent then too.
func TestWatchFailedConflictDeleteRetriedFirst(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"conf": "local", "dir/a.txt": "a"})
	stub := newStubClient()
	stub.remoteEntries = []apiclient.FindEntry{{Path: "dst/conf", LinkTarget: "/etc/passwd"}, {Path: "dst/dir", Size: 3}}
	stub.failDeletes = map[string]bool{"dst/conf": true, "dst/dir": true}
	opts := &mirrorOptions{client: stub, spaceID: "sp", spaceName: "s", localRoot: root, remoteDir: "dst", parallel: 1}
	w, err := newMirrorWatcher(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.close)
	if _, err := w.mirror(context.Background()); err != nil {
		t.Fatal(err)
	}
	if w.known["conf"].kind != kindOther || w.knowsAt("dir/a.txt") {
		t.Fatalf("known %v: want conf as something else and dir/a.txt unsent", w.known)
	}
	stub.mu.Lock()
	stub.failDeletes = nil
	stub.ops = nil
	stub.mu.Unlock()
	w.flush(context.Background())
	got := strings.Join(stub.ops, "|")
	if !strings.Contains(got, "delete dst/conf|write dst/conf") || !strings.Contains(got, "write dst/dir/a.txt") {
		t.Errorf("ops %v: want conf deleted before it is written, and dir/a.txt sent", stub.ops)
	}
}
