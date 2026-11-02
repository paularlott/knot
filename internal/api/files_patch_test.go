package api

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/filestore"
)

type testRange struct {
	off  int64
	data []byte
}

func encodePatch(ranges []testRange) []byte {
	var b bytes.Buffer
	for _, r := range ranges {
		binary.Write(&b, binary.BigEndian, uint64(r.off))
		binary.Write(&b, binary.BigEndian, uint64(len(r.data)))
		b.Write(r.data)
	}
	return b.Bytes()
}

// applyPatch is the reference: what a patch should make of base.
func applyPatch(base []byte, ranges []testRange, size int64) []byte {
	out := make([]byte, size)
	copy(out, base)
	for _, r := range ranges {
		copy(out[r.off:], r.data)
	}
	return out
}

func runPatch(base []byte, body []byte, size int64) ([]byte, error) {
	pr := &patchReader{base: bytes.NewReader(base), baseLen: int64(len(base)), body: bufio.NewReader(bytes.NewReader(body)), size: size}
	// Read in odd sizes, as a consumer might.
	var out []byte
	buf := make([]byte, 777)
	for {
		n, err := pr.Read(buf)
		out = append(out, buf[:n]...)
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
	}
}

func TestPatchReaderMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 500; i++ {
		base := make([]byte, rng.Intn(20000))
		rng.Read(base)
		// The new size: the same, shorter or longer.
		size := int64(len(base))
		switch rng.Intn(3) {
		case 0:
			size = int64(rng.Intn(len(base) + 1))
		case 1:
			size += int64(rng.Intn(5000))
		}
		// Ranges here and there over the stored file; everything past it
		// is sent, as the server fills nothing in.
		var ranges []testRange
		stored := min(int64(len(base)), size)
		pos, end := int64(0), int64(0)
		for pos < stored {
			pos += int64(rng.Intn(3000))
			if pos >= stored {
				break
			}
			n := min(int64(rng.Intn(2000)), stored-pos)
			data := make([]byte, n)
			rng.Read(data)
			ranges = append(ranges, testRange{pos, data})
			pos += n
			end = pos
		}
		if tail := max(end, stored); tail < size {
			data := make([]byte, size-tail)
			rng.Read(data)
			ranges = append(ranges, testRange{tail, data})
		}
		got, err := runPatch(base, encodePatch(ranges), size)
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if want := applyPatch(base, ranges, size); !bytes.Equal(got, want) {
			t.Fatalf("case %d: content differs (got %d bytes, want %d)", i, len(got), len(want))
		}
	}
}

func TestPatchReaderRefusesBadPatches(t *testing.T) {
	base := []byte("0123456789")
	for name, c := range map[string]struct {
		body []byte
		size int64
	}{
		"overlapping":  {encodePatch([]testRange{{2, []byte("abc")}, {3, []byte("x")}}), 10},
		"out of order": {encodePatch([]testRange{{5, []byte("a")}, {1, []byte("b")}}), 10},
		"past size":    {encodePatch([]testRange{{8, []byte("abc")}}), 10},
		"cut short":    {encodePatch([]testRange{{1, []byte("abcdef")}})[:20], 10},
		"stray bytes":  {append(encodePatch([]testRange{{1, []byte("a")}}), 1, 2, 3), 10},
		"grown unsent": {encodePatch([]testRange{{1, []byte("a")}}), 1 << 40},
		"gap past end": {encodePatch([]testRange{{20, []byte("a")}}), 21},
	} {
		if _, err := runPatch(base, c.body, c.size); !errors.Is(err, errBadPatch) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestFileStatAndPatchHandlers(t *testing.T) {
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
		Zone:     "z-files-patch",
	})
	t.Cleanup(func() { config.SetServerConfig(prev) })
	model.SetRoleCache(nil)

	store, err := filestore.Open(filestore.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	filestore.SetInstance(store)
	t.Cleanup(func() {
		filestore.SetInstance(nil)
		store.Close()
	})
	role := model.NewRole("files-patch-"+t.Name(), []uint16{model.PermissionUseFiles}, "")
	model.SaveRoleToCache(role)
	user := &model.User{Id: "fp-carol", Username: "carol", Email: "carol@test.local", Active: true, Roles: []string{role.Id}}
	if err := database.GetInstance().SaveUser(user, nil); err != nil {
		t.Fatal(err)
	}
	p, _ := filestore.PrincipalFor(user)
	if _, err := store.CreateBucket(p, "carol--b"); err != nil {
		t.Fatal(err)
	}
	put := func(key, content string, meta map[string]string) *filestore.Object {
		o, err := store.PutObject(p, "carol--b", key, strings.NewReader(content), filestore.PutOptions{Size: int64(len(content)), Meta: meta})
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	file := put("dir/file.txt", "hello world", map[string]string{"mode": "33188"})
	put("dir/sub/", "", map[string]string{"mode": "16877"})
	put("dir/implicit/x", "x", nil)

	stat := func(key string) (int, apiclient.FileStat) {
		req := filesRequest("GET", "/api/files/stat/b/"+key, "", user)
		req.SetPathValue("bucket", "b")
		req.SetPathValue("key", key)
		rr := httptest.NewRecorder()
		HandleStatFileObject(rr, req)
		var out apiclient.FileStat
		json.Unmarshal(rr.Body.Bytes(), &out)
		return rr.Code, out
	}
	if code, s := stat("dir/file.txt"); code != 200 || s.File == nil || s.File.Size != 11 || s.Folder {
		t.Errorf("stat file: %d %+v", code, s)
	}
	if code, s := stat("dir/sub"); code != 200 || !s.Folder || s.Marker == nil || s.Marker.Meta["mode"] != "16877" {
		t.Errorf("stat folder with marker: %d %+v", code, s)
	}
	if code, s := stat("dir/implicit"); code != 200 || !s.Folder || s.Marker != nil {
		t.Errorf("stat folder without marker: %d %+v", code, s)
	}
	if code, _ := stat("dir/missing"); code != http.StatusNotFound {
		t.Errorf("stat missing: %d", code)
	}

	// A listing with markers gives each folder's marker.
	req := filesRequest("GET", "/api/files/list/b?prefix=dir/&delimiter=/&markers=1", "", user)
	req.SetPathValue("bucket", "b")
	rr := httptest.NewRecorder()
	HandleListFileObjects(rr, req)
	var list apiclient.FileObjectList
	json.Unmarshal(rr.Body.Bytes(), &list)
	if m, ok := list.Markers["dir/sub/"]; !ok || m.Meta["mode"] != "16877" || len(list.Markers) != 1 {
		t.Errorf("list markers: %d %+v", rr.Code, list.Markers)
	}

	patch := func(ranges []testRange, size int64, ifMatch string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("PATCH", "/api/files/objects/b/dir/file.txt", bytes.NewReader(encodePatch(ranges)))
		req = req.WithContext(filesRequest("PATCH", "/", "", user).Context())
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", apiclient.FilePatchContentType)
		req.Header.Set(apiclient.FileSizeHeader, strconv.FormatInt(size, 10))
		if ifMatch != "" {
			req.Header.Set("If-Match", `"`+ifMatch+`"`)
		}
		req.SetPathValue("bucket", "b")
		req.SetPathValue("key", "dir/file.txt")
		rr := httptest.NewRecorder()
		HandlePatchFileObject(rr, req)
		return rr
	}
	read := func() string {
		_, c, err := store.OpenObject(t.Context(), p, "carol--b", "dir/file.txt")
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		b, _ := io.ReadAll(c)
		return string(b)
	}

	if rr := patch([]testRange{{6, []byte("WORLD")}, {11, []byte("!!")}}, 13, file.ETag); rr.Code != 200 {
		t.Fatalf("patch: %d %s", rr.Code, rr.Body.String())
	}
	if got := read(); got != "hello WORLD!!" {
		t.Fatalf("patched = %q", got)
	}
	// Metadata the patch does not give is kept.
	if o, _ := store.HeadObject(p, "carol--b", "dir/file.txt"); o.Meta["mode"] != "33188" {
		t.Errorf("metadata lost: %v", o.Meta)
	}
	// Against a file that has changed since, refused.
	if rr := patch([]testRange{{0, []byte("H")}}, 13, file.ETag); rr.Code != http.StatusPreconditionFailed {
		t.Errorf("stale patch: %d %s", rr.Code, rr.Body.String())
	}
	// Truncating, and a bad patch.
	if rr := patch(nil, 5, ""); rr.Code != 200 || read() != "hello" {
		t.Errorf("truncate: %d %q", rr.Code, read())
	}
	if rr := patch([]testRange{{3, []byte("abcdef")}}, 5, ""); rr.Code != http.StatusBadRequest {
		t.Errorf("bad patch: %d %s", rr.Code, rr.Body.String())
	}
}
