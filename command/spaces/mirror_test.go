package command_spaces

import (
	"context"
	"hash/crc64"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
)

// stubClient implements mirrorClient for tests. Records every upload and
// delete so assertions can verify the cycle's outcome.
type stubClient struct {
	mu            sync.Mutex
	remoteEntries []apiclient.FindEntry
	remotePaths   []string
	uploaded      map[string]uploadedFile
	deleted       map[string]bool
	symlinks      map[string]string // dest → target
	lastFindReq   apiclient.FindRequest
}

type uploadedFile struct {
	content string
	mtimeNs int64
	perm    uint32
}

func newStubClient() *stubClient {
	return &stubClient{
		uploaded: map[string]uploadedFile{},
		deleted:  map[string]bool{},
		symlinks: map[string]string{},
	}
}

func (s *stubClient) Find(ctx context.Context, spaceID string, req apiclient.FindRequest) (*apiclient.FindResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastFindReq = req
	// Mirror the agent's behaviour: filter dotfiles unless IncludeHidden is
	// set, and zero Hash unless IncludeHash is set. This catches tests that
	// forget to set hash: true when they want hash-based comparison.
	out := make([]apiclient.FindEntry, 0, len(s.remoteEntries))
	for _, e := range s.remoteEntries {
		if !req.IncludeHidden && hasDotSegment(e.Path) {
			continue
		}
		if !req.IncludeHash {
			e.Hash = 0
		}
		out = append(out, e)
	}
	return &apiclient.FindResponse{Success: true, Entries: out}, nil
}

// hasDotSegment reports whether any path segment starts with '.'.
func hasDotSegment(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if strings.HasPrefix(seg, ".") && seg != "." && seg != ".." {
			return true
		}
	}
	return false
}

func (s *stubClient) WriteSpaceFileOpts(ctx context.Context, spaceID, filePath, content, mode string, mtimeNs int64, filePerm uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.uploaded[filePath]
	switch mode {
	case "append":
		if !ok {
			existing = uploadedFile{}
		}
		existing.content += content
	case "prepend":
		if !ok {
			existing = uploadedFile{}
		}
		existing.content = content + existing.content
	default: // overwrite or empty
		existing.content = content
	}
	// Last-write-wins for metadata — the chunked uploader only sets
	// mtime/perm on the final block.
	if mtimeNs != 0 {
		existing.mtimeNs = mtimeNs
	}
	if filePerm != 0 {
		existing.perm = filePerm
	}
	s.uploaded[filePath] = existing
	return nil
}

func (s *stubClient) DeleteSpaceFile(ctx context.Context, spaceID string, req apiclient.DeleteFileRequest) (*apiclient.DeleteFileResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleted[req.Path] = req.Recursive
	return &apiclient.DeleteFileResponse{Success: true, Removed: 1}, nil
}

func (s *stubClient) CreateSymlinkSpaceFile(ctx context.Context, spaceID, filePath, target string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.symlinks[filePath] = target
	return nil
}

func (s *stubClient) hasUploaded(p string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.uploaded[p]
	return ok
}

func (s *stubClient) hasDeleted(p string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.deleted[p]
	return ok
}

// Compile-time check.
var _ mirrorClient = (*stubClient)(nil)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// --- upload path tests ------------------------------------------------------

func TestMirrorUploadsAllFiles(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"index.php":      "<?php echo 1;",
		"sub/a.php":      "<?php echo 2;",
		"sub/deep/b.php": "<?php echo 3;",
	})

	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "myspace",
		localRoot: root, remoteDir: "/var/www",
		parallel: 2,
	}
	stats, err := opts.run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	for _, want := range []string{
		"/var/www/index.php",
		"/var/www/sub/a.php",
		"/var/www/sub/deep/b.php",
	} {
		if !stub.hasUploaded(want) {
			t.Errorf("expected upload of %s; got %+v", want, stub.uploaded)
		}
	}
	if got := stats.uploaded.Load(); got != 3 {
		t.Errorf("uploaded count: got %d, want 3", got)
	}
}

func TestMirrorExcludes(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"keep.php":           "k",
		"node_modules/react": "r",
		"debug.log":          "l",
		"sub/.env":           "s",
		"sub/keep2.php":      "k2",
	})

	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "myspace",
		localRoot: root, remoteDir: "/var/www",
		excludes: []string{"node_modules", "*.log", ".env"},
		parallel: 2,
	}
	_, err := opts.run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if !stub.hasUploaded("/var/www/keep.php") {
		t.Errorf("keep.php should be uploaded")
	}
	if !stub.hasUploaded("/var/www/sub/keep2.php") {
		t.Errorf("sub/keep2.php should be uploaded")
	}
	for _, shouldNot := range []string{
		"/var/www/node_modules/react",
		"/var/www/debug.log",
		"/var/www/sub/.env",
	} {
		if stub.hasUploaded(shouldNot) {
			t.Errorf("%s should NOT be uploaded (excluded)", shouldNot)
		}
	}
}

func TestMirrorDryRunNoIO(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"a.php": "x"})

	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "myspace",
		localRoot: root, remoteDir: "/var/www",
		parallel: 1,
		dryRun:   true,
	}
	stats, err := opts.run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(stub.uploaded) != 0 {
		t.Errorf("dry-run should not call WriteSpaceFileOpts; got %+v", stub.uploaded)
	}
	if got := stats.uploaded.Load(); got != 1 {
		t.Errorf("dry-run should still count intended uploads: got %d, want 1", got)
	}
}

func TestMirrorRejectsFileAsLocal(t *testing.T) {
	// run() assumes a directory; the guard is in the CLI Run wrapper. Verify
	// here that a file-as-root produces zero work (no panic).
	root := t.TempDir()
	single := filepath.Join(root, "file.txt")
	if err := os.WriteFile(single, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "myspace",
		localRoot: single, remoteDir: "/var/www",
		parallel: 1,
	}
	stats, _ := opts.run(context.Background())
	if got := stats.uploaded.Load(); got != 0 {
		t.Errorf("file-as-root: expected 0 uploads, got %d", got)
	}
}

// --- delete phase tests -----------------------------------------------------

func TestMirrorDeletesExtras(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"keep.php": "k"})

	stub := newStubClient()
	// Remote already has keep.php plus two extras (one file, one dir).
	stub.remoteEntries = []apiclient.FindEntry{
		{Path: "/var/www/keep.php", Size: 1, Mtime: float64(time.Now().UnixNano()) / 1e9, IsDir: false},
		{Path: "/var/www/stale.php", Size: 1, IsDir: false},
		{Path: "/var/www/old_dir", IsDir: true},
	}

	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "myspace",
		localRoot: root, remoteDir: "/var/www",
		parallel: 1,
	}
	stats, err := opts.run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if !stub.hasDeleted("/var/www/stale.php") {
		t.Errorf("stale.php should be deleted; got %+v", stub.deleted)
	}
	if !stub.hasDeleted("/var/www/old_dir") {
		t.Errorf("old_dir should be deleted; got %+v", stub.deleted)
	}
	// keep.php is local + remote, must not be deleted.
	if stub.hasDeleted("/var/www/keep.php") {
		t.Errorf("keep.php must not be deleted")
	}
	if got := stats.deleted.Load(); got != 2 {
		t.Errorf("deleted count: got %d, want 2", got)
	}
}

func TestMirrorExcludesShieldFromDelete(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"keep.php": "k"})

	stub := newStubClient()
	// Remote has a stale file under node_modules/ and a stale .log that the
	// excludes should shield from deletion.
	stub.remoteEntries = []apiclient.FindEntry{
		{Path: "/var/www/keep.php"},
		{Path: "/var/www/node_modules/old.js"},
		{Path: "/var/www/debug.log"},
	}

	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "myspace",
		localRoot: root, remoteDir: "/var/www",
		excludes: []string{"node_modules", "*.log"},
		parallel: 1,
	}
	_, err := opts.run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	for _, shielded := range []string{
		"/var/www/node_modules/old.js",
		"/var/www/debug.log",
	} {
		if stub.hasDeleted(shielded) {
			t.Errorf("excluded path %s must not be deleted", shielded)
		}
	}
}

func TestMirrorPreservesMtimeAndPerm(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "script.sh")
	if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	mtime := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(target, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "myspace",
		localRoot: root, remoteDir: "/var/www",
		parallel: 1,
	}
	_, err := opts.run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	got, ok := stub.uploaded["/var/www/script.sh"]
	if !ok {
		t.Fatalf("script.sh not uploaded: %+v", stub.uploaded)
	}
	if got.mtimeNs != mtime.UnixNano() {
		t.Errorf("mtime ns: got %d, want %d", got.mtimeNs, mtime.UnixNano())
	}
	if got.perm != 0755 {
		t.Errorf("perm: got %o, want 755", got.perm)
	}
}

// TestMirrorSkipsUnchangedFiles is the key idempotence test: a second run
// against a remote that already has matching size+mtime should upload
// nothing and skip everything.
func TestMirrorSkipsUnchangedFiles(t *testing.T) {
	root := t.TempDir()
	c := []byte("<?php echo 1;")
	path := filepath.Join(root, "a.php")
	if err := os.WriteFile(path, c, 0644); err != nil {
		t.Fatal(err)
	}
	mtime := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	stub := newStubClient()
	stub.remoteEntries = []apiclient.FindEntry{
		{Path: "/var/www/a.php", Size: int64(len(c)), Mtime: float64(mtime.UnixNano()) / 1e9, IsDir: false},
	}

	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "myspace",
		localRoot: root, remoteDir: "/var/www",
		parallel: 1,
	}
	stats, err := opts.run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := stats.uploaded.Load(); got != 0 {
		t.Errorf("uploaded: got %d, want 0 (everything should skip)", got)
	}
	if got := stats.skipped.Load(); got != 1 {
		t.Errorf("skipped: got %d, want 1", got)
	}
	if len(stub.uploaded) != 0 {
		t.Errorf("no files should be uploaded; got %+v", stub.uploaded)
	}
}

// TestMirrorReuploadsWhenSizeDiffers confirms that a size mismatch forces
// re-upload even when mtime matches.
func TestMirrorReuploadsWhenSizeDiffers(t *testing.T) {
	root := t.TempDir()
	c := []byte("<?php echo longer;")
	path := filepath.Join(root, "a.php")
	if err := os.WriteFile(path, c, 0644); err != nil {
		t.Fatal(err)
	}
	mtime := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	stub := newStubClient()
	// Remote has same mtime, smaller size.
	stub.remoteEntries = []apiclient.FindEntry{
		{Path: "/var/www/a.php", Size: 5, Mtime: float64(mtime.UnixNano()) / 1e9, IsDir: false},
	}

	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "myspace",
		localRoot: root, remoteDir: "/var/www",
		parallel: 1,
	}
	stats, err := opts.run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := stats.uploaded.Load(); got != 1 {
		t.Errorf("uploaded: got %d, want 1 (size differs → re-upload)", got)
	}
}

// TestMirrorReuploadsWhenMtimeDiffers confirms that an mtime drift beyond
// tolerance forces re-upload.
func TestMirrorReuploadsWhenMtimeDiffers(t *testing.T) {
	root := t.TempDir()
	c := []byte("<?php echo 1;")
	path := filepath.Join(root, "a.php")
	if err := os.WriteFile(path, c, 0644); err != nil {
		t.Fatal(err)
	}
	localMtime := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(path, localMtime, localMtime); err != nil {
		t.Fatal(err)
	}

	stub := newStubClient()
	// Remote has same size, but mtime 1 day ago (way past ±3s tolerance).
	stub.remoteEntries = []apiclient.FindEntry{
		{Path: "/var/www/a.php", Size: int64(len(c)), Mtime: float64(time.Now().Add(-24*time.Hour).UnixNano()) / 1e9, IsDir: false},
	}

	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "myspace",
		localRoot: root, remoteDir: "/var/www",
		parallel: 1,
	}
	stats, err := opts.run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := stats.uploaded.Load(); got != 1 {
		t.Errorf("uploaded: got %d, want 1 (mtime drift → re-upload)", got)
	}
}

// TestMirrorDebugLogsUploadReason confirms --debug emits a per-file reason
// line for files that get queued for upload. We assert the reason text
// contains the distinguishing token, not the exact format.
func TestMirrorDebugLogsUploadReason(t *testing.T) {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = oldStderr })

	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"new.php":     "n",
		"changed.php": "c",
	})
	// Force changed.php's mtime so we can make the remote disagree past tolerance.
	changedMtime := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(filepath.Join(root, "changed.php"), changedMtime, changedMtime); err != nil {
		t.Fatal(err)
	}

	stub := newStubClient()
	stub.remoteEntries = []apiclient.FindEntry{
		{Path: "/var/www/changed.php", Size: 1, Mtime: float64(time.Now().Add(-24*time.Hour).UnixNano()) / 1e9, IsDir: false},
	}

	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "myspace",
		localRoot: root, remoteDir: "/var/www",
		parallel: 1,
		debug:    true,
	}
	_, err := opts.run(context.Background())
	w.Close()
	os.Stderr = oldStderr

	out, _ := io.ReadAll(r)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "new.php — new") {
		t.Errorf("expected a debug line for new.php (reason: new); got:\n%s", s)
	}
	if !strings.Contains(s, "changed.php — mtime drift") {
		t.Errorf("expected a debug line for changed.php (reason: mtime drift); got:\n%s", s)
	}
}

// TestMirrorDefaultHasNoPerFileOutput confirms that without --verbose or
// --debug, the walker emits NO per-file lines — just the work happens
// silently. The summary line is printed by the CLI Run wrapper (not by
// run() itself), so this test asserts an empty stderr.
func TestMirrorDefaultHasNoPerFileOutput(t *testing.T) {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = oldStderr })

	root := t.TempDir()
	writeTree(t, root, map[string]string{"new.php": "n"})

	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "myspace",
		localRoot: root, remoteDir: "/var/www",
		parallel: 1,
		// verbose and debug both intentionally false
	}
	_, _ = opts.run(context.Background())
	w.Close()
	os.Stderr = oldStderr

	out, _ := io.ReadAll(r)
	if s := string(out); s != "" {
		t.Errorf("default mode should produce no per-file output; got:\n%s", s)
	}
}

// TestMirrorVerbosePrintsUploadLine confirms --verbose surfaces the per-file
// upload arrow that default mode suppresses.
func TestMirrorVerbosePrintsUploadLine(t *testing.T) {
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = oldStderr })

	root := t.TempDir()
	writeTree(t, root, map[string]string{"new.php": "n"})

	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "myspace",
		localRoot: root, remoteDir: "/var/www",
		parallel: 1,
		verbose:  true,
	}
	_, err := opts.run(context.Background())
	w.Close()
	os.Stderr = oldStderr
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), "↑ new.php") {
		t.Errorf("verbose mode should print the upload line; got:\n%s", out)
	}
}

// TestMirrorChunkedUploadReassemblesCorrectly proves that files larger than
// chunkSize are uploaded in blocks (first overwrite, rest append) and the
// destination ends up with byte-identical content + the source's mtime and
// perm. The stub concatenates based on mode (same as the agent), so the
// test just verifies the final stored content.
//
// chunkSize is a var so we shrink it to 100 bytes here — the code path is
// identical regardless of the threshold value, just much faster.
func TestMirrorChunkedUploadReassemblesCorrectly(t *testing.T) {
	orig := chunkSize
	chunkSize = 100
	defer func() { chunkSize = orig }()

	root := t.TempDir()
	// 250 bytes → 3 chunks: 100 (overwrite) + 100 (append) + 50 (append, last).
	payload := make([]byte, 250)
	for i := range payload {
		payload[i] = byte(i)
	}
	src := filepath.Join(root, "big.bin")
	if err := os.WriteFile(src, payload, 0644); err != nil {
		t.Fatal(err)
	}
	mtime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(src, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	stub := newStubClient()
	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "myspace",
		localRoot: root, remoteDir: "/var/www",
		parallel: 1,
	}
	_, err := opts.run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got, ok := stub.uploaded["/var/www/big.bin"]
	if !ok {
		t.Fatalf("big.bin not uploaded: %+v", stub.uploaded)
	}
	if len(got.content) != len(payload) {
		t.Fatalf("reassembled size: got %d, want %d", len(got.content), len(payload))
	}
	if got.content != string(payload) {
		// Find first diff.
		for i := 0; i < len(payload); i++ {
			if got.content[i] != payload[i] {
				t.Errorf("first byte diff at %d: got %x, want %x", i, got.content[i], payload[i])
				break
			}
		}
	}
	if got.mtimeNs != mtime.UnixNano() {
		t.Errorf("mtime: got %d, want %d", got.mtimeNs, mtime.UnixNano())
	}
	if got.perm != 0644 {
		t.Errorf("perm: got %o, want 0644", got.perm)
	}
}

// TestMirrorSkipsDotfilesWhenRemoteHasThem is the regression test for the
// "every dotfile looks new forever" bug. Mirror's local walker sees
// .gitignore, .task/, etc.; the agent's Find filters dotfiles unless
// IncludeHidden=true is set in the request. If mirror forgets that flag,
// every dotfile re-uploads on every run.
func TestMirrorSkipsDotfilesWhenRemoteHasThem(t *testing.T) {
	root := t.TempDir()
	// Local has a dotfile.
	c := []byte("node_modules/\n")
	path := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(path, c, 0644); err != nil {
		t.Fatal(err)
	}
	mtime := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	stub := newStubClient()
	// Remote has the same dotfile with matching size+mtime.
	stub.remoteEntries = []apiclient.FindEntry{
		{Path: "/var/www/.gitignore", Size: int64(len(c)), Mtime: float64(mtime.UnixNano()) / 1e9, IsDir: false},
	}

	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "myspace",
		localRoot: root, remoteDir: "/var/www",
		parallel: 1,
	}
	stats, err := opts.run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// The Find request MUST set IncludeHidden=true — otherwise the agent
	// filters .gitignore from the response and mirror re-uploads it.
	if !stub.lastFindReq.IncludeHidden {
		t.Errorf("Find request should set IncludeHidden=true so dotfiles are visible; got %+v", stub.lastFindReq)
	}
	// And the dotfile should be skipped (matched), not uploaded.
	if got := stats.uploaded.Load(); got != 0 {
		t.Errorf("uploaded: got %d, want 0 (.gitignore should match remote and skip)", got)
	}
	if got := stats.skipped.Load(); got != 1 {
		t.Errorf("skipped: got %d, want 1", got)
	}
}

// TestMirrorHashBasedSkip verifies that when the remote provides a hash
// (IncludeHash=true), mirror uses it for the skip decision instead of mtime.
// Same content → same hash → skip even when mtime drifts.
func TestMirrorHashBasedSkip(t *testing.T) {
	root := t.TempDir()
	content := []byte("<?php echo 1;")
	path := filepath.Join(root, "a.php")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	// Set local mtime to 1 hour ago.
	localMtime := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(path, localMtime, localMtime); err != nil {
		t.Fatal(err)
	}

	// Compute the expected hash.
	h := crc64.New(crc64.MakeTable(crc64.ISO))
	h.Write(content)
	expectedHash := h.Sum64()

	stub := newStubClient()
	// Remote has SAME size + hash but DIFFERENT mtime (24h ago).
	// With mtime-only, this would re-upload. With hash, it skips.
	stub.remoteEntries = []apiclient.FindEntry{
		{
			Path:  "/var/www/a.php",
			Size:  int64(len(content)),
			Mtime: float64(time.Now().Add(-24 * time.Hour).UnixNano()) / 1e9,
			IsDir: false,
			Hash:  expectedHash,
		},
	}

	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "myspace",
		localRoot: root, remoteDir: "/var/www",
		parallel: 1,
		hash:    true,
	}
	stats, err := opts.run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := stats.uploaded.Load(); got != 0 {
		t.Errorf("uploaded: got %d, want 0 (hash matches → skip)", got)
	}
	if got := stats.skipped.Load(); got != 1 {
		t.Errorf("skipped: got %d, want 1", got)
	}
}

// TestMirrorHashMismatchForcesReupload verifies that a hash mismatch (same
// size, different content) forces a re-upload even though the size matches.
func TestMirrorHashMismatchForcesReupload(t *testing.T) {
	root := t.TempDir()
	content := []byte("<?php echo new;")
	path := filepath.Join(root, "a.php")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	// Remote has same size but different content (hash won't match).
	stub := newStubClient()
	stub.remoteEntries = []apiclient.FindEntry{
		{
			Path:  "/var/www/a.php",
			Size:  int64(len(content)), // same size
			Mtime: float64(time.Now().UnixNano()) / 1e9,
			Hash:  999, // wrong hash
		},
	}

	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "myspace",
		localRoot: root, remoteDir: "/var/www",
		parallel: 1,
		hash:    true,
	}
	stats, err := opts.run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := stats.uploaded.Load(); got != 1 {
		t.Errorf("uploaded: got %d, want 1 (hash mismatch → re-upload)", got)
	}
}

// TestMirrorVerifyReportsMatch exercises --verify: all files match.
func TestMirrorVerifyReportsMatch(t *testing.T) {
	root := t.TempDir()
	content := []byte("<?php echo 1;")
	path := filepath.Join(root, "a.php")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	h := crc64.New(crc64.MakeTable(crc64.ISO))
	h.Write(content)
	expectedHash := h.Sum64()

	stub := newStubClient()
	stub.remoteEntries = []apiclient.FindEntry{
		{Path: "/var/www/a.php", Size: int64(len(content)), Hash: expectedHash, IsDir: false},
	}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "myspace",
		localRoot: root, remoteDir: "/var/www",
		verify: true, hash: true,
	}
	_, err := opts.run(context.Background())
	w.Close()
	os.Stderr = oldStderr
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out, _ := io.ReadAll(r)
	s := string(out)
	if !strings.Contains(s, "1 match") {
		t.Errorf("expected '1 match'; got:\n%s", s)
	}
	if strings.Contains(s, "✗") {
		t.Errorf("no mismatches expected; got:\n%s", s)
	}
}

// TestMirrorVerifyReportsMismatch exercises --verify: file content differs.
func TestMirrorVerifyReportsMismatch(t *testing.T) {
	root := t.TempDir()
	content := []byte("<?php echo new;")
	if err := os.WriteFile(filepath.Join(root, "a.php"), content, 0644); err != nil {
		t.Fatal(err)
	}

	stub := newStubClient()
	stub.remoteEntries = []apiclient.FindEntry{
		{Path: "/var/www/a.php", Size: int64(len(content)), Hash: 999, IsDir: false},
	}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "myspace",
		localRoot: root, remoteDir: "/var/www",
		verify: true, hash: true,
	}
	_, err := opts.run(context.Background())
	w.Close()
	os.Stderr = oldStderr
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out, _ := io.ReadAll(r)
	s := string(out)
	if !strings.Contains(s, "1 differ") {
		t.Errorf("expected '1 differ'; got:\n%s", s)
	}
	if !strings.Contains(s, "hash mismatch") {
		t.Errorf("expected 'hash mismatch'; got:\n%s", s)
	}
}

// TestMirrorVerifyReportsMissingRemote exercises --verify: file exists
// locally but not on remote.
func TestMirrorVerifyReportsMissingRemote(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "only-local.php"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	stub := newStubClient()
	stub.remoteEntries = nil // empty remote

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "myspace",
		localRoot: root, remoteDir: "/var/www",
		verify: true, hash: true,
	}
	_, err := opts.run(context.Background())
	w.Close()
	os.Stderr = oldStderr
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out, _ := io.ReadAll(r)
	s := string(out)
	if !strings.Contains(s, "missing on remote") {
		t.Errorf("expected 'missing on remote'; got:\n%s", s)
	}
}

// TestMirrorDirOnlyExcludeShieldsRemoteDir is the regression test for the
// delete-phase bug: a dirOnly exclude pattern ("build/") must shield the
// remote directory entry from deletion. Previously the delete phase passed
// isDir=false to the exclude matcher, which skipped dirOnly patterns,
// leaking the directory through to deletion.
func TestMirrorDirOnlyExcludeShieldsRemoteDir(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"keep.php": "k"})

	stub := newStubClient()
	// Remote has a stale "build" directory that's excluded by "build/".
	// The directory entry itself must NOT be deleted.
	stub.remoteEntries = []apiclient.FindEntry{
		{Path: "/var/www/keep.php"},
		{Path: "/var/www/build", IsDir: true},
		{Path: "/var/www/build/output.txt"},
	}

	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "myspace",
		localRoot: root, remoteDir: "/var/www",
		excludes: []string{"build/"}, // dirOnly pattern
		parallel: 1,
	}
	_, err := opts.run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if stub.hasDeleted("/var/www/build") {
		t.Errorf("build/ dir must NOT be deleted (excluded by build/)")
	}
	if stub.hasDeleted("/var/www/build/output.txt") {
		t.Errorf("build/output.txt must NOT be deleted (ancestor excluded)")
	}
}

// --- helpers tests ----------------------------------------------------------

func TestCompileExcludesDirOnly(t *testing.T) {
	ex := compileExcludes([]string{"build/"})
	if !ex("build", true) {
		t.Errorf("build/ should exclude dir 'build'")
	}
	if ex("build", false) {
		t.Errorf("build/ should NOT exclude a file named 'build'")
	}
	if !ex("build/out.txt", false) {
		t.Errorf("build/ should exclude descendants (transitive)")
	}
}

func TestCompileExcludesStripsDotPrefix(t *testing.T) {
	// "./node_modules" and "./node_modules/" must behave identically to
	// "node_modules" / "node_modules/". Relative paths from the walker are
	// slash-style without a leading "./".
	cases := []struct {
		patterns []string
		rel      string
		isDir    bool
		want     bool
	}{
		{[]string{"./node_modules"}, "node_modules", true, true},
		{[]string{"./node_modules"}, "node_modules/pkg/index.js", false, true},
		{[]string{"./node_modules/"}, "node_modules", true, true},
		{[]string{"./node_modules/"}, "node_modules/pkg/index.js", false, true},
		{[]string{"./node_modules/"}, "node_modules", false, false},
		{[]string{"./src/*.log"}, "src/run.log", false, true},
		{[]string{"./*.log"}, "run.log", false, true},
		{[]string{"./build/"}, "app/x.txt", false, false},
	}
	for _, c := range cases {
		ex := compileExcludes(c.patterns)
		got := ex(c.rel, c.isDir)
		if got != c.want {
			t.Errorf("compileExcludes(%v)(%q, isDir=%v): got %v, want %v",
				c.patterns, c.rel, c.isDir, got, c.want)
		}
	}
}

func TestMirrorExcludedRemotePathsNotDeleted(t *testing.T) {
	// Regression: excluded paths must be stripped from the remote set before
	// the delete phase, so they're never compared or deleted.
	root := t.TempDir()
	writeTree(t, root, map[string]string{"app.py": "x"})

	stub := newStubClient()
	// Remote has a stale node_modules tree that's excluded locally.
	stub.remoteEntries = []apiclient.FindEntry{
		{Path: "/var/www/app.py"},
		{Path: "/var/www/node_modules", IsDir: true},
		{Path: "/var/www/node_modules/express/index.js"},
		{Path: "/var/www/node_modules/lodash/index.js"},
	}

	opts := &mirrorOptions{
		client: stub, spaceID: "sp", spaceName: "myspace",
		localRoot: root, remoteDir: "/var/www",
		excludes: []string{"./node_modules/"}, // ./ prefix like the bug report
		parallel: 1,
	}
	_, err := opts.run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if stub.hasDeleted("/var/www/node_modules") {
		t.Errorf("node_modules dir must NOT be deleted")
	}
	if stub.hasDeleted("/var/www/node_modules/express/index.js") {
		t.Errorf("node_modules descendants must NOT be deleted")
	}
	if stub.hasDeleted("/var/www/node_modules/lodash/index.js") {
		t.Errorf("node_modules descendants must NOT be deleted")
	}
}

func TestRelativiseRemote(t *testing.T) {
	cases := []struct{ base, sub, want string }{
		{".", "foo.php", "foo.php"},
		{"src", "src/a.php", "a.php"},
		{"src", "other/a.php", ""},
		{"src", "src", ""},
		{"/var/www", "/var/www/html/index.php", "html/index.php"},
		{"/var/www", "/opt/other/x", ""},
	}
	for _, c := range cases {
		got := relativiseRemote(c.base, c.sub)
		if got != c.want {
			t.Errorf("relativiseRemote(%q, %q): got %q, want %q", c.base, c.sub, got, c.want)
		}
	}
}
