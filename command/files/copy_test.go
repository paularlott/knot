package command_files

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/filestore"
)

func TestParseLocation(t *testing.T) {
	remote := map[string][2]string{
		"bucket1:abc.py":          {"bucket1", "abc.py"},
		"bucket1:":                {"bucket1", ""},
		"bucket1:dir/":            {"bucket1", "dir/"},
		"alice--docs:a/b/c.txt":   {"alice--docs", "a/b/c.txt"},
		"Grace.Hopper--x:k":       {"Grace.Hopper--x", "k"},
		"bucket1:/lead":           {"bucket1", "lead"},
		"abc:x":                   {"abc", "x"},
		"b-1.2:a:b":               {"b-1.2", "a:b"},
		"bucket1:dir/*.py":        {"bucket1", "dir/*.py"},
		"bucket1:a.b/c..d/.shown": {"bucket1", "a.b/c..d/.shown"},
	}
	for arg, want := range remote {
		l, err := parseLocation(arg)
		if err != nil || !l.remote || l.bucket != want[0] || l.key != want[1] {
			t.Errorf("parseLocation(%q) = %+v, %v; want bucket %q key %q", arg, l, err, want[0], want[1])
		}
	}

	for _, arg := range []string{
		"./a:b", "../a:b", "/abs/path", "relative/path", "rel/path:x", "file.txt", `C:\dir\file`, "C:/dir/file", "ab:foo", "a:b",
		"my_bucket:x", ":x", "-bucket:x", "bucket-:x", "with space:x", `dir\bucket1:x`, "~/x:y",
	} {
		l, err := parseLocation(arg)
		if err != nil || l.remote || l.stdio || l.raw != arg {
			t.Errorf("parseLocation(%q) = %+v, %v; want a local path", arg, l, err)
		}
	}

	if l, err := parseLocation("-"); err != nil || !l.stdio {
		t.Errorf("parseLocation(-) = %+v, %v", l, err)
	}
	if _, err := parseLocation(""); err == nil {
		t.Error("an empty path was accepted")
	}

	// Keys a URL would rewrite to something else are refused.
	for _, bad := range []string{"bucket1:a/../victim", "bucket1:./x", "bucket1:..", "bucket1:a//c", "bucket1://a", "bucket1:a/./b"} {
		if _, err := parseLocation(bad); err == nil {
			t.Errorf("parseLocation(%q) accepted", bad)
		}
	}
	// Dots and a trailing slash are fine.
	for _, good := range []string{"bucket1:a.b/c..d/.hidden", "bucket1:dir/", "bucket1:.dotfile"} {
		if _, err := parseLocation(good); err != nil {
			t.Errorf("parseLocation(%q): %v", good, err)
		}
	}

	if _, err := requireRemote("bucket1"); err == nil || !strings.Contains(err.Error(), "bucket1:") {
		t.Errorf("a plain bucket name should point at bucket:, got %v", err)
	}
	if _, err := requireRemote("./local"); err == nil {
		t.Error("a local path was accepted as a bucket path")
	}
}

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pattern, key string
		folders      bool
		rel          string
		ok           bool
	}{
		{"*.py", "a.py", false, "a.py", true},
		{"*.py", "a.txt", false, "", false},
		{"*.py", "dir/a.py", false, "", false}, // * stays within a folder
		{"src/*.py", "src/a.py", false, "a.py", true},
		{"src/*.py", "src/sub/a.py", false, "", false},
		{"src/*.py", "other/a.py", false, "", false},
		{"src/**/*.py", "src/a.py", false, "a.py", true},
		{"src/**/*.py", "src/sub/deeper/a.py", false, "sub/deeper/a.py", true},
		{"src/**/*.py", "src/sub/a.txt", false, "", false},
		{"**/*.py", "a.py", false, "a.py", true},
		{"**/*.py", "x/y/a.py", false, "x/y/a.py", true},
		{"a?c.txt", "abc.txt", false, "abc.txt", true},
		{"a?c.txt", "abbc.txt", false, "", false},
		{"[ab]*.txt", "b1.txt", false, "b1.txt", true},
		{"[ab]*.txt", "c1.txt", false, "", false},
		{"a[*]b", "a*b", false, "a*b", true}, // a literal * in brackets
		{"a[*]b", "axb", false, "", false},
		{"src/*", "src/a.py", false, "a.py", true},
		{"src/*", "src/sub/b.py", false, "", false},
		{"src/*", "src/sub/b.py", true, "sub/b.py", true}, // a matched folder is taken whole
		{"src/*", "src/sub/deep/c.py", true, "sub/deep/c.py", true},
		{"src/d*", "src/docs/x.md", true, "docs/x.md", true},
		{"src/d*", "src/other/x.md", true, "", false},
		{"*", "top.txt", false, "top.txt", true},
		{"*", "dir/file", true, "dir/file", true},
		{"[", "x", false, "", false}, // a malformed pattern matches nothing
	}
	for _, c := range cases {
		rel, ok := matchGlob(c.pattern, c.key, c.folders)
		if rel != c.rel || ok != c.ok {
			t.Errorf("matchGlob(%q, %q, %v) = %q, %v; want %q, %v", c.pattern, c.key, c.folders, rel, ok, c.rel, c.ok)
		}
	}

	roots := map[string]string{
		"*.py": "", "src/*.py": "src/", "a/b/c/*": "a/b/c/", "a/*/c": "a/", "a/b": "", "/abs/x/*.py": "/abs/x/", "**/x": "",
	}
	for pattern, want := range roots {
		if got, _ := globRoot(pattern); got != want {
			t.Errorf("globRoot(%q) = %q, want %q", pattern, got, want)
		}
	}

	if f, ok := globFolder("src/*", "src/sub/b.py"); !ok || f != "src/sub/" {
		t.Errorf("globFolder = %q, %v", f, ok)
	}
	if _, ok := globFolder("src/*", "src/b.py"); ok {
		t.Error("a file matched as a folder")
	}
}

// ---------------------------------------------------------------------------
// Copy, against an in-process server
// ---------------------------------------------------------------------------

func testBucket(t *testing.T, store *filestore.Store, name string) {
	t.Helper()
	p, _ := filestore.PrincipalFor(adminUser)
	if _, err := store.CreateBucket(p, "tester--"+name); err != nil {
		t.Fatal(err)
	}
}

func putRemote(t *testing.T, client *apiclient.ApiClient, bucket, key, content string) {
	t.Helper()
	if _, err := client.PutFileObject(context.Background(), bucket, key, strings.NewReader(content), int64(len(content)), "text/plain", time.Time{}); err != nil {
		t.Fatalf("put %s:%s: %v", bucket, key, err)
	}
}

func remoteContent(t *testing.T, client *apiclient.ApiClient, bucket, key string) (string, bool) {
	t.Helper()
	resp, err := client.GetFileObject(context.Background(), bucket, key)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b), true
}

func bucketKeys(t *testing.T, client *apiclient.ApiClient, bucket string) []string {
	t.Helper()
	objs, _, err := listAll(context.Background(), client, bucket, "", "")
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	for _, o := range objs {
		keys = append(keys, o.Key)
	}
	sort.Strings(keys)
	return keys
}

func localFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(out)
	return out
}

func expectKeys(t *testing.T, got []string, want ...string) {
	t.Helper()
	if want == nil {
		want = []string{}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func capture(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	err := fn()
	w.Close()
	os.Stdout = old
	return <-done, err
}

// copyTo runs a copy, failing the test unless it succeeds.
func mustCopy(t *testing.T, client *apiclient.ApiClient, recursive bool, args ...string) {
	t.Helper()
	if err := runCopy(context.Background(), client, args, recursive); err != nil {
		t.Fatalf("copy %v: %v", args, err)
	}
}

func copyFails(t *testing.T, client *apiclient.ApiClient, recursive bool, wantErr string, args ...string) {
	t.Helper()
	err := runCopy(context.Background(), client, args, recursive)
	if err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Errorf("copy %v: error %v, want one containing %q", args, err, wantErr)
	}
}

func TestCopyUpload(t *testing.T) {
	client, store := newFilesServer(t)
	_ = store
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "abc.py"), "print(1)")
	writeFile(t, filepath.Join(dir, "b.py"), "print(2)")
	writeFile(t, filepath.Join(dir, "notes.txt"), "notes")
	writeFile(t, filepath.Join(dir, "tree", "x.txt"), "x")
	writeFile(t, filepath.Join(dir, "tree", "deep", "y.txt"), "y")
	old := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	os.Chtimes(filepath.Join(dir, "abc.py"), old, old)
	abc := filepath.Join(dir, "abc.py")

	// To a named file, keeping the modification time and content type.
	mustCopy(t, client, false, abc, "sync-test:renamed.py")
	if c, _ := remoteContent(t, client, "sync-test", "renamed.py"); c != "print(1)" {
		t.Errorf("uploaded %q", c)
	}
	list, _, _ := listAll(context.Background(), client, "sync-test", "renamed.py", "")
	if len(list) != 1 || !list[0].ModifiedAt.Equal(old) || list[0].ContentType == "" {
		t.Errorf("uploaded file info %+v", list)
	}

	// To a folder, the file keeps its name; and to the bucket's root.
	mustCopy(t, client, false, abc, "sync-test:dir/")
	mustCopy(t, client, false, abc, "sync-test:")
	// Without a trailing slash the destination names a file, even if it
	// looks like a folder.
	mustCopy(t, client, false, abc, "sync-test:file-not-folder")
	expectKeys(t, bucketKeys(t, client, "sync-test"), "abc.py", "dir/abc.py", "file-not-folder", "renamed.py")

	// Several sources need a folder; without the slash it is refused and
	// nothing is copied.
	copyFails(t, client, false, "must be a folder", abc, filepath.Join(dir, "b.py"), "sync-test:many")
	if _, ok := remoteContent(t, client, "sync-test", "many"); ok {
		t.Error("a refused copy wrote a file")
	}
	mustCopy(t, client, false, abc, filepath.Join(dir, "b.py"), "sync-test:many/")
	expectKeys(t, bucketKeys(t, client, "sync-test"), "abc.py", "dir/abc.py", "file-not-folder", "many/abc.py", "many/b.py", "renamed.py")

	// A directory needs -r, and its contents go into the folder.
	copyFails(t, client, false, "use -r", filepath.Join(dir, "tree"), "sync-test:t/")
	copyFails(t, client, true, "must be a folder", filepath.Join(dir, "tree"), "sync-test:t")
	mustCopy(t, client, true, filepath.Join(dir, "tree"), "sync-test:t/")
	if c, _ := remoteContent(t, client, "sync-test", "t/deep/y.txt"); c != "y" {
		t.Errorf("recursive upload: %q", c)
	}
	if _, ok := remoteContent(t, client, "sync-test", "t/tree/x.txt"); ok {
		t.Error("the directory was nested inside the destination; its contents should go in")
	}
	if c, _ := remoteContent(t, client, "sync-test", "t/x.txt"); c != "x" {
		t.Errorf("recursive upload: %q", c)
	}

	// -r to the bucket root.
	mustCopy(t, client, true, filepath.Join(dir, "tree"), "sync-test:")
	if c, _ := remoteContent(t, client, "sync-test", "deep/y.txt"); c != "y" {
		t.Errorf("recursive upload to the root: %q", c)
	}

	// A wildcard in the argument, as when quoted or on a shell without globbing.
	mustCopy(t, client, false, filepath.Join(dir, "*.py"), "sync-test:py/")
	expectKeysWithPrefix(t, client, "py/", "py/abc.py", "py/b.py")
	copyFails(t, client, false, "no files match", filepath.Join(dir, "*.zzz"), "sync-test:z/")
	copyFails(t, client, false, "must be a folder", filepath.Join(dir, "*.py"), "sync-test:one-name")
	mustCopy(t, client, false, filepath.Join(dir, "**", "*.txt"), "sync-test:txt/")
	// Structure below the pattern's fixed part is kept.
	expectKeysWithPrefix(t, client, "txt/", "txt/notes.txt", "txt/tree/deep/y.txt", "txt/tree/x.txt")

	// Copying over an existing file replaces it.
	writeFile(t, abc, "print('v2')")
	mustCopy(t, client, false, abc, "sync-test:renamed.py")
	if c, _ := remoteContent(t, client, "sync-test", "renamed.py"); c != "print('v2')" {
		t.Errorf("not replaced: %q", c)
	}

	// Mistakes.
	copyFails(t, client, false, "no such file", filepath.Join(dir, "missing.txt"), "sync-test:x")
	copyFails(t, client, false, "both local", abc, filepath.Join(dir, "elsewhere.py"))
	copyFails(t, client, false, "both local", abc, "./out/")
	copyFails(t, client, false, "bucket not found", abc, "nobucket:x")
}

func expectKeysWithPrefix(t *testing.T, client *apiclient.ApiClient, prefix string, want ...string) {
	t.Helper()
	var got []string
	for _, k := range bucketKeys(t, client, "sync-test") {
		if strings.HasPrefix(k, prefix) {
			got = append(got, k)
		}
	}
	expectKeys(t, got, want...)
}

func TestCopyUploadDuplicateNames(t *testing.T) {
	client, _ := newFilesServer(t)
	a, b := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(a, "same.txt"), "from a")
	writeFile(t, filepath.Join(b, "same.txt"), "from b")
	copyFails(t, client, false, "would both be copied", filepath.Join(a, "same.txt"), filepath.Join(b, "same.txt"), "sync-test:dup/")
	expectKeys(t, bucketKeys(t, client, "sync-test")) // nothing was copied
}

func TestCopyDownload(t *testing.T) {
	client, _ := newFilesServer(t)
	putRemote(t, client, "sync-test", "path/abc.py", "abc")
	putRemote(t, client, "sync-test", "path/b.py", "b")
	putRemote(t, client, "sync-test", "path/sub/c.py", "c")
	putRemote(t, client, "sync-test", "path/sub/d.txt", "d")
	putRemote(t, client, "sync-test", "top.txt", "top")
	// An empty folder is a zero-byte object ending in /.
	putRemote(t, client, "sync-test", "path/empty/", "")
	out := t.TempDir()

	// To a named file.
	mustCopy(t, client, false, "sync-test:path/abc.py", filepath.Join(out, "test.py"))
	if b, _ := os.ReadFile(filepath.Join(out, "test.py")); string(b) != "abc" {
		t.Errorf("downloaded %q", b)
	}

	// Into an existing directory the file keeps its name; a path ending in /
	// is made.
	mustCopy(t, client, false, "sync-test:path/b.py", out)
	mustCopy(t, client, false, "sync-test:path/b.py", filepath.Join(out, "newdir")+"/")
	expectKeys(t, localFiles(t, out), "b.py", "newdir/b.py", "test.py")

	// Several files, and a missing directory is made.
	multi := filepath.Join(out, "multi")
	mustCopy(t, client, false, "sync-test:path/abc.py", "sync-test:top.txt", multi)
	expectKeys(t, localFiles(t, multi), "abc.py", "top.txt")
	copyFails(t, client, false, "exists and is not a directory", "sync-test:path/abc.py", "sync-test:top.txt", filepath.Join(out, "test.py"))

	// A folder needs -r; its contents go into the directory, nested keys
	// keep their structure, and an empty folder becomes an empty directory.
	rec := filepath.Join(out, "rec")
	copyFails(t, client, false, "use -r", "sync-test:path", rec)
	copyFails(t, client, false, "use -r", "sync-test:path/", rec)
	mustCopy(t, client, true, "sync-test:path", rec)
	expectKeys(t, localFiles(t, rec), "abc.py", "b.py", "sub/c.py", "sub/d.txt")
	if info, err := os.Stat(filepath.Join(rec, "empty")); err != nil || !info.IsDir() {
		t.Errorf("empty folder not made as a directory: %v", err)
	}
	// The same with the trailing slash, and the whole bucket.
	rec2 := filepath.Join(out, "rec2")
	mustCopy(t, client, true, "sync-test:path/", rec2)
	expectKeys(t, localFiles(t, rec2), "abc.py", "b.py", "sub/c.py", "sub/d.txt")
	all := filepath.Join(out, "all")
	copyFails(t, client, false, "use -r", "sync-test:", all)
	mustCopy(t, client, true, "sync-test:", all)
	expectKeys(t, localFiles(t, all), "path/abc.py", "path/b.py", "path/sub/c.py", "path/sub/d.txt", "top.txt")

	// Wildcards, quoted so they reach us.
	g := filepath.Join(out, "glob")
	mustCopy(t, client, false, "sync-test:path/*.py", g)
	expectKeys(t, localFiles(t, g), "abc.py", "b.py")
	g2 := filepath.Join(out, "glob2")
	mustCopy(t, client, false, "sync-test:path/**/*.py", g2)
	expectKeys(t, localFiles(t, g2), "abc.py", "b.py", "sub/c.py")
	g3 := filepath.Join(out, "glob3")
	mustCopy(t, client, true, "sync-test:path/*", g3)
	expectKeys(t, localFiles(t, g3), "abc.py", "b.py", "sub/c.py", "sub/d.txt")
	g4 := filepath.Join(out, "glob4")
	mustCopy(t, client, false, "sync-test:path/*", g4)
	expectKeys(t, localFiles(t, g4), "abc.py", "b.py") // without -r the folder is not taken
	copyFails(t, client, false, "no files match", "sync-test:path/*.zzz", g)

	// Mistakes.
	copyFails(t, client, false, "no such file or folder", "sync-test:path/missing.py", out)
	copyFails(t, client, false, "bucket not found", "nobucket:x", out)
	copyFails(t, client, false, "no such file or folder", "sync-test:pat", out) // a prefix is not a folder

	// A file is not replaced by a failed download.
	if b, _ := os.ReadFile(filepath.Join(out, "test.py")); string(b) != "abc" {
		t.Errorf("existing file damaged: %q", b)
	}

	// Never outside the destination: the server's keys can be anything.
	if _, err := within(out, "../x"); err == nil {
		t.Error("within allowed a path outside")
	}
}

func TestCopyBetweenBuckets(t *testing.T) {
	client, store := newFilesServer(t)
	testBucket(t, store, "other")
	old := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if _, err := client.PutFileObject(context.Background(), "sync-test", "path/abc.py", strings.NewReader("abc"), 3, "text/x-python", old); err != nil {
		t.Fatal(err)
	}
	putRemote(t, client, "sync-test", "path/b.py", "b")
	putRemote(t, client, "sync-test", "path/sub/c.py", "c")
	putRemote(t, client, "sync-test", "path/sub/d.txt", "d")
	putRemote(t, client, "sync-test", "path/empty/", "")

	// A file to a name, with its type and time.
	mustCopy(t, client, false, "sync-test:path/abc.py", "other:path2/abc.py")
	if c, _ := remoteContent(t, client, "other", "path2/abc.py"); c != "abc" {
		t.Errorf("copied %q", c)
	}
	list, _, _ := listAll(context.Background(), client, "other", "path2/", "")
	if len(list) != 1 || list[0].ContentType != "text/x-python" || !list[0].ModifiedAt.Equal(old) {
		t.Errorf("copy lost the type or modification time: %+v", list)
	}

	// Into a folder; by full name; the same bucket.
	mustCopy(t, client, false, "sync-test:path/b.py", "tester--other:in/")
	mustCopy(t, client, false, "sync-test:path/b.py", "sync-test:copy-of-b.py")
	expectKeys(t, bucketKeys(t, client, "other"), "in/b.py", "path2/abc.py")

	// Recursive: contents go into the folder, markers come across.
	mustCopy(t, client, true, "sync-test:path", "other:tree/")
	expectKeys(t, bucketKeys(t, client, "other"),
		"in/b.py", "path2/abc.py", "tree/abc.py", "tree/b.py", "tree/empty/", "tree/sub/c.py", "tree/sub/d.txt")

	// Wildcards.
	mustCopy(t, client, false, "sync-test:path/**/*.py", "other:py/")
	expectKeysWithPrefixIn(t, client, "other", "py/", "py/abc.py", "py/b.py", "py/sub/c.py")

	// The whole bucket into another.
	mustCopy(t, client, true, "sync-test:", "other:backup/")
	if c, _ := remoteContent(t, client, "other", "backup/path/sub/d.txt"); c != "d" {
		t.Errorf("whole bucket copy: %q", c)
	}

	// Copying a file onto itself is refused, in either spelling.
	copyFails(t, client, false, "same file", "sync-test:path/b.py", "sync-test:path/b.py")
	copyFails(t, client, false, "same file", "sync-test:path/b.py", "tester--sync-test:path/b.py")

	// Mixed sources: a local file and a bucket file into a bucket folder.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "local.txt"), "local")
	mustCopy(t, client, false, filepath.Join(dir, "local.txt"), "sync-test:path/b.py", "other:mixed/")
	expectKeysWithPrefixIn(t, client, "other", "mixed/", "mixed/b.py", "mixed/local.txt")

	// Mistakes.
	copyFails(t, client, false, "no such file or folder", "sync-test:nothing", "other:x")
	copyFails(t, client, false, "use -r", "sync-test:path", "other:x/")
	copyFails(t, client, false, "bucket not found", "sync-test:path/b.py", "nobucket:x")

	// The copy survives the source going.
	if err := client.DeleteFileObject(context.Background(), "sync-test", "path/abc.py"); err != nil {
		t.Fatal(err)
	}
	if c, _ := remoteContent(t, client, "other", "path2/abc.py"); c != "abc" {
		t.Errorf("copy lost with its source: %q", c)
	}
}

func expectKeysWithPrefixIn(t *testing.T, client *apiclient.ApiClient, bucket, prefix string, want ...string) {
	t.Helper()
	var got []string
	for _, k := range bucketKeys(t, client, bucket) {
		if strings.HasPrefix(k, prefix) {
			got = append(got, k)
		}
	}
	expectKeys(t, got, want...)
}

func TestCopyStdio(t *testing.T) {
	client, _ := newFilesServer(t)
	putRemote(t, client, "sync-test", "a.txt", "alpha")
	putRemote(t, client, "sync-test", "b.txt", "beta")

	// To stdout: just the bytes.
	out, err := capture(t, func() error { return runCopy(context.Background(), client, []string{"sync-test:a.txt", "-"}, false) })
	if err != nil || out != "alpha" {
		t.Errorf("to stdout: %q, %v", out, err)
	}
	copyFails(t, client, false, "single", "sync-test:*.txt", "-")
	copyFails(t, client, false, "single", "sync-test:a.txt", "sync-test:b.txt", "-")
	copyFails(t, client, true, "-r cannot be used with stdout", "sync-test:a.txt", "-")
	copyFails(t, client, false, "single bucket file", "./local.txt", "-")
	copyFails(t, client, false, "both the source and the destination", "-", "-")

	// From stdin.
	r, w, _ := os.Pipe()
	old := os.Stdin
	os.Stdin = r
	go func() {
		w.WriteString("from stdin")
		w.Close()
	}()
	err = runCopy(context.Background(), client, []string{"-", "sync-test:dir/stdin.txt"}, false)
	os.Stdin = old
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := remoteContent(t, client, "sync-test", "dir/stdin.txt"); c != "from stdin" {
		t.Errorf("from stdin: %q", c)
	}
	copyFails(t, client, true, "-r cannot be used with stdin", "-", "sync-test:x")
	copyFails(t, client, false, "file name is required", "-", "sync-test:dir/")
	copyFails(t, client, false, "file name is required", "-", "sync-test:")
	copyFails(t, client, false, "can only be copied to a bucket", "-", "./local")
	copyFails(t, client, false, "can only be the one source", "-", "sync-test:a.txt", "sync-test:x/")
}

func TestCopyNeedsTwoArguments(t *testing.T) {
	client, _ := newFilesServer(t)
	copyFails(t, client, false, "needs a source and a destination", "only-one")
	copyFails(t, client, false, "needs a source and a destination")
}

func TestListRemote(t *testing.T) {
	client, _ := newFilesServer(t)
	ctx := context.Background()
	for _, k := range []string{"top.txt", "app/a.txt", "app/b.py", "app/sub/c.py", "app/sub/deep/d.py", "apple.txt", "logs/1.log", "logs/2.log"} {
		putRemote(t, client, "sync-test", k, "x")
	}
	putRemote(t, client, "sync-test", "app/empty/", "")

	list := func(arg string, recursive bool) (objects, prefixes []string, err error) {
		loc, err := requireRemote(arg)
		if err != nil {
			return nil, nil, err
		}
		l, err := listRemote(ctx, client, loc, recursive)
		if err != nil {
			return nil, nil, err
		}
		for _, o := range l.Objects {
			objects = append(objects, o.Key)
		}
		return objects, l.Prefixes, nil
	}
	check := func(arg string, recursive bool, wantObjects, wantPrefixes []string) {
		t.Helper()
		o, p, err := list(arg, recursive)
		if err != nil {
			t.Errorf("ls %q: %v", arg, err)
			return
		}
		sort.Strings(o)
		sort.Strings(p)
		if !reflect.DeepEqual(append([]string{}, o...), append([]string{}, wantObjects...)) || !reflect.DeepEqual(append([]string{}, p...), append([]string{}, wantPrefixes...)) {
			t.Errorf("ls %q -r=%v: objects %v prefixes %v, want %v %v", arg, recursive, o, p, wantObjects, wantPrefixes)
		}
	}

	check("sync-test:", false, []string{"apple.txt", "top.txt"}, []string{"app/", "logs/"})
	check("sync-test:app", false, []string{"app/a.txt", "app/b.py"}, []string{"app/empty/", "app/sub/"}) // a folder, not the prefix "app"
	check("sync-test:app/", false, []string{"app/a.txt", "app/b.py"}, []string{"app/empty/", "app/sub/"})
	check("sync-test:app", true, []string{"app/a.txt", "app/b.py", "app/empty/", "app/sub/c.py", "app/sub/deep/d.py"}, nil)
	check("sync-test:top.txt", false, []string{"top.txt"}, nil) // a file lists as itself
	check("sync-test:app/*.py", false, []string{"app/b.py"}, nil)
	check("sync-test:app/**/*.py", false, []string{"app/b.py", "app/sub/c.py", "app/sub/deep/d.py"}, nil)
	check("sync-test:logs/*.log", false, []string{"logs/1.log", "logs/2.log"}, nil)
	check("sync-test:app/s*", false, nil, []string{"app/sub/"})
	check("sync-test:app/s*", true, []string{"app/sub/c.py", "app/sub/deep/d.py"}, nil)

	for _, bad := range []string{"sync-test:nothing", "sync-test:ap", "sync-test:*.zzz"} {
		if _, _, err := list(bad, false); err == nil {
			t.Errorf("ls %q found something", bad)
		}
	}
	if _, _, err := list("nobucket:", false); err == nil || !strings.Contains(err.Error(), "bucket not found") {
		t.Errorf("ls of a missing bucket: %v", err)
	}
}

func TestRemoveSources(t *testing.T) {
	client, _ := newFilesServer(t)
	ctx := context.Background()
	fill := func() {
		for _, k := range []string{"top.txt", "app/a.txt", "app/b.py", "app/sub/c.py", "logs/1.log", "logs/2.log", "logs/keep.txt"} {
			putRemote(t, client, "sync-test", k, "x")
		}
		putRemote(t, client, "sync-test", "app/", "") // the folder's own marker
	}
	rm := func(recursive bool, args ...string) error {
		var files []source
		for _, a := range args {
			loc, err := requireRemote(a)
			if err != nil {
				return err
			}
			list, _, err := remoteSources(ctx, client, loc, recursive, true)
			if err != nil {
				return err
			}
			files = append(files, list...)
		}
		for _, f := range files {
			if err := client.DeleteFileObject(ctx, f.bucket, f.key); err != nil {
				return err
			}
		}
		return nil
	}

	fill()
	// A file; a folder needs -r; a missing file is an error.
	if err := rm(false, "sync-test:top.txt"); err != nil {
		t.Fatal(err)
	}
	if err := rm(false, "sync-test:app"); err == nil || !strings.Contains(err.Error(), "use -r") {
		t.Errorf("rm of a folder without -r: %v", err)
	}
	if err := rm(false, "sync-test:"); err == nil || !strings.Contains(err.Error(), "use -r") {
		t.Errorf("rm of a bucket without -r: %v", err)
	}
	if err := rm(false, "sync-test:nothing"); err == nil {
		t.Error("rm of a missing file succeeded")
	}
	expectKeys(t, bucketKeys(t, client, "sync-test"), "app/", "app/a.txt", "app/b.py", "app/sub/c.py", "logs/1.log", "logs/2.log", "logs/keep.txt")

	// A wildcard; one that matches nothing deletes nothing, even among
	// several arguments.
	if err := rm(false, "sync-test:logs/*.log"); err != nil {
		t.Fatal(err)
	}
	expectKeys(t, bucketKeys(t, client, "sync-test"), "app/", "app/a.txt", "app/b.py", "app/sub/c.py", "logs/keep.txt")
	if err := rm(false, "sync-test:logs/keep.txt", "sync-test:*.zzz"); err == nil {
		t.Error("rm with an unmatched pattern succeeded")
	}
	if _, ok := remoteContent(t, client, "sync-test", "logs/keep.txt"); !ok {
		t.Error("a file was deleted though another argument matched nothing")
	}

	// -r takes a folder whole, with its own marker.
	if err := rm(true, "sync-test:app"); err != nil {
		t.Fatal(err)
	}
	expectKeys(t, bucketKeys(t, client, "sync-test"), "logs/keep.txt")

	// -r on the bucket empties it.
	fill()
	if err := rm(true, "sync-test:"); err != nil {
		t.Fatal(err)
	}
	expectKeys(t, bucketKeys(t, client, "sync-test"))
}

func TestCatSources(t *testing.T) {
	client, _ := newFilesServer(t)
	putRemote(t, client, "sync-test", "a.txt", "alpha ")
	putRemote(t, client, "sync-test", "b.txt", "beta")
	putRemote(t, client, "sync-test", "dir/c.txt", "gamma")

	cat := func(args ...string) (string, error) {
		return capture(t, func() error {
			for _, a := range args {
				loc, err := requireRemote(a)
				if err != nil {
					return err
				}
				files, _, err := remoteSources(context.Background(), client, loc, false, false)
				if err != nil {
					return err
				}
				for _, f := range files {
					if err := downloadFile(context.Background(), client, f.bucket, f.key, "-"); err != nil {
						return err
					}
				}
			}
			return nil
		})
	}
	if out, err := cat("sync-test:a.txt", "sync-test:b.txt"); err != nil || out != "alpha beta" {
		t.Errorf("cat: %q %v", out, err)
	}
	if out, err := cat("sync-test:*.txt"); err != nil || out != "alpha beta" {
		t.Errorf("cat with a wildcard: %q %v", out, err)
	}
	if _, err := cat("sync-test:dir"); err == nil || !strings.Contains(err.Error(), "folder") {
		t.Errorf("cat of a folder: %v", err)
	}
	if _, err := cat("sync-test:nothing"); err == nil {
		t.Error("cat of a missing file succeeded")
	}
}

func TestStatRemote(t *testing.T) {
	client, _ := newFilesServer(t)
	ctx := context.Background()
	putRemote(t, client, "sync-test", "file", "x")
	putRemote(t, client, "sync-test", "both", "x")
	putRemote(t, client, "sync-test", "both/inside", "y")
	putRemote(t, client, "sync-test", "folder/inside", "y")
	putRemote(t, client, "sync-test", "afile", "x")

	cases := []struct {
		key          string
		file, folder bool
	}{
		{"file", true, false}, {"both", true, true}, {"folder", false, true}, {"fol", false, false}, {"af", false, false},
		{"nothing", false, false}, {"folder/", false, true}, {"folder/inside", true, false}, {"", false, true}, {"nothing/", false, false},
	}
	for _, c := range cases {
		info, err := statRemote(ctx, client, "sync-test", c.key)
		if err != nil {
			t.Errorf("stat %q: %v", c.key, err)
			continue
		}
		if (info.file != nil) != c.file || info.folder != c.folder {
			t.Errorf("stat %q: file %v folder %v, want %v %v", c.key, info.file != nil, info.folder, c.file, c.folder)
		}
	}
	if _, err := statRemote(ctx, client, "nobucket", "x"); err == nil {
		t.Error("stat in a missing bucket succeeded")
	}

	// A file and a folder of one name: recursing takes the folder, without
	// -r the file.
	loc, _ := parseLocation("sync-test:both")
	list, folder, err := remoteSources(ctx, client, loc, false, false)
	if err != nil || folder || len(list) != 1 || list[0].key != "both" {
		t.Errorf("both, no -r: %v %v %v", list, folder, err)
	}
	list, folder, err = remoteSources(ctx, client, loc, true, false)
	if err != nil || !folder || len(list) != 1 || list[0].key != "both/inside" {
		t.Errorf("both, -r: %v %v %v", list, folder, err)
	}
}

func TestRunSyncDirections(t *testing.T) {
	client, store := newFilesServer(t)
	testBucket(t, store, "other")
	ctx := context.Background()
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.txt"), "alpha")
	writeFile(t, filepath.Join(src, "sub", "b.txt"), "beta")

	// Up, to a folder and to the root of a bucket.
	if err := runSync(ctx, client, src, "sync-test:site", false, false); err != nil {
		t.Fatal(err)
	}
	expectKeys(t, bucketKeys(t, client, "sync-test"), "site/a.txt", "site/sub/b.txt")
	if err := runSync(ctx, client, src, "other:", false, false); err != nil {
		t.Fatal(err)
	}
	expectKeys(t, bucketKeys(t, client, "other"), "a.txt", "sub/b.txt")

	// Down.
	dst := filepath.Join(t.TempDir(), "out")
	if err := runSync(ctx, client, "sync-test:site/", dst, false, false); err != nil {
		t.Fatal(err)
	}
	expectKeys(t, localFiles(t, dst), "a.txt", "sub/b.txt")

	// Between buckets: only what differs is copied.
	putRemote(t, client, "sync-test", "site/extra.txt", "extra")
	if err := runSync(ctx, client, "sync-test:site", "other:copy", false, false); err != nil {
		t.Fatal(err)
	}
	expectKeysWithPrefixIn(t, client, "other", "copy/", "copy/a.txt", "copy/extra.txt", "copy/sub/b.txt")
	admin := &filestore.Principal{IsAdmin: true}
	before, err := store.HeadObject(admin, "tester--other", "copy/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	putRemote(t, client, "sync-test", "site/a.txt", "alpha v2")
	if err := runSync(ctx, client, "sync-test:site", "other:copy", false, false); err != nil {
		t.Fatal(err)
	}
	if c, _ := remoteContent(t, client, "other", "copy/a.txt"); c != "alpha v2" {
		t.Errorf("changed file not copied: %q", c)
	}
	subBefore, _ := store.HeadObject(admin, "tester--other", "copy/sub/b.txt")
	if err := runSync(ctx, client, "sync-test:site", "other:copy", false, false); err != nil {
		t.Fatal(err)
	}
	if again, _ := store.HeadObject(admin, "tester--other", "copy/sub/b.txt"); again.UpdatedAt != subBefore.UpdatedAt {
		t.Error("unchanged file copied again")
	}
	_ = before

	// --delete between buckets, and a dry run changes nothing.
	putRemote(t, client, "other", "copy/stale.txt", "stale")
	if err := runSync(ctx, client, "sync-test:site", "other:copy", true, true); err != nil {
		t.Fatal(err)
	}
	if _, ok := remoteContent(t, client, "other", "copy/stale.txt"); !ok {
		t.Error("dry run deleted a file")
	}
	if err := runSync(ctx, client, "sync-test:site", "other:copy", true, false); err != nil {
		t.Fatal(err)
	}
	expectKeysWithPrefixIn(t, client, "other", "copy/", "copy/a.txt", "copy/extra.txt", "copy/sub/b.txt")

	// Overlapping folders in one bucket are refused, in either spelling.
	for _, pair := range [][2]string{
		{"sync-test:site", "sync-test:site"}, {"sync-test:site", "sync-test:site/sub"}, {"sync-test:site/sub", "sync-test:site"},
		{"sync-test:site", "tester--sync-test:site/x"}, {"sync-test:", "sync-test:new"}, {"sync-test:new", "sync-test:"},
	} {
		if err := runSync(ctx, client, pair[0], pair[1], true, false); err == nil || !strings.Contains(err.Error(), "overlap") {
			t.Errorf("sync %v: %v", pair, err)
		}
	}
	expectKeysWithPrefixIn(t, client, "sync-test", "site/", "site/a.txt", "site/extra.txt", "site/sub/b.txt")
	// Neighbours with a shared name prefix do not overlap.
	if err := runSync(ctx, client, "sync-test:site", "sync-test:site2", false, false); err != nil {
		t.Errorf("sync to a folder whose name starts the same: %v", err)
	}

	// Mistakes.
	for _, c := range []struct{ src, dst, want string }{
		{src, t.TempDir(), "both local"},
		{"-", "sync-test:x", "not stdin"},
		{"sync-test:x", "-", "not stdin"},
		{"sync-test:*.txt", dst, "not a pattern"},
		{src, "sync-test:*.txt", "not a pattern"},
		{"nobucket:x", dst, "bucket not found"},
		{src, "nobucket:x", "bucket not found"},
		{"sync-test:a", "nobucket:x", "bucket not found"},
	} {
		if err := runSync(ctx, client, c.src, c.dst, false, false); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("sync %s %s: %v, want %q", c.src, c.dst, err, c.want)
		}
	}
}

// Empty folders in a bucket become directories on download, and are left
// alone by an upload sync with --delete.
func TestSyncFolderMarkers(t *testing.T) {
	client, _ := newFilesServer(t)
	ctx := context.Background()
	putRemote(t, client, "sync-test", "f/a.txt", "a")
	putRemote(t, client, "sync-test", "f/empty/", "")
	putRemote(t, client, "sync-test", "f/", "")

	dst := filepath.Join(t.TempDir(), "out")
	if err := runSync(ctx, client, "sync-test:f", dst, true, false); err != nil {
		t.Fatalf("sync down with markers: %v", err)
	}
	expectKeys(t, localFiles(t, dst), "a.txt")
	if info, err := os.Stat(filepath.Join(dst, "empty")); err != nil || !info.IsDir() {
		t.Errorf("empty folder not a directory: %v", err)
	}

	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.txt"), "a")
	if err := runSync(ctx, client, src, "sync-test:f", true, false); err != nil {
		t.Fatal(err)
	}
	expectKeys(t, bucketKeys(t, client, "sync-test"), "f/", "f/a.txt", "f/empty/")
}
