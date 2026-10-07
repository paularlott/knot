package backupfile

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const key = "0123456789abcdef0123456789abcdef"

func records(t *testing.T, dir, kind, k string) []string {
	t.Helper()
	r, err := Open(dir, kind, k)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var out []string
	if err := r.Each(func(l []byte) error { out = append(out, string(l)); return nil }); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRecordsRoundTrip(t *testing.T) {
	for _, k := range []string{"", key} {
		dir := t.TempDir()
		w, err := Create(dir, "users", k)
		if err != nil {
			t.Fatal(err)
		}
		var want []string
		for i := 0; i < 2500; i++ { // several blocks
			line := fmt.Sprintf(`{"id":%d,"note":"line with \"quotes\" and unicode é 日本"}`, i)
			want = append(want, line)
			if err := w.Add([]byte(line)); err != nil {
				t.Fatal(err)
			}
		}
		if w.Count() != 2500 {
			t.Errorf("count %d", w.Count())
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}

		got := records(t, dir, "users", k)
		if len(got) != len(want) || got[0] != want[0] || got[1234] != want[1234] || got[2499] != want[2499] {
			t.Errorf("key %q: read back %d records", k, len(got))
		}
		raw, _ := os.ReadFile(RecordPath(dir, "users"))
		if (k != "") == strings.Contains(string(raw), "unicode") {
			t.Errorf("key %q: encrypted=%v but the file shows its content=%v", k, k != "", strings.Contains(string(raw), "unicode"))
		}
	}
}

func TestEmptyKindAndMissing(t *testing.T) {
	dir := t.TempDir()
	w, _ := Create(dir, "groups", key)
	w.Close()
	if got := records(t, dir, "groups", key); len(got) != 0 {
		t.Errorf("empty kind read back %v", got)
	}
	if _, err := Open(dir, "roles", ""); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a kind with no file: %v", err)
	}
}

func TestEncryptionChecks(t *testing.T) {
	dir := t.TempDir()
	w, _ := Create(dir, "users", key)
	for i := 0; i < 3000; i++ {
		w.Add([]byte(fmt.Sprintf(`{"i":%d}`, i)))
	}
	w.Close()

	if _, err := Open(dir, "users", ""); err == nil || !strings.Contains(err.Error(), "encrypted") {
		t.Errorf("no key: %v", err)
	}
	r, _ := Open(dir, "users", strings.Repeat("x", 32))
	if err := r.Each(func([]byte) error { return nil }); !errors.Is(err, ErrWrongKey) {
		t.Errorf("wrong key: %v", err)
	}
	r.Close()
	if _, err := Create(dir, "bad", "short"); err == nil {
		t.Error("a short key was accepted")
	}

	// Blocks cannot be reordered or dropped from the middle unnoticed.
	raw, _ := os.ReadFile(RecordPath(dir, "users"))
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 4 { // header and three blocks
		t.Fatalf("%d lines", len(lines))
	}
	for name, edited := range map[string][]string{
		"swapped": {lines[0], lines[2], lines[1], lines[3]},
		"dropped": {lines[0], lines[1], lines[3]},
	} {
		os.WriteFile(RecordPath(dir, "users"), []byte(strings.Join(edited, "\n")+"\n"), 0600)
		r, err := Open(dir, "users", key)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Each(func([]byte) error { return nil }); !errors.Is(err, ErrWrongKey) {
			t.Errorf("%s blocks: %v", name, err)
		}
		r.Close()
	}
}

func TestEachStopsOnError(t *testing.T) {
	dir := t.TempDir()
	w, _ := Create(dir, "users", "")
	for i := 0; i < 10; i++ {
		w.Add([]byte(`{}`))
	}
	w.Close()
	r, _ := Open(dir, "users", "")
	defer r.Close()
	stop := errors.New("stop")
	n := 0
	if err := r.Each(func([]byte) error {
		n++
		if n == 3 {
			return stop
		}
		return nil
	}); !errors.Is(err, stop) || n != 3 {
		t.Errorf("stopped after %d: %v", n, err)
	}
}

func TestContent(t *testing.T) {
	dir := t.TempDir()
	data := strings.Repeat("content ", 1000)
	sum := sha256.Sum256([]byte(data))
	sha := hex.EncodeToString(sum[:])

	if HasContent(dir, sha) {
		t.Fatal("held before written")
	}
	if _, err := WriteContent(dir, sha, strings.NewReader("other data")); err == nil {
		t.Error("content that does not match its checksum was kept")
	}
	if HasContent(dir, sha) {
		t.Error("mismatched content stored")
	}
	if entries, _ := os.ReadDir(filepath.Dir(ContentPath(dir, sha))); len(entries) != 0 {
		t.Errorf("a failed write left %d files", len(entries))
	}
	n, err := WriteContent(dir, sha, strings.NewReader(data))
	if err != nil || n != int64(len(data)) || !HasContent(dir, sha) {
		t.Fatalf("write: %d %v", n, err)
	}
	f, err := OpenContent(dir, sha)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := WriteContent(dir, "short", strings.NewReader("x")); err == nil {
		t.Error("an invalid checksum was accepted")
	}
}

func TestManifest(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadManifest(dir); err == nil || !strings.Contains(err.Error(), "no complete backup") {
		t.Errorf("empty folder: %v", err)
	}
	if err := WriteManifest(dir, &Manifest{Server: "v1", Counts: map[string]int{"users": 3}, Content: true}); err != nil {
		t.Fatal(err)
	}
	m, err := ReadManifest(dir)
	if err != nil || m.Version != Version || m.Counts["users"] != 3 || !m.Content {
		t.Errorf("manifest %+v %v", m, err)
	}
	os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"version":99}`), 0600)
	if _, err := ReadManifest(dir); err == nil {
		t.Error("a newer format was read")
	}
}

// A copy that breaks off is carried on from where it stopped, and a server
// that ignores the offset is handled.
func TestDownloadContentResumes(t *testing.T) {
	dir := t.TempDir()
	data := strings.Repeat("0123456789", 5000)
	sum := sha256.Sum256([]byte(data))
	sha := hex.EncodeToString(sum[:])

	// The first attempt breaks off part way.
	_, err := DownloadContent(dir, sha, func(offset int64) (io.ReadCloser, bool, error) {
		return io.NopCloser(io.MultiReader(strings.NewReader(data[:20000]), errReader{io.ErrUnexpectedEOF})), true, nil
	})
	if err == nil || HasContent(dir, sha) {
		t.Fatalf("a broken copy: %v", err)
	}
	if info, _ := os.Stat(PartialPath(dir, sha)); info == nil || info.Size() != 20000 {
		t.Fatalf("partial copy %v", info)
	}

	// The second carries on from the offset.
	var asked int64 = -1
	n, err := DownloadContent(dir, sha, func(offset int64) (io.ReadCloser, bool, error) {
		asked = offset
		return io.NopCloser(strings.NewReader(data[offset:])), true, nil
	})
	if err != nil || asked != 20000 || n != int64(len(data))-20000 || !HasContent(dir, sha) {
		t.Fatalf("resumed copy: asked %d, copied %d: %v", asked, n, err)
	}
	if _, err := os.Stat(PartialPath(dir, sha)); !os.IsNotExist(err) {
		t.Error("partial copy left behind")
	}
	got, _ := os.ReadFile(ContentPath(dir, sha))
	if string(got) != data {
		t.Error("resumed content differs")
	}

	// A server that sends it all again, though asked for an offset.
	dir2 := t.TempDir()
	DownloadContent(dir2, sha, func(int64) (io.ReadCloser, bool, error) {
		return io.NopCloser(io.MultiReader(strings.NewReader(data[:100]), errReader{io.ErrUnexpectedEOF})), true, nil
	})
	if _, err := DownloadContent(dir2, sha, func(int64) (io.ReadCloser, bool, error) {
		return io.NopCloser(strings.NewReader(data)), false, nil
	}); err != nil || !HasContent(dir2, sha) {
		t.Errorf("a full resend: %v", err)
	}

	// Content that is wrong is not kept, nor its partial copy.
	dir3 := t.TempDir()
	if _, err := DownloadContent(dir3, sha, func(int64) (io.ReadCloser, bool, error) {
		return io.NopCloser(strings.NewReader("wrong")), false, nil
	}); err == nil || HasContent(dir3, sha) {
		t.Error("wrong content kept")
	}
	if _, err := os.Stat(PartialPath(dir3, sha)); !os.IsNotExist(err) {
		t.Error("partial copy of wrong content kept")
	}
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

func TestPrune(t *testing.T) {
	dir := t.TempDir()
	var keep, drop string
	for i, v := range []*string{&keep, &drop} {
		data := fmt.Sprintf("content %d", i)
		sum := sha256.Sum256([]byte(data))
		*v = hex.EncodeToString(sum[:])
		if _, err := WriteContent(dir, *v, strings.NewReader(data)); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(PartialPath(dir, drop), []byte("half"), 0600)
	removed, freed, err := Prune(dir, func(sha string) bool { return sha == keep })
	if err != nil || removed != 2 || freed == 0 {
		t.Fatalf("pruned %d (%d bytes): %v", removed, freed, err)
	}
	if !HasContent(dir, keep) || HasContent(dir, drop) {
		t.Error("wrong content pruned")
	}
	if _, err := os.Stat(filepath.Dir(ContentPath(dir, drop))); !os.IsNotExist(err) && filepath.Dir(ContentPath(dir, drop)) != filepath.Dir(ContentPath(dir, keep)) {
		t.Error("empty folder left")
	}
}

func TestUnfinishedMark(t *testing.T) {
	dir := t.TempDir()
	if IsBackupDir(dir) {
		t.Fatal("an empty folder is not a backup")
	}
	if err := MarkUnfinished(dir); err != nil {
		t.Fatal(err)
	}
	if !IsBackupDir(dir) {
		t.Fatal("an unfinished backup folder is a backup folder")
	}
	if _, err := ReadManifest(dir); err == nil {
		t.Fatal("an unfinished backup has no manifest to read")
	}
	if err := WriteManifest(dir, &Manifest{Counts: map[string]int{}}); err != nil {
		t.Fatal(err)
	}
	if err := ClearUnfinished(dir); err != nil {
		t.Fatal(err)
	}
	if err := ClearUnfinished(dir); err != nil {
		t.Fatalf("clearing twice: %v", err)
	}
	if !IsBackupDir(dir) {
		t.Fatal("a finished backup is a backup folder")
	}
}
