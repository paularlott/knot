package agent_client

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadFileRange(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.bin")
	if err := os.WriteFile(p, []byte("0123456789"), 0640); err != nil {
		t.Fatal(err)
	}
	mtime := time.Unix(1700000000, 0)
	os.Chtimes(p, mtime, mtime)

	r := readFileRange(p, 2, 4)
	if !r.Success || !r.Ranged || string(r.Content) != "2345" || r.FileSize != 10 || r.MtimeNs != mtime.UnixNano() || r.FilePerm != 0640 {
		t.Errorf("middle range = %+v", r)
	}
	if r := readFileRange(p, 8, 100); string(r.Content) != "89" {
		t.Errorf("past the end = %q", r.Content)
	}
	if r := readFileRange(p, 10, 4); !r.Success || len(r.Content) != 0 || r.FileSize != 10 {
		t.Errorf("at the end = %+v", r)
	}
	if r := readFileRange(p, -1, 4); r.Success {
		t.Error("negative offset: want a failure")
	}
	if r := readFileRange(dir, 0, 4); r.Success {
		t.Error("directory: want a failure")
	}
	if r := readFileRange(filepath.Join(dir, "missing"), 0, 4); r.Success {
		t.Error("missing file: want a failure")
	}
}
