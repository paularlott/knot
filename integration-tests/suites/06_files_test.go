//go:build integration

package suites

import (
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
)

func TestSpaceFiles(t *testing.T) {
	harness.Feature(t, "files")
	id := workspace(t)
	c := user1.Client
	ctx, cancel := testCtx(120)
	defer cancel()

	// Write + read.
	if err := c.WriteSpaceFile(ctx, id, "it-test.txt", "line one\nline two\nline three\n"); err != nil {
		t.Fatalf("write file: %v", err)
	}
	content, err := c.ReadSpaceFile(ctx, id, "it-test.txt")
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	mustEqual(t, "file content", content, "line one\nline two\nline three\n")

	// Range read starting at line 2: never includes line one or three.
	ranged, total, err := c.ReadSpaceFileRange(ctx, id, "it-test.txt", 2, 1)
	if err != nil {
		t.Fatalf("read range: %v", err)
	}
	mustContain(t, "range content", ranged, "line two")
	if contains(ranged, "line one") || contains(ranged, "line three") {
		t.Fatalf("range content leaked outside the window: %q", ranged)
	}
	mustEqual(t, "total lines", total, 3)

	// Grep.
	grep, err := c.Grep(ctx, id, apiclient.GrepRequest{Pattern: "two", Path: "it-test.txt"})
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if len(grep.Matches) != 1 || grep.Matches[0].Line != 2 {
		t.Fatalf("grep matches = %+v", grep.Matches)
	}

	// Find.
	err = c.WriteSpaceFile(ctx, id, "sub/other.txt", "other")
	if err != nil {
		t.Fatalf("write nested file: %v", err)
	}
	find, err := c.Find(ctx, id, apiclient.FindRequest{Path: ".", Recursive: true, Name: "other.txt"})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(find.Paths) != 1 || !strings.HasSuffix(find.Paths[0], "sub/other.txt") {
		t.Fatalf("find paths = %v", find.Paths)
	}

	// Sed replace.
	sed, err := c.Sed(ctx, id, apiclient.SedRequest{
		Mode: "replace", Pattern: "two", Replacement: "TWO", Path: "it-test.txt",
	})
	if err != nil {
		t.Fatalf("sed: %v", err)
	}
	if sed.FilesModified != 1 {
		t.Fatalf("sed files modified = %d", sed.FilesModified)
	}
	content, _ = c.ReadSpaceFile(ctx, id, "it-test.txt")
	mustEqual(t, "post-sed content", content, "line one\nline TWO\nline three\n")

	// EditFile (unique search/replace).
	if _, err := c.EditFile(ctx, id, apiclient.EditFileRequest{
		Path: "it-test.txt", Search: "line three", Replace: "line 3",
	}); err != nil {
		t.Fatalf("edit file: %v", err)
	}
	content, _ = c.ReadSpaceFile(ctx, id, "it-test.txt")
	mustEqual(t, "post-edit content", content, "line one\nline TWO\nline 3\n")

	// Symlink.
	if err := c.CreateSymlinkSpaceFile(ctx, id, "it-link.txt", "it-test.txt"); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	out := harness.RunCommand(t, c, id, 30, "readlink", "it-link.txt")
	mustEqual(t, "symlink target", strings.TrimSpace(out), "it-test.txt")

	// Delete.
	del, err := c.DeleteSpaceFile(ctx, id, apiclient.DeleteFileRequest{Path: "it-test.txt"})
	if err != nil || !del.Success {
		t.Fatalf("delete file: %v %+v", err, del)
	}
	if _, err := c.ReadSpaceFile(ctx, id, "it-test.txt"); err == nil {
		t.Fatal("deleted file still readable")
	}
}

func TestSpaceRunCommandBehaviour(t *testing.T) {
	harness.Feature(t, "run-command")
	id := workspace(t)

	// The agent joins command+args and runs everything through a shell, so
	// shell syntax goes in a single command string.
	out := harness.RunCommand(t, user1.Client, id, 30, "printf 'a\nb\nc\n' | grep -c b")
	mustEqual(t, "piped command", strings.TrimSpace(out), "1")

	out = harness.RunCommand(t, user1.Client, id, 30, "echo to-out; echo to-err 1>&2")
	mustContain(t, "combined output", out, "to-out")
	mustContain(t, "combined output", out, "to-err")

	out = harness.RunCommand(t, user1.Client, id, 30, "id -un; pwd")
	mustContain(t, "command user", out, "user1")
	mustContain(t, "command home", out, "/home/user1")

	if _, err := harness.TryRunCommand(user1.Client, id, 30, "exit 7"); err == nil {
		t.Fatal("failing command did not report an error")
	}
}
