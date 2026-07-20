package command_spaces

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// --- helpers ----------------------------------------------------------------

// startTestWatcher creates a real fsnotify watcher on localRoot and runs a
// minimal event loop (onEvent + ticker flush) in a goroutine. Returns the
// watcher handle and a stop func. The caller makes filesystem changes and
// sleeps briefly to let the debounce + flush pick them up.
func startTestWatcher(t *testing.T, opts *mirrorOptions) func() {
	t.Helper()
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("create watcher: %v", err)
	}

	w := &mirrorWatcher{
		opts:     opts,
		excludes: compileExcludes(opts.excludes),
		fsw:      fsw,
		pending:  make(map[string]fsnotify.Op),
	}
	w.seedDirs(opts.localRoot)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		ticker := time.NewTicker(50 * time.Millisecond) // faster than prod for tests
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				close(done)
				return
			case event, ok := <-fsw.Events:
				if !ok {
					close(done)
					return
				}
				w.onEvent(event)
			case <-fsw.Errors:
			case <-ticker.C:
				w.flush(ctx)
			}
		}
	}()

	return func() {
		cancel()
		fsw.Close()
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
	fsw, _ := fsnotify.NewWatcher()
	defer fsw.Close()
	w := &mirrorWatcher{
		opts:     opts,
		excludes: compileExcludes(nil),
		fsw:      fsw,
		pending:  make(map[string]fsnotify.Op),
	}

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
	fsw, _ := fsnotify.NewWatcher()
	defer fsw.Close()
	w := &mirrorWatcher{
		opts:     opts,
		excludes: compileExcludes(nil),
		fsw:      fsw,
		pending:  make(map[string]fsnotify.Op),
	}

	// Path doesn't exist locally → should be deleted from remote.
	w.syncPath(context.Background(), "gone.txt")

	if !stub.hasDeleted("dst/gone.txt") {
		t.Error("expected gone.txt to be deleted from remote")
	}
}

func TestWatchSyncPathUploadsDirContents(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "sub")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "x.txt"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "y.txt"), []byte("yy"), 0644)

	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "s",
		localRoot: root, remoteDir: "dst",
		parallel: 1,
	}
	fsw, _ := fsnotify.NewWatcher()
	defer fsw.Close()
	w := &mirrorWatcher{
		opts:     opts,
		excludes: compileExcludes(nil),
		fsw:      fsw,
		pending:  make(map[string]fsnotify.Op),
	}

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
	fsw, _ := fsnotify.NewWatcher()
	defer fsw.Close()
	w := &mirrorWatcher{
		opts:     opts,
		excludes: compileExcludes(nil),
		fsw:      fsw,
		pending:  make(map[string]fsnotify.Op),
	}

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
	fsw, _ := fsnotify.NewWatcher()
	defer fsw.Close()
	w := &mirrorWatcher{
		opts:     opts,
		excludes: compileExcludes(opts.excludes),
		fsw:      fsw,
		pending:  make(map[string]fsnotify.Op),
	}

	w.syncPath(context.Background(), "secret.key")
	w.syncPath(context.Background(), "app.js")

	if stub.hasUploaded("dst/secret.key") {
		t.Error("excluded file should not be uploaded")
	}
	if !stub.hasUploaded("dst/app.js") {
		t.Error("non-excluded file should be uploaded")
	}
}

// --- onEvent / flush tests --------------------------------------------------

func TestWatchOnEventCoalescesByPath(t *testing.T) {
	root := t.TempDir()
	fsw, _ := fsnotify.NewWatcher()
	defer fsw.Close()
	w := &mirrorWatcher{
		opts:     &mirrorOptions{localRoot: root, remoteDir: "dst"},
		excludes: compileExcludes(nil),
		fsw:      fsw,
		pending:  make(map[string]fsnotify.Op),
	}

	w.onEvent(fsnotify.Event{Name: filepath.Join(root, "f.txt"), Op: fsnotify.Create})
	w.onEvent(fsnotify.Event{Name: filepath.Join(root, "f.txt"), Op: fsnotify.Write})

	w.mu.Lock()
	op := w.pending["f.txt"]
	w.mu.Unlock()

	if op&(fsnotify.Create|fsnotify.Write) == 0 {
		t.Errorf("expected coalesced Create|Write, got %v", op)
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
	fsw, _ := fsnotify.NewWatcher()
	defer fsw.Close()
	w := &mirrorWatcher{
		opts:     opts,
		excludes: compileExcludes(nil),
		fsw:      fsw,
		pending:  make(map[string]fsnotify.Op),
	}

	w.mu.Lock()
	w.pending["a.txt"] = fsnotify.Create
	w.mu.Unlock()

	w.flush(context.Background())

	w.mu.Lock()
	l := len(w.pending)
	w.mu.Unlock()

	if l != 0 {
		t.Errorf("expected pending drained, got %d", l)
	}
	if !stub.hasUploaded("dst/a.txt") {
		t.Error("expected a.txt uploaded after flush")
	}
}

func TestWatchOnEventIgnoresExcluded(t *testing.T) {
	root := t.TempDir()
	fsw, _ := fsnotify.NewWatcher()
	defer fsw.Close()
	w := &mirrorWatcher{
		opts:     &mirrorOptions{localRoot: root, remoteDir: "dst"},
		excludes: compileExcludes([]string{"*.log"}),
		fsw:      fsw,
		pending:  make(map[string]fsnotify.Op),
	}

	w.onEvent(fsnotify.Event{Name: filepath.Join(root, "app.log"), Op: fsnotify.Create})

	w.mu.Lock()
	l := len(w.pending)
	w.mu.Unlock()

	if l != 0 {
		t.Errorf("excluded file should not enter pending, got %d entries", l)
	}
}

// --- integration tests (real fsnotify events) -------------------------------

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

func TestWatchSkipSocket(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "real.txt"), []byte("ok"), 0644)

	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "s",
		localRoot: root, remoteDir: "dst",
		parallel: 1,
	}
	fsw, _ := fsnotify.NewWatcher()
	defer fsw.Close()
	w := &mirrorWatcher{
		opts:     opts,
		excludes: compileExcludes(nil),
		fsw:      fsw,
		pending:  make(map[string]fsnotify.Op),
	}

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
