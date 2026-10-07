package commands_admin

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/paularlott/knot/internal/backupfile"
)

const testBucket = "11111111-1111-7111-8111-111111111111"

// backupWith writes a backup folder holding the files in one bucket, and one
// file in another, to list.
func backupWith(t *testing.T, key string, keys ...string) string {
	t.Helper()
	dir := t.TempDir()
	b, err := backupfile.Create(dir, "file-buckets", key)
	if err != nil {
		t.Fatal(err)
	}
	b.Add([]byte(`{"id":"` + testBucket + `","name":"alice--docs","owner_id":"u1"}`))
	b.Add([]byte(`{"id":"22222222-2222-7222-8222-222222222222","name":"bob--logs","owner_id":"u2"}`))
	b.Close()
	o, err := backupfile.Create(dir, "file-objects", key)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		line, _ := json.Marshal(map[string]any{"bucket_id": testBucket, "key": k, "size": 1})
		o.Add(line)
	}
	other, _ := json.Marshal(map[string]any{"bucket_id": "22222222-2222-7222-8222-222222222222", "key": "dir/other.log", "size": 1})
	o.Add(other)
	o.Close()
	if err := backupfile.WriteManifest(dir, &backupfile.Manifest{Encrypted: key != "", Counts: map[string]int{"file-buckets": 2, "file-objects": len(keys) + 1}}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func keysOf(files []backupObject) []string {
	out := []string{}
	for _, f := range files {
		out = append(out, f.Key)
	}
	return out
}

func TestListPath(t *testing.T) {
	const key = "0123456789abcdef0123456789abcdef"
	for _, k := range []string{"", key} {
		dir := backupWith(t, k, "a.txt", "dir/", "dir/b.txt", "dir/sub/c.txt", "dir/sub/deeper/d.txt", "dir2/e.txt", "file", "file.bak")

		for _, tc := range []struct {
			name      string
			prefix    string
			recursive bool
			folders   []string
			files     []string
		}{
			{"the bucket, one level", "", false, []string{"dir/", "dir2/"}, []string{"a.txt", "file", "file.bak"}},
			{"the bucket, everything", "", true, nil, []string{"a.txt", "dir/", "dir/b.txt", "dir/sub/c.txt", "dir/sub/deeper/d.txt", "dir2/e.txt", "file", "file.bak"}},
			{"a folder with its slash", "dir/", false, []string{"dir/sub/"}, []string{"dir/b.txt"}},
			{"a folder without it", "dir", false, []string{"dir/sub/"}, []string{"dir/b.txt"}},
			{"a folder, everything below", "dir/", true, nil, []string{"dir/b.txt", "dir/sub/c.txt", "dir/sub/deeper/d.txt"}},
			{"a nested folder", "dir/sub", false, []string{"dir/sub/deeper/"}, []string{"dir/sub/c.txt"}},
			{"a file", "dir/b.txt", false, nil, []string{"dir/b.txt"}},
			{"a file that is also a prefix of another name", "file", false, nil, []string{"file"}},
			{"a prefix that is not a folder", "dir2x", false, nil, nil},
			{"nothing", "missing/", true, nil, nil},
		} {
			folders, files, err := listPath(dir, k, testBucket, tc.prefix, tc.recursive)
			if err != nil {
				t.Fatal(err)
			}
			if got := keysOf(files); !reflect.DeepEqual(got, orEmpty(tc.files)) {
				t.Errorf("key %q, %s: files %v, want %v", k, tc.name, got, tc.files)
			}
			if !reflect.DeepEqual(orEmpty(folders), orEmpty(tc.folders)) {
				t.Errorf("key %q, %s: folders %v, want %v", k, tc.name, folders, tc.folders)
			}
		}

		buckets, err := openBackupFiles(dir, k)
		if err != nil || len(buckets) != 2 || findBucket(buckets, "alice--docs") == nil || findBucket(buckets, testBucket) == nil || findBucket(buckets, "nobody") != nil {
			t.Errorf("buckets %+v: %v", buckets, err)
		}
	}

	if _, err := openBackupFiles(backupWith(t, "0123456789abcdef0123456789abcdef", "a"), ""); err == nil {
		t.Error("an encrypted backup was opened without its key")
	}
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func TestSplitBucketPath(t *testing.T) {
	for arg, want := range map[string][2]string{
		"alice--docs:a/b.txt": {"alice--docs", "a/b.txt"},
		"alice--docs:":        {"alice--docs", ""},
		"alice--docs:dir/":    {"alice--docs", "dir/"},
		"b:with:colons":       {"b", "with:colons"},
	} {
		b, p, err := splitBucketPath(arg)
		if err != nil || b != want[0] || p != want[1] {
			t.Errorf("%q split as %q %q: %v", arg, b, p, err)
		}
	}
	for _, bad := range []string{"nocolon", ":path", ""} {
		if _, _, err := splitBucketPath(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}
