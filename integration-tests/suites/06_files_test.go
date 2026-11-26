//go:build integration

package suites

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

	// A command may run as long as its timeout, not a fixed few seconds.
	out = harness.RunCommand(t, user1.Client, id, 30, "sleep 7; echo finished")
	mustContain(t, "a command running 7 seconds", out, "finished")
	if _, err := harness.TryRunCommand(user1.Client, id, 2, "sleep 10"); err == nil {
		t.Fatal("a command running past its timeout did not fail")
	}
}

// knotCLI runs the knot command built for the suite as user1 against the
// default server, from a scratch folder so no knot.toml is picked up. It
// returns stdout and stderr separately, and the error from a failed run.
func knotCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(bins.Server, append(args, "--server", server.BaseURL, "--token", user1.Token)...)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "HOME="+cmd.Dir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// mustKnot runs the knot command and fails the test if it fails.
func mustKnot(t *testing.T, args ...string) (string, string) {
	t.Helper()
	stdout, stderr, err := knotCLI(t, args...)
	if err != nil {
		t.Fatalf("knot %s: %v\nstdout: %q\nstderr: %q", strings.Join(args, " "), err, stdout, stderr)
	}
	return stdout, stderr
}

// readTree returns every file and symlink under root, keyed by slash path:
// a file's content, or "-> target" for a symlink.
func readTree(t *testing.T, root string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if d.Type()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(p)
			tree[filepath.ToSlash(rel)] = "-> " + target
			return nil
		}
		content, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		tree[filepath.ToSlash(rel)] = string(content)
		return nil
	})
	return tree
}

// TestSpaceMirrorRoundTrip uploads a tree holding binary files with the
// real knot command, mirrors it back to another folder, and checks every
// byte, mtime and symlink survives both ways; then that reruns copy
// nothing, --verify agrees, deletes follow the source, the empty-source
// guard holds, and read-file returns binary files intact over JSON and
// msgpack.
func TestSpaceMirrorRoundTrip(t *testing.T) {
	harness.Feature(t, "files")
	id := workspace(t)
	ctx, cancel := testCtx(300)
	defer cancel()

	home := strings.TrimSpace(harness.RunCommand(t, user1.Client, id, 30, "pwd"))
	remote := "it-wh:" + home + "/it-mirror"

	// Every byte value, over several 4 MB pieces, plus small, nested,
	// hidden and empty files and a symlink.
	big := make([]byte, 10*1024*1024+123)
	for i := range big {
		big[i] = byte(i * 7)
	}
	src := t.TempDir()
	mtime := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	for rel, content := range map[string][]byte{
		"data/big.bin":       big,
		"index.html":         []byte("<h1>mirror</h1>"),
		"assets/img/dot.png": {0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0xff},
		".env":               []byte("SECRET=not-really\n"),
		"empty.txt":          {},
	} {
		p := filepath.Join(src, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0755)
		if err := os.WriteFile(p, content, 0640); err != nil {
			t.Fatal(err)
		}
		os.Chtimes(p, mtime, mtime)
	}
	if err := os.Symlink("index.html", filepath.Join(src, "home.html")); err != nil {
		t.Fatal(err)
	}
	want := readTree(t, src)

	// Upload, then download into a folder that doesn't exist yet and holds
	// nothing, and compare.
	_, stderr := mustKnot(t, "space", "mirror", src, remote)
	mustContain(t, "push", stderr, "6 uploaded") // five files and the symlink
	dst := filepath.Join(t.TempDir(), "copy")
	_, stderr = mustKnot(t, "space", "mirror", remote, dst)
	mustContain(t, "pull", stderr, "6 downloaded")
	got := readTree(t, dst)
	if len(got) != len(want) {
		t.Fatalf("pulled %d entries, want %d: %v", len(got), len(want), keys(got))
	}
	for rel, content := range want {
		if got[rel] != content {
			t.Errorf("%s came back %d bytes, want %d", rel, len(got[rel]), len(content))
		}
	}
	info, err := os.Stat(filepath.Join(dst, "data", "big.bin"))
	if err != nil || !info.ModTime().Equal(mtime) || info.Mode().Perm() != 0640 {
		t.Errorf("big.bin mtime %v perm %v (err %v), want %v 0640", info.ModTime(), info.Mode().Perm(), err, mtime)
	}

	// Nothing to copy on a rerun either way, and --verify agrees.
	_, stderr = mustKnot(t, "space", "mirror", remote, dst)
	mustContain(t, "second pull", stderr, "0 downloaded")
	_, stderr = mustKnot(t, "space", "mirror", src, remote)
	mustContain(t, "second push", stderr, "0 uploaded")
	_, stderr = mustKnot(t, "space", "mirror", "--verify", remote, dst)
	mustContain(t, "verify", stderr, "0 differ, 0 missing on remote, 0 missing on local")

	// A file changed and one removed in the space follow to the copy; a
	// local file the space doesn't have goes.
	if err := user1.Client.WriteSpaceFile(ctx, id, home+"/it-mirror/index.html", "<h1>changed</h1>"); err != nil {
		t.Fatal(err)
	}
	if _, err := user1.Client.DeleteSpaceFile(ctx, id, apiclient.DeleteFileRequest{Path: home + "/it-mirror/empty.txt"}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dst, "local-only.txt"), []byte("x"), 0644)
	mustKnot(t, "space", "mirror", remote, dst)
	got = readTree(t, dst)
	if got["index.html"] != "<h1>changed</h1>" {
		t.Errorf("index.html = %q after the space changed it", got["index.html"])
	}
	for _, gone := range []string{"empty.txt", "local-only.txt"} {
		if _, ok := got[gone]; ok {
			t.Errorf("%s should be deleted", gone)
		}
	}

	// Mirroring from a missing folder in the space refuses rather than
	// deleting the copy; so does mirroring an empty folder up.
	_, stderr, err = knotCLI(t, "space", "mirror", "it-wh:"+home+"/it-no-such-dir", dst)
	if err == nil || !strings.Contains(stderr, "empty or missing") {
		t.Errorf("pull from a missing folder: err %v, stderr %q, want a refusal", err, stderr)
	}
	if _, err := os.Stat(filepath.Join(dst, "data", "big.bin")); err != nil {
		t.Errorf("the refused pull touched the copy: %v", err)
	}
	_, stderr, err = knotCLI(t, "space", "mirror", t.TempDir(), remote)
	if err == nil || !strings.Contains(stderr, "is empty") {
		t.Errorf("push of an empty folder: err %v, stderr %q, want a refusal", err, stderr)
	}

	// read-file returns a binary file byte for byte: the knot command, the
	// JSON-speaking harness client and a msgpack client alike.
	stdout, _ := mustKnot(t, "space", "read-file", "it-wh", home+"/it-mirror/data/big.bin")
	if stdout != string(big) {
		t.Errorf("knot space read-file returned %d bytes, want %d", len(stdout), len(big))
	}
	msgpackClient, err := apiclient.NewClient(server.BaseURL, user1.Token, true)
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]*apiclient.ApiClient{"json": user1.Client, "msgpack": msgpackClient} {
		content, err := c.ReadSpaceFile(ctx, id, home+"/it-mirror/data/big.bin")
		if err != nil || content != string(big) {
			t.Errorf("%s ReadSpaceFile: %d bytes, err %v, want %d", name, len(content), err, len(big))
		}
	}
	// A line range is still text.
	stdout, _ = mustKnot(t, "space", "read-file", "--limit", "1", "it-wh", home+"/it-mirror/index.html")
	mustContain(t, "line range", stdout, "<h1>changed</h1>")
}

// TestSpaceMirrorFoldersAndWatch mirrors with the real knot command and
// agent: empty folders and folder modes go both ways, a symlink replaced by
// a file is replaced in the space rather than written through, no temporary
// upload is left behind, and --watch sends new files, new empty folders and
// deletes, then exits on Ctrl+C.
func TestSpaceMirrorFoldersAndWatch(t *testing.T) {
	harness.Feature(t, "files")
	id := workspace(t)
	ctx, cancel := testCtx(300)
	defer cancel()

	home := strings.TrimSpace(harness.RunCommand(t, user1.Client, id, 30, "pwd"))
	dir := home + "/it-mirror-dirs"
	remote := "it-wh:" + dir
	stat := func(rel string) string {
		t.Helper()
		return strings.TrimSpace(harness.RunCommand(t, user1.Client, id, 30, "sh", "-c", "stat -c '%a %F' "+dir+"/"+rel+" 2>/dev/null || echo missing"))
	}

	// A file the space's symlink points at, which the push must not touch.
	if err := user1.Client.WriteSpaceFile(ctx, id, dir+"-target.txt", "keep me"); err != nil {
		t.Fatal(err)
	}
	if err := user1.Client.CreateSymlinkSpaceFile(ctx, id, dir+"/conf", dir+"-target.txt"); err != nil {
		t.Fatal(err)
	}

	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "empty", "inner"), 0755)
	os.MkdirAll(filepath.Join(src, "private"), 0755)
	os.WriteFile(filepath.Join(src, "private", "key.txt"), []byte("k"), 0600)
	os.Chmod(filepath.Join(src, "private"), 0700)
	os.MkdirAll(filepath.Join(src, "sealed"), 0755)
	os.WriteFile(filepath.Join(src, "sealed", "in.txt"), []byte("sealed in"), 0644)
	os.Chmod(filepath.Join(src, "sealed"), 0555) // filled, then made read-only
	t.Cleanup(func() { os.Chmod(filepath.Join(src, "sealed"), 0755) })
	os.WriteFile(filepath.Join(src, "conf"), []byte("a real file"), 0644)
	big := bytes.Repeat([]byte("0123456789abcdef"), 600*1024) // ~9.4 MB: several pieces
	os.WriteFile(filepath.Join(src, "big.bin"), big, 0644)

	mustKnot(t, "space", "mirror", src, remote)
	mustEqual(t, "empty/inner", stat("empty/inner"), "755 directory")
	mustEqual(t, "private", stat("private"), "700 directory")
	mustEqual(t, "sealed", stat("sealed"), "555 directory")
	mustEqual(t, "sealed/in.txt", stat("sealed/in.txt"), "644 regular file")
	mustEqual(t, "conf", stat("conf"), "644 regular file")
	target, _ := user1.Client.ReadSpaceFile(ctx, id, dir+"-target.txt")
	mustEqual(t, "symlink target", target, "keep me")
	got, err := user1.Client.ReadSpaceFile(ctx, id, dir+"/big.bin")
	if err != nil || got != string(big) {
		t.Errorf("big.bin: %d bytes, err %v, want %d", len(got), err, len(big))
	}
	left, err := user1.Client.Find(ctx, id, apiclient.FindRequest{Path: dir, Recursive: true, IncludeHidden: true, Name: "*.knot-upload"})
	if err != nil || len(left.Paths) != 0 {
		t.Errorf("temporary uploads left: %v (err %v)", left.Paths, err)
	}
	_, stderr := mustKnot(t, "space", "mirror", src, remote)
	mustContain(t, "second push", stderr, "0 uploaded")

	// Back down: the empty folder and the folder's mode come too.
	dst := filepath.Join(t.TempDir(), "copy")
	t.Cleanup(func() { os.Chmod(filepath.Join(dst, "sealed"), 0755) })
	mustKnot(t, "space", "mirror", remote, dst)
	if b, err := os.ReadFile(filepath.Join(dst, "sealed", "in.txt")); err != nil || string(b) != "sealed in" {
		t.Errorf("pulled sealed/in.txt: %q %v", b, err)
	}
	for rel, want := range map[string]os.FileMode{"empty/inner": 0755, "private": 0700, "sealed": 0555} {
		info, err := os.Stat(filepath.Join(dst, filepath.FromSlash(rel)))
		if err != nil || !info.IsDir() || info.Mode().Perm() != want {
			t.Errorf("pulled %s: %v, want a folder of mode %o", rel, err, want)
		}
	}

	// A file made executable: its mode is set in the space.
	os.Chmod(filepath.Join(src, "conf"), 0755)
	_, stderr = mustKnot(t, "space", "mirror", src, remote)
	mustContain(t, "chmod push", stderr, "1 uploaded")
	mustEqual(t, "conf after chmod", stat("conf"), "755 regular file")

	// Later changes inside a folder already read-only in the space: the
	// agent opens it for them and gives it back its mode.
	sealed := filepath.Join(src, "sealed")
	os.Chmod(sealed, 0755)
	os.WriteFile(filepath.Join(sealed, "added.txt"), []byte("added"), 0644)
	os.Remove(filepath.Join(sealed, "in.txt"))
	os.Chmod(sealed, 0555)
	_, stderr = mustKnot(t, "space", "mirror", src, remote)
	mustContain(t, "push into a read-only folder", stderr, "0 failed")
	mustEqual(t, "sealed/added.txt", stat("sealed/added.txt"), "644 regular file")
	mustEqual(t, "sealed/in.txt", stat("sealed/in.txt"), "missing")
	mustEqual(t, "sealed after", stat("sealed"), "555 directory")
	// And the read-only folder removed altogether.
	os.Chmod(sealed, 0755)
	os.RemoveAll(sealed)
	_, stderr = mustKnot(t, "space", "mirror", src, remote)
	mustContain(t, "remove a read-only folder", stderr, "0 failed")
	mustEqual(t, "sealed removed", stat("sealed"), "missing")

	// --watch: a new file, a new empty folder and a delete reach the space.
	cmd := exec.Command(bins.Server, "space", "mirror", "--watch", src, remote, "--server", server.BaseURL, "--token", user1.Token)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "HOME="+cmd.Dir)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	defer cmd.Process.Kill()
	time.Sleep(3 * time.Second) // the initial mirror, then watching

	os.WriteFile(filepath.Join(src, "new.txt"), []byte("watched"), 0644)
	os.Mkdir(filepath.Join(src, "new-empty"), 0750)
	os.Remove(filepath.Join(src, "conf"))
	deadline := time.Now().Add(30 * time.Second)
	for {
		content, _ := user1.Client.ReadSpaceFile(ctx, id, dir+"/new.txt")
		if content == "watched" && stat("new-empty") == "750 directory" && stat("conf") == "missing" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("watch did not send the changes: new.txt %q, new-empty %s, conf %s\n%s", content, stat("new-empty"), stat("conf"), out.String())
		}
		time.Sleep(500 * time.Millisecond)
	}

	cmd.Process.Signal(os.Interrupt)
	select {
	case <-waited:
	case <-time.After(15 * time.Second):
		t.Fatalf("watch did not stop on Ctrl+C:\n%s", out.String())
	}
	harness.RunCommand(t, user1.Client, id, 30, "chmod", "-R", "u+w", dir)
	user1.Client.DeleteSpaceFile(ctx, id, apiclient.DeleteFileRequest{Path: dir, Recursive: true})
	user1.Client.DeleteSpaceFile(ctx, id, apiclient.DeleteFileRequest{Path: dir + "-target.txt"})
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
