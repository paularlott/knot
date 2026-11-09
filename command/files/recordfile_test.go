package command_files

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testFile is a record file of hash cache entries at path.
func testFile(path string) *recordFile[hashCacheEntry] {
	return newRecordFile(path, hashCacheMagic, hashCacheVersion, "/some/dir", hashCodec)
}

func testEntry(i int) hashCacheEntry {
	e := hashCacheEntry{Size: int64(i), Mtime: 1_700_000_000_000_000_000 + int64(i), Ino: uint64(i) * 7, Ctime: -int64(i)}
	e.SHA[0], e.SHA[31] = byte(i), byte(i>>8)
	return e
}

// setAll makes want what f holds, and saves it.
func setAll(t *testing.T, f *recordFile[hashCacheEntry], want map[string]hashCacheEntry) {
	t.Helper()
	f.update(maps.All(want), func(p string) (hashCacheEntry, bool) { e, ok := want[p]; return e, ok })
	if err := f.save(); err != nil {
		t.Fatal(err)
	}
}

func reload(t *testing.T, path string) (map[string]hashCacheEntry, *recordFile[hashCacheEntry]) {
	t.Helper()
	f := testFile(path)
	held, err := f.load()
	if err != nil {
		t.Fatal(err)
	}
	return maps.Clone(held), f
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

// A change to one entry of many appends one small record; a removal, one
// delete record.
func TestRecordFileAppendsChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hashes.bin")
	want := map[string]hashCacheEntry{}
	for i := range 10000 {
		want[fmt.Sprintf("dir%03d/file%05d.txt", i%100, i)] = testEntry(i)
	}
	f := testFile(path)
	f.load()
	setAll(t, f, want)
	full := fileSize(t, path)

	want["dir007/file00007.txt"] = testEntry(99999)
	setAll(t, f, want)
	if grown := fileSize(t, path) - full; grown <= 0 || grown > 100 {
		t.Errorf("one change wrote %d bytes, want one record", grown)
	}
	delete(want, "dir008/file00008.txt")
	before := fileSize(t, path)
	setAll(t, f, want)
	if grown := fileSize(t, path) - before; grown <= 0 || grown > 40 {
		t.Errorf("one removal wrote %d bytes, want one delete record", grown)
	}
	// Nothing changed: nothing written.
	before = fileSize(t, path)
	setAll(t, f, want)
	if fileSize(t, path) != before {
		t.Error("a save with nothing changed wrote")
	}

	got, g := reload(t, path)
	if !maps.Equal(got, want) || g.lost {
		t.Errorf("reloaded %d entries (lost %v), want %d", len(got), g.lost, len(want))
	}
}

// Once the records replaced outweigh those that count, the file is written
// afresh: it never grows past about twice what it holds.
func TestRecordFileRewritesWhenGarbageOutweighs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hashes.bin")
	want := map[string]hashCacheEntry{}
	for i := range 4000 {
		want[fmt.Sprintf("f%05d", i)] = testEntry(i)
	}
	f := testFile(path)
	f.load()
	setAll(t, f, want)
	rewrites, last := 0, fileSize(t, path)
	for round := range 40 {
		for i := range 400 {
			want[fmt.Sprintf("f%05d", (round*400+i)%4000)] = testEntry(round*1000 + i)
		}
		setAll(t, f, want)
		size := fileSize(t, path)
		if size < last {
			rewrites++
		}
		last = size
		if limit := int64(len(f.header())) + 2*f.live + recMinGarbage; size > limit {
			t.Fatalf("round %d: file is %d bytes, holding %d", round, size, f.live)
		}
		if f.size+int64(len(f.header())) != size {
			t.Fatalf("round %d: counted %d bytes of records, file has %d", round, f.size, size-int64(len(f.header())))
		}
	}
	if rewrites == 0 {
		t.Error("never written afresh")
	}
	if got, _ := reload(t, path); !maps.Equal(got, want) {
		t.Error("reloaded entries differ")
	}
}

// buildInOrder writes n entries, one record each, in order.
func buildInOrder(t *testing.T, path string, n int) map[string]hashCacheEntry {
	t.Helper()
	f := testFile(path)
	f.load()
	want := map[string]hashCacheEntry{}
	setAll(t, f, want)
	for i := range n {
		want[fmt.Sprintf("p%03d", i)] = testEntry(i)
		setAll(t, f, want)
	}
	return want
}

// Damage loses the records from the damaged one on, keeps those before, and
// the next save writes the file afresh.
func TestRecordFileDamage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hashes.bin")
	want := buildInOrder(t, path, 100)
	data, _ := os.ReadFile(path)
	// The 51st record: records are the same length here.
	recLen := (len(data) - len(testFile(path).header())) / 100
	data[len(testFile(path).header())+50*recLen+recLen/2] ^= 0x40
	os.WriteFile(path, data, 0600)

	got, f := reload(t, path)
	if !f.lost || len(got) != 50 {
		t.Fatalf("kept %d entries (lost %v), want the 50 before the damage", len(got), f.lost)
	}
	for p, e := range got {
		if want[p] != e || p >= "p050" {
			t.Errorf("kept %s wrongly", p)
		}
	}
	// The sync puts back what was lost; the file is written afresh.
	setAll(t, f, want)
	if got, g := reload(t, path); g.lost || !maps.Equal(got, want) {
		t.Errorf("after the save: %d entries, lost %v", len(got), g.lost)
	}
}

// A record cut short, as by a crash part way through an append, is
// dropped, and the next save does not append after it.
func TestRecordFileTornTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hashes.bin")
	want := buildInOrder(t, path, 10)
	os.Truncate(path, fileSize(t, path)-3)
	got, f := reload(t, path)
	if !f.lost || len(got) != 9 {
		t.Fatalf("kept %d (lost %v), want 9", len(got), f.lost)
	}
	want["new"] = testEntry(500)
	setAll(t, f, want)
	if got, g := reload(t, path); g.lost || !maps.Equal(got, want) {
		t.Errorf("after the save: %d entries, lost %v", len(got), g.lost)
	}
}

// A file another sync wrote meanwhile is written afresh, not appended to,
// so the two never interleave.
func TestRecordFileWrittenMeanwhile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hashes.bin")
	want := buildInOrder(t, path, 10)
	_, a := reload(t, path)
	_, b := reload(t, path)
	wantA, wantB := maps.Clone(want), maps.Clone(want)
	wantA["a"], wantB["b"] = testEntry(1000), testEntry(2000)
	setAll(t, a, wantA)
	setAll(t, b, wantB)
	if got, g := reload(t, path); g.lost || !maps.Equal(got, wantB) {
		t.Errorf("got %d entries (lost %v), want the last writer's", len(got), g.lost)
	}
}

// A file left empty or zeroed, as a power cut can leave one written just
// before, is damage: nothing kept, and written afresh.
func TestRecordFileZeroed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hashes.bin")
	n := fileSize(t, func() string { buildInOrder(t, path, 5); return path }())
	os.WriteFile(path, make([]byte, n), 0600)
	got, f := reload(t, path)
	if !f.lost || len(got) != 0 {
		t.Fatalf("zeroed file: kept %d, lost %v", len(got), f.lost)
	}
	want := map[string]hashCacheEntry{"a": testEntry(1)}
	setAll(t, f, want)
	if got, g := reload(t, path); g.lost || !maps.Equal(got, want) {
		t.Errorf("after the save: %v, lost %v", got, g.lost)
	}
}

// A file that is not a sync file is refused, and left alone; one of another
// directory is no cache, and replaced.
func TestRecordFileForeignAndOther(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.json")
	os.WriteFile(path, []byte(`{"precious":true}`), 0600)
	if _, err := testFile(path).load(); !errors.Is(err, errRecordForeign) {
		t.Errorf("load of a foreign file: %v", err)
	}

	other := filepath.Join(t.TempDir(), "hashes.bin")
	buildInOrder(t, other, 3)
	f := newRecordFile(other, hashCacheMagic, hashCacheVersion, "/another/dir", hashCodec)
	if held, err := f.load(); err != nil || len(held) != 0 || f.otherIdentity != "/some/dir" {
		t.Fatalf("load of another directory's cache: %d %v %q", len(held), err, f.otherIdentity)
	}
	want := map[string]hashCacheEntry{"x": testEntry(1)}
	setAll(t, f, want)
	g := newRecordFile(other, hashCacheMagic, hashCacheVersion, "/another/dir", hashCodec)
	if held, err := g.load(); err != nil || !maps.Equal(held, want) {
		t.Errorf("replaced cache holds %v %v", held, err)
	}

	newer := filepath.Join(t.TempDir(), "hashes.bin")
	buildInOrder(t, newer, 1)
	if _, err := newRecordFile(newer, hashCacheMagic, hashCacheVersion+1, "/some/dir", hashCodec).load(); !errors.Is(err, errRecordVersion) {
		t.Errorf("load of another version: %v", err)
	}
}

// A file named as the cache that is not one stops the sync rather than
// being overwritten.
func TestSyncRefusesForeignHashFile(t *testing.T) {
	client, _ := newFilesServer(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "a")
	path := filepath.Join(t.TempDir(), "mine.txt")
	os.WriteFile(path, []byte("my own notes"), 0600)
	err := runSyncRequest(context.Background(), client, dir, "sync-test:foreign", syncRequest{hashPath: path})
	if err == nil || !strings.Contains(err.Error(), "not a knot sync file") {
		t.Errorf("sync with a foreign --hash-file: %v", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "my own notes" {
		t.Errorf("foreign file changed: %q", b)
	}
}

// Every field of a state entry comes back, whether the key is the prefix
// and path or not, and whatever the checksum looks like.
func TestBaseCodecRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.bin")
	prefix := "tw/"
	want := map[string]baseEntry{
		"a.txt":     {Key: "tw/a.txt", SHA: strings.Repeat("ab", 32), Size: 5, LocalSize: 5, LocalMtime: 1_700_000_000_123_456_789, Mode: 0o100755, LocalIno: 1 << 40, LocalCtime: 1_700_000_001_000_000_000},
		"readme.md": {Key: "tw/README.md", SHA: strings.Repeat("CD", 32), Size: 1},
		"folder/":   {Key: "tw/folder/", Mode: 0o40755},
		"odd":       {Key: "tw/odd", SHA: "not-a-checksum", Size: -1, LocalMtime: -5},
	}
	mk := func() *recordFile[baseEntry] {
		return newRecordFile(path, stateMagic, stateVersion, stateIdentity("b", prefix, "/d"), baseCodec(prefix))
	}
	f := mk()
	f.load()
	f.update(maps.All(want), func(p string) (baseEntry, bool) { e, ok := want[p]; return e, ok })
	if err := f.save(); err != nil {
		t.Fatal(err)
	}
	got, err := mk().load()
	if err != nil || !maps.Equal(got, want) {
		t.Errorf("round trip:\n got %+v\nwant %+v (%v)", got, want, err)
	}
}

// A two-way sync with one file changed appends to its state rather than
// writing it all again.
func TestSyncStateAppends(t *testing.T) {
	client, _ := newFilesServer(t)
	dir := t.TempDir()
	state := filepath.Join(t.TempDir(), "state.bin")
	for i := range 200 {
		writeFile(t, filepath.Join(dir, fmt.Sprintf("f%03d.txt", i)), fmt.Sprintf("file %d", i))
	}
	twoWay(t, client, dir, "sync-test:app", state, false)
	full := fileSize(t, state)
	writeFile(t, filepath.Join(dir, "f007.txt"), "changed")
	twoWay(t, client, dir, "sync-test:app", state, false)
	if grown := fileSize(t, state) - full; grown <= 0 || grown > 150 {
		t.Errorf("one change grew the state by %d bytes (of %d), want one record", grown, full)
	}
}

// A damaged state costs only a first run for what it lost: a file deleted
// here since is put back rather than deleted from the bucket, and the state
// is whole again after.
func TestSyncDamagedState(t *testing.T) {
	client, _ := newFilesServer(t)
	dir := t.TempDir()
	state := filepath.Join(t.TempDir(), "state.bin")
	for i := range 20 {
		writeFile(t, filepath.Join(dir, fmt.Sprintf("f%02d.txt", i)), fmt.Sprintf("file %d", i))
	}
	twoWay(t, client, dir, "sync-test:dmg", state, true)

	// Damage the header: everything is lost.
	data, _ := os.ReadFile(state)
	data[6] ^= 0xff
	os.WriteFile(state, data, 0600)
	os.Remove(filepath.Join(dir, "f03.txt"))
	out := twoWay(t, client, dir, "sync-test:dmg", state, true)
	if !logged(out, "sync state damaged", `"kept":0`) {
		t.Errorf("damage not reported:\n%s", out)
	}
	if readLocal(t, filepath.Join(dir, "f03.txt")) != "file 3" {
		t.Error("a file deleted here was not put back on a first run")
	}
	if _, ok := remoteContent(t, client, "sync-test", "dmg/f03.txt"); !ok {
		t.Error("a damaged state deleted from the bucket")
	}

	// Whole again: a deletion now passes across.
	out = twoWay(t, client, dir, "sync-test:dmg", state, true)
	if strings.Contains(out, "damaged") {
		t.Errorf("state still damaged:\n%s", out)
	}
	os.Remove(filepath.Join(dir, "f03.txt"))
	twoWay(t, client, dir, "sync-test:dmg", state, true)
	if _, ok := remoteContent(t, client, "sync-test", "dmg/f03.txt"); ok {
		t.Error("deletion not passed across after the state was rebuilt")
	}

	// Damage part way: the records before it are kept.
	data, _ = os.ReadFile(state)
	os.WriteFile(state, data[:len(data)/2], 0600)
	out = twoWay(t, client, dir, "sync-test:dmg", state, true)
	if !logged(out, "sync state damaged") || logged(out, "sync state damaged", `"kept":0`) {
		t.Errorf("part of the state not kept:\n%s", out)
	}
	if got := len(localFiles(t, dir)); got != 19 {
		t.Errorf("%d files after a partly damaged state, want 19", got)
	}
}

// Whatever a file holds, reading it never fails worse than losing records,
// and what was read saves and reads back the same.
func FuzzRecordFileLoad(f *testing.F) {
	dir := f.TempDir()
	seed := filepath.Join(dir, "seed.bin")
	g := testFile(seed)
	g.load()
	g.update(maps.All(map[string]hashCacheEntry{"a": testEntry(1), "b/c": testEntry(2)}), func(string) (hashCacheEntry, bool) { return hashCacheEntry{}, true })
	g.save()
	data, _ := os.ReadFile(seed)
	f.Add(data)
	f.Add(data[:len(data)-5])
	f.Add([]byte(hashCacheMagic))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		path := filepath.Join(t.TempDir(), "f.bin")
		os.WriteFile(path, data, 0600)
		r := newRecordFile(path, hashCacheMagic, hashCacheVersion, "/some/dir", hashCodec)
		held, err := r.load()
		if err != nil {
			return
		}
		held = maps.Clone(held)
		r.update(maps.All(held), func(p string) (hashCacheEntry, bool) { e, ok := held[p]; return e, ok })
		if err := r.save(); err != nil {
			t.Fatal(err)
		}
		again, g := reload(t, path)
		if g.lost || !maps.Equal(again, held) {
			t.Fatalf("read back %d entries (lost %v), saved %d", len(again), g.lost, len(held))
		}
	})
}
