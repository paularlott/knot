package commands_admin

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/filestore"
)

var owner = &filestore.Principal{UserId: "u1", Username: "alice", CanOwn: true}

func openStore(t *testing.T, dir string) *filestore.Store {
	t.Helper()
	s, err := filestore.Open(filestore.Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestExportImportFiles(t *testing.T) {
	srcDir := t.TempDir()
	src := openStore(t, srcDir)
	if _, err := src.CreateBucket(owner, "configs"); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"a":                      "file where a folder is wanted",
		"a/b":                    "beneath a",
		"dir/nested/deep.txt":    "deep",
		"folder/":                "",
		"empty.txt":              "",
		"/leading-slash":         "leading",
		"double//slash":          "double",
		"back\\slash":            "backslash",
		strings.Repeat("x", 300): "long name",
		"unicodé/файл.txt":       "unicode",
		"Case.txt":               "upper",
		"case.txt":               "lower",
		"dup1.txt":               "same content",
		"dup2.txt":               "same content",
	}
	for key, content := range files {
		if _, err := src.PutObject(owner, "configs", key, strings.NewReader(content), filestore.PutOptions{Size: int64(len(content))}); err != nil {
			t.Fatalf("put %q: %v", key, err)
		}
	}

	d, err := filestore.ReadBackup(srcDir)
	if err != nil {
		t.Fatal(err)
	}
	exportDir := filepath.Join(t.TempDir(), "export")
	if err := exportFiles(srcDir, exportDir, d); err != nil {
		t.Fatalf("export: %v", err)
	}

	// Plain keys are where you would look for them, with their times.
	data, err := os.ReadFile(filepath.Join(exportDir, "configs", "dir", "nested", "deep.txt"))
	if err != nil || string(data) != "deep" {
		t.Errorf("exported deep.txt: %q %v", data, err)
	}
	if fi, err := os.Stat(filepath.Join(exportDir, "configs", "folder")); err != nil || !fi.IsDir() {
		t.Errorf("folder marker not exported as a folder: %v", err)
	}
	fi, err := os.Stat(filepath.Join(exportDir, "configs", "unicodé", "файл.txt"))
	if err != nil || time.Since(fi.ModTime()) > time.Minute {
		t.Errorf("unicode file: %v", err)
	}
	index, err := os.ReadFile(filepath.Join(exportDir, conflictsDir, "index.txt"))
	if err != nil {
		t.Fatalf("no conflicts index: %v", err)
	}
	for _, key := range []string{"/leading-slash", "double//slash", "back\\slash", strings.Repeat("x", 300)} {
		if !strings.Contains(string(index), "configs/"+key+"\n") {
			t.Errorf("%q is not in the conflicts index", key)
		}
	}

	// A second export into the same directory is refused.
	if err := exportFiles(srcDir, exportDir, d); err == nil {
		t.Error("export into a non-empty directory")
	}

	src.Close()

	// Restore into fresh storage and read every file back.
	dstDir := t.TempDir()
	dst := openStore(t, dstDir)
	defer dst.Close()
	dst.Restore(d)
	importFiles(dst, exportDir, d)
	for key, content := range files {
		_, f, err := dst.OpenObject(context.Background(), owner, "configs", key)
		if err != nil {
			t.Errorf("open restored %q: %v", key, err)
			continue
		}
		got, _ := io.ReadAll(f)
		f.Close()
		if string(got) != content {
			t.Errorf("restored %q = %q, want %q", key, got, content)
		}
	}
}

func TestExportMissingContent(t *testing.T) {
	srcDir := t.TempDir()
	src := openStore(t, srcDir)
	if _, err := src.CreateBucket(owner, "configs"); err != nil {
		t.Fatal(err)
	}
	o, err := src.PutObject(owner, "configs", "a/b.txt", strings.NewReader("abc"), filestore.PutOptions{Size: 3})
	if err != nil {
		t.Fatal(err)
	}
	src.Close()
	sha := o.SHA256
	os.Remove(filepath.Join(srcDir, "blobs", sha[0:2], sha[2:4], sha))

	d, _ := filestore.ReadBackup(srcDir)
	exportDir := t.TempDir()
	if err := exportFiles(srcDir, exportDir, d); err == nil {
		t.Error("export with missing content reported no error")
	}
	if _, err := os.Stat(filepath.Join(exportDir, "configs", "a")); err == nil {
		t.Error("folder created for missing content")
	}
	if _, err := os.Stat(filepath.Join(exportDir, conflictsDir)); err == nil {
		t.Error("missing content put in the conflicts folder")
	}
}

func TestFilesOwnedBy(t *testing.T) {
	d := &filestore.BackupData{
		Buckets: []*filestore.Bucket{{Name: "aaa", OwnerId: "u1"}, {Name: "bbb", OwnerId: "u2"}},
		Objects: []*filestore.Object{{Bucket: "aaa", Key: "x"}, {Bucket: "bbb", Key: "y"}},
	}
	out := filesOwnedBy(d, "u1")
	if len(out.Buckets) != 1 || out.Buckets[0].Name != "aaa" || len(out.Objects) != 1 || out.Objects[0].Key != "x" {
		t.Errorf("filtered %+v", out)
	}
}
