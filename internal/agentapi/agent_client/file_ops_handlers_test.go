package agent_client

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/vmihailenco/msgpack/v5"
)

// runFindHandler invokes handleFindExecution against an in-memory pipe,
// reads the response, and returns it. Lets us assert the wire shape without a
// real agent/server.
func runFindHandler(t *testing.T, req msg.FindMessage) msg.FindResponse {
	t.Helper()

	client, server := net.Pipe()
	t.Cleanup(func() {
		client.Close()
		server.Close()
	})

	done := make(chan struct{})
	go func() {
		handleFindExecution(server, req)
		close(done)
	}()

	var resp msg.FindResponse
	if err := msg.ReadMessage(client, &resp); err != nil {
		t.Fatalf("reading find response: %v", err)
	}
	<-done
	return resp
}

func mkTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHandleFindExecutionReturnsEntries(t *testing.T) {
	root := t.TempDir()
	mkTree(t, root, map[string]string{
		"a.php":          "<?php echo 1;",
		"sub/b.php":      "<?php echo 2;",
		"sub/big.log":    string(make([]byte, 1000)),
		"sub/deep/c.txt": "c",
	})

	resp := runFindHandler(t, msg.FindMessage{
		Path:            root,
		Recursive:       true,
		Type:            "any",
		IncludeMetadata: true, // opt into the stat-every-match path
	})

	if !resp.Success {
		t.Fatalf("expected success, got error: %s", resp.Error)
	}
	if resp.Entries == nil {
		t.Fatal("expected non-nil Entries when IncludeMetadata=true")
	}
	if resp.Paths != nil {
		t.Errorf("Paths should be empty in metadata mode; got %v", resp.Paths)
	}

	byRel := map[string]msg.FindEntry{}
	for _, e := range resp.Entries {
		rel, err := filepath.Rel(root, e.Path)
		if err != nil {
			t.Fatal(err)
		}
		byRel[filepath.ToSlash(rel)] = e
	}

	cases := []struct {
		rel    string
		isDir  bool
		size   int64
		exists bool
	}{
		{"a.php", false, 13, true},
		{"sub", true, 0, true},
		{"sub/b.php", false, 13, true},
		{"sub/big.log", false, 1000, true},
		{"sub/deep", true, 0, true},
		{"sub/deep/c.txt", false, 1, true},
		{"missing.txt", false, 0, false},
	}
	for _, c := range cases {
		e, ok := byRel[c.rel]
		if !ok {
			if c.exists {
				t.Errorf("expected entry %q missing; got %d entries", c.rel, len(byRel))
			}
			continue
		}
		if !c.exists {
			t.Errorf("unexpected entry %q present", c.rel)
			continue
		}
		if e.IsDir != c.isDir {
			t.Errorf("%q IsDir: got %v, want %v", c.rel, e.IsDir, c.isDir)
		}
		if !c.isDir && e.Size != c.size {
			t.Errorf("%q Size: got %d, want %d", c.rel, e.Size, c.size)
		}
		// Mtime should be ~now (within last minute) for freshly written files.
		age := time.Since(time.Unix(0, int64(e.Mtime*1e9)))
		if age > time.Minute || age < -time.Minute {
			t.Errorf("%q Mtime: age %v out of tolerance", c.rel, age)
		}
	}
}

// TestHandleFindExecutionDefaultReturnsPaths covers the hot path: default
// (no metadata) skips per-entry stats and returns paths only.
func TestHandleFindExecutionDefaultReturnsPaths(t *testing.T) {
	root := t.TempDir()
	mkTree(t, root, map[string]string{
		"a.php":          "x",
		"sub/b.php":      "y",
		"sub/deep/c.txt": "z",
	})

	resp := runFindHandler(t, msg.FindMessage{
		Path:      root,
		Recursive: true,
		Type:      "any",
		// IncludeMetadata intentionally false — default mode.
	})

	if !resp.Success {
		t.Fatalf("expected success: %s", resp.Error)
	}
	if resp.Entries != nil {
		t.Errorf("Entries should be nil in default mode; got %d entries", len(resp.Entries))
	}
	if resp.Paths == nil {
		t.Fatal("expected non-nil Paths in default mode")
	}

	got := map[string]bool{}
	for _, p := range resp.Paths {
		rel, _ := filepath.Rel(root, p)
		got[filepath.ToSlash(rel)] = true
	}
	for _, want := range []string{"a.php", "sub", "sub/b.php", "sub/deep", "sub/deep/c.txt"} {
		if !got[want] {
			t.Errorf("expected path %q in results; got %v", want, got)
		}
	}
}

func TestHandleFindExecutionRootIsFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "single.php")
	if err := os.WriteFile(target, []byte("<?= 1"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("metadata", func(t *testing.T) {
		resp := runFindHandler(t, msg.FindMessage{
			Path:            target,
			Recursive:       true,
			Type:            "any",
			IncludeMetadata: true,
		})
		if !resp.Success {
			t.Fatalf("expected success: %s", resp.Error)
		}
		if len(resp.Entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(resp.Entries))
		}
		if resp.Entries[0].Path != target {
			t.Errorf("path: got %q, want %q", resp.Entries[0].Path, target)
		}
	})

	t.Run("default", func(t *testing.T) {
		resp := runFindHandler(t, msg.FindMessage{
			Path:      target,
			Recursive: true,
			Type:      "any",
		})
		if !resp.Success {
			t.Fatalf("expected success: %s", resp.Error)
		}
		if len(resp.Paths) != 1 || resp.Paths[0] != target {
			t.Errorf("expected paths=[%q], got %v", target, resp.Paths)
		}
	})
}

func TestFindResponseWireFormat(t *testing.T) {
	// Verify the response serialises cleanly through both msgpack and JSON-shaped
	// field tags, so a Python client reading "entries" works.
	original := msg.FindResponse{
		Success: true,
		Entries: []msg.FindEntry{
			{Path: "/a.php", Size: 100, Mtime: 1700000000.5, IsDir: false},
			{Path: "/sub", Size: 0, Mtime: 1700000001.0, IsDir: true},
		},
	}

	encoded, err := msgpack.Marshal(&original)
	if err != nil {
		t.Fatalf("msgpack marshal: %v", err)
	}

	var decoded msg.FindResponse
	if err := msgpack.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("msgpack unmarshal: %v", err)
	}

	if !decoded.Success || len(decoded.Entries) != 2 {
		t.Fatalf("roundtrip lost data: %+v", decoded)
	}
	if decoded.Entries[0].Path != "/a.php" || decoded.Entries[0].Size != 100 {
		t.Errorf("entry 0 wrong: %+v", decoded.Entries[0])
	}
	if decoded.Entries[1].IsDir != true {
		t.Errorf("entry 1 IsDir wrong: %+v", decoded.Entries[1])
	}
}

func TestHandleFindExecutionErrorOnBadType(t *testing.T) {
	root := t.TempDir()
	resp := runFindHandler(t, msg.FindMessage{
		Path: root,
		Type: "block", // invalid
	})
	// Bad type yields an error response from extlibs.FindEntries.
	if resp.Success {
		t.Errorf("expected failure on bad type, got success")
	}
}

// --- DeleteFile handler tests ------------------------------------------------

func runDeleteHandler(t *testing.T, req msg.DeleteFileMessage) msg.DeleteFileResponse {
	t.Helper()

	client, server := net.Pipe()
	t.Cleanup(func() {
		client.Close()
		server.Close()
	})

	done := make(chan struct{})
	go func() {
		handleDeleteFileExecution(server, req)
		close(done)
	}()

	var resp msg.DeleteFileResponse
	if err := msg.ReadMessage(client, &resp); err != nil {
		t.Fatalf("reading delete response: %v", err)
	}
	<-done
	return resp
}

func TestHandleDeleteFileExecutionRemovesFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "remove.me")
	if err := os.WriteFile(target, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	resp := runDeleteHandler(t, msg.DeleteFileMessage{Path: target})
	if !resp.Success {
		t.Fatalf("expected success: %s", resp.Error)
	}
	if resp.Removed != 1 {
		t.Errorf("Removed: got %d, want 1", resp.Removed)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("file still exists after delete: %v", err)
	}
}

func TestHandleDeleteFileExecutionMissingPathIsSuccess(t *testing.T) {
	root := t.TempDir()
	resp := runDeleteHandler(t, msg.DeleteFileMessage{Path: filepath.Join(root, "nope")})
	if !resp.Success {
		t.Fatalf("missing path should be success, got error: %s", resp.Error)
	}
	if resp.Removed != 0 {
		t.Errorf("Removed: got %d, want 0 for missing path", resp.Removed)
	}
}

func TestHandleDeleteFileExecutionRecursive(t *testing.T) {
	root := t.TempDir()
	mkTree(t, root, map[string]string{
		"a.php":      "<?php",
		"sub/b.php":  "<?php",
		"sub/c.php":  "<?php",
		"sub/deep/d": "x",
	})

	resp := runDeleteHandler(t, msg.DeleteFileMessage{Path: filepath.Join(root, "sub"), Recursive: true})
	if !resp.Success {
		t.Fatalf("expected success: %s", resp.Error)
	}
	// sub/ + sub/b.php + sub/c.php + sub/deep/ + sub/deep/d = 5
	if resp.Removed != 5 {
		t.Errorf("Removed: got %d, want 5", resp.Removed)
	}
	if _, err := os.Stat(filepath.Join(root, "sub")); !os.IsNotExist(err) {
		t.Errorf("dir still exists after recursive delete: %v", err)
	}
	// Root file untouched.
	if _, err := os.Stat(filepath.Join(root, "a.php")); err != nil {
		t.Errorf("root file should still exist: %v", err)
	}
}

func TestHandleDeleteFileExecutionNonRecursiveDirFailsIfNotEmpty(t *testing.T) {
	root := t.TempDir()
	mkTree(t, root, map[string]string{"dir/child": "x"})

	resp := runDeleteHandler(t, msg.DeleteFileMessage{Path: filepath.Join(root, "dir"), Recursive: false})
	if resp.Success {
		t.Fatalf("non-recursive delete on non-empty dir should fail")
	}
	// Directory should still exist.
	if _, err := os.Stat(filepath.Join(root, "dir")); err != nil {
		t.Errorf("dir should still exist after failed delete: %v", err)
	}
}

func TestHandleDeleteFileExecutionNonRecursiveDirEmptySucceeds(t *testing.T) {
	root := t.TempDir()
	empty := filepath.Join(root, "empty")
	if err := os.MkdirAll(empty, 0755); err != nil {
		t.Fatal(err)
	}

	resp := runDeleteHandler(t, msg.DeleteFileMessage{Path: empty, Recursive: false})
	if !resp.Success {
		t.Fatalf("expected success: %s", resp.Error)
	}
	if resp.Removed != 1 {
		t.Errorf("Removed: got %d, want 1", resp.Removed)
	}
}

func TestHandleDeleteFileExecutionWorkdirRelative(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "rel.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	resp := runDeleteHandler(t, msg.DeleteFileMessage{Path: "rel.txt", Workdir: root})
	if !resp.Success {
		t.Fatalf("expected success: %s", resp.Error)
	}
	if resp.Removed != 1 {
		t.Errorf("Removed: got %d, want 1", resp.Removed)
	}
}

func TestDeleteFileResponseWireFormat(t *testing.T) {
	original := msg.DeleteFileResponse{Success: true, Removed: 42}
	encoded, err := msgpack.Marshal(&original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded msg.DeleteFileResponse
	if err := msgpack.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !decoded.Success || decoded.Removed != 42 {
		t.Errorf("roundtrip lost data: %+v", decoded)
	}
}

// TestHandleCopyToSpaceOverwritesReadOnly covers the .git/objects/pack/*.pack
// case: a pre-existing read-only destination file must be overwritable, and
// when the caller passes FilePerm the destination ends up with that mode
// (allowing readonly files to be re-mirrored without losing their readonly
// bit).
func TestHandleCopyToSpaceOverwritesReadOnly(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "readonly.pack")
	if err := os.WriteFile(target, []byte("original"), 0444); err != nil {
		t.Fatal(err)
	}

	client, server := net.Pipe()
	t.Cleanup(func() {
		client.Close()
		server.Close()
	})
	done := make(chan struct{})
	go func() {
		handleCopyFileExecution(server, msg.CopyFileMessage{
			Direction: "to_space",
			DestPath:  target,
			Content:   []byte("replaced"),
			FilePerm:  0444, // restore readonly
		})
		close(done)
	}()

	var resp msg.CopyFileResponse
	if err := msg.ReadMessage(client, &resp); err != nil {
		t.Fatalf("read response: %v", err)
	}
	<-done

	if !resp.Success {
		t.Fatalf("expected success, got error: %s", resp.Error)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read after overwrite: %v", err)
	}
	if string(got) != "replaced" {
		t.Errorf("content: got %q, want %q", got, "replaced")
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0444 {
		t.Errorf("perm after overwrite: got %o, want 0444 (restored by FilePerm)", info.Mode().Perm())
	}
}

// TestHandleCopyToSpaceOverwritesReadOnlyNoFilePerm confirms that without
// FilePerm the destination ends up user-writable (we had to chmod it to
// write) rather than crashing. This is the "overwrite a readonly file
// without specifying the desired final mode" case.
func TestHandleCopyToSpaceOverwritesReadOnlyNoFilePerm(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses permission bits — test only meaningful as non-root")
	}
	root := t.TempDir()
	target := filepath.Join(root, "readonly.txt")
	if err := os.WriteFile(target, []byte("original"), 0444); err != nil {
		t.Fatal(err)
	}

	client, server := net.Pipe()
	t.Cleanup(func() {
		client.Close()
		server.Close()
	})
	done := make(chan struct{})
	go func() {
		handleCopyFileExecution(server, msg.CopyFileMessage{
			Direction: "to_space",
			DestPath:  target,
			Content:   []byte("replaced"),
			// FilePerm intentionally unset
		})
		close(done)
	}()

	var resp msg.CopyFileResponse
	if err := msg.ReadMessage(client, &resp); err != nil {
		t.Fatalf("read response: %v", err)
	}
	<-done

	if !resp.Success {
		t.Fatalf("expected success, got error: %s", resp.Error)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	// File should now be writable (we added user-write to do the overwrite
	// and no FilePerm was set to restore a different mode).
	if info.Mode().Perm()&0200 == 0 {
		t.Errorf("perm %o should be user-writable after overwrite without FilePerm", info.Mode().Perm())
	}
}
