//go:build integration

package suites

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
)

// snapFile is the content of write seq, to file i: a first line naming
// both, and filler to its size. Every eighth file is larger than the store
// keeps in its database, so content files are shared too.
func snapFile(seq, i int) []byte {
	head := fmt.Sprintf("seq %d file %d\n", seq, i)
	size := 2 << 10
	if i%8 == 0 {
		size = 100 << 10
	}
	return bytes.Repeat([]byte(head), size/len(head)+1)[:size]
}

// TestFileSnapshot takes snapshots of folders being rewritten as fast as one
// client can, on both servers of a cluster, with knot file snapshot. A
// writer goes round each folder's files, numbering its writes, one at a
// time: write k to file 7k mod n, an order unlike the files' own, so a copy
// read a file at a time, in order, cannot keep step with it. The newest
// write a snapshot holds, S, then says what every file held at that moment:
// its last write numbered no more than S. A snapshot holding anything else
// was read over a time, not at one moment, as a copy made a file at a time
// is whenever a write lands while it is made. Each snapshot holds every file and its manifest, both
// servers end with every snapshot, every file of which reads back in full
// from both, and fsck is clean.
func TestFileSnapshot(t *testing.T) {
	harness.Feature(t, "file-storage")
	root := filesRoot(t)
	nodeA, nodeB, adminA, adminB := bootFilePair(t, root, "snapa", "snapb")
	servers := []writer{{"A", nodeA, adminA}, {"B", nodeB, adminB}}
	ctx, cancel := testCtx(900)
	defer cancel()
	const bucket = "snap"
	if _, err := adminA.Client.CreateFileBucket(ctx, bucket); err != nil {
		t.Fatal(err)
	}
	if !waitForCond(60, func() bool { _, err := adminB.Client.GetFileBucket(ctx, bucket); return err == nil }) {
		t.Fatal("the bucket did not reach server B")
	}

	const files = 40
	const snapshots = 10
	// Write k is to file fileOf(k); file i's writes are those k = writeOf(i)
	// mod files (7 and 23 are inverses mod 40).
	fileOf := func(k int) int { return k * 7 % files }
	writeOf := func(i int) int { return i * 23 % files }
	key := func(s writer, i int) string { return fmt.Sprintf("proj%s/f%03d", s.name, i) }
	put := func(s writer, i, seq int) error {
		data := snapFile(seq, i)
		_, err := s.client().PutFileObject(ctx, bucket, key(s, i), bytes.NewReader(data), int64(len(data)), "", apiclient.FileAttrs{})
		return err
	}
	for _, s := range servers {
		for k := range files {
			if err := put(s, fileOf(k), k); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Each server's folder is rewritten through that server, and snapshotted
	// on it, as the other server's is at the same time.
	var errs errorList
	var writers, snappers sync.WaitGroup
	stop := make(chan struct{})
	writes := map[string]int{}
	var writesMu sync.Mutex
	for _, s := range servers {
		writers.Add(1)
		go func(s writer) {
			defer writers.Done()
			// Writes 0 to files-1 made the files.
			for seq := files; ; seq++ {
				select {
				case <-stop:
					return
				default:
				}
				if err := put(s, fileOf(seq), seq); err != nil {
					errs.add("write %s on %s: %v", key(s, fileOf(seq)), s.name, err)
					return
				}
				writesMu.Lock()
				writes[s.name] = seq + 1
				writesMu.Unlock()
			}
		}(s)
	}
	took := map[string][]time.Duration{}
	var tookMu sync.Mutex
	for _, s := range servers {
		snappers.Add(1)
		go func(s writer) {
			defer snappers.Done()
			for n := range snapshots {
				time.Sleep(150 * time.Millisecond)
				start := time.Now()
				out, err := runKnot(s.server, s.user, "file", "snapshot", fmt.Sprintf("%s:proj%s/", bucket, s.name), fmt.Sprintf("%s:snaps/%s%02d/", bucket, s.name, n))
				if err != nil {
					errs.add("snapshot %d on %s: %v: %s", n, s.name, err, out)
					continue
				}
				tookMu.Lock()
				took[s.name] = append(took[s.name], time.Since(start))
				tookMu.Unlock()
			}
		}(s)
	}
	snappers.Wait()
	close(stop)
	writers.Wait()
	errs.check(t, "writing and snapshotting")
	writesMu.Lock()
	t.Logf("snapshots taken while the writers made %d (A) and %d (B) writes; knot file snapshot took %v (A), %v (B)", writes["A"], writes["B"], took["A"], took["B"])
	writesMu.Unlock()

	read := func(s writer, k string) ([]byte, error) {
		resp, err := s.client().GetFileObject(ctx, bucket, k)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		return io.ReadAll(resp.Body)
	}

	t.Run("each snapshot is of one moment", func(t *testing.T) {
		// Snapshots with a write landing in the time they took, which a copy
		// made over that time would show: the snapshot's newest write is
		// then later than files read before it would hold.
		during := 0
		for _, s := range servers {
			for n := range snapshots {
				dir := fmt.Sprintf("snaps/%s%02d/", s.name, n)
				held := make([]int, files)
				newest := 0
				for i := range files {
					k := dir + fmt.Sprintf("f%03d", i)
					data, err := read(s, k)
					if err != nil {
						t.Fatalf("read %s on %s: %v", k, s.name, err)
					}
					var seq, fi int
					if _, err := fmt.Sscanf(string(data), "seq %d file %d\n", &seq, &fi); err != nil || fi != i || fileOf(seq) != i || !bytes.Equal(data, snapFile(seq, i)) {
						t.Fatalf("%s on %s is not a write made: %.40q", k, s.name, data)
					}
					held[i] = seq
					newest = max(newest, seq)
				}
				for i, seq := range held {
					// File i's last write numbered no more than the newest.
					if want := newest - ((newest-writeOf(i))%files+files)%files; seq != want {
						t.Fatalf("%s is not of one moment: file %d holds write %d, but write %d, to it, came before write %d, which the snapshot holds", dir, i, seq, want, newest)
					}
				}
				if newest >= files {
					during++
				}
				var m apiclient.FileSnapshot
				data, err := read(s, dir+".knot-snapshot.json")
				if err != nil {
					t.Fatalf("manifest of %s on %s: %v", dir, s.name, err)
				}
				if err := json.Unmarshal(data, &m); err != nil || m.Files != files || m.SourcePath != "proj"+s.name || m.TakenAt.IsZero() {
					t.Errorf("manifest of %s: %+v %v", dir, m, err)
				}
			}
		}
		if during == 0 {
			t.Error("no snapshot was taken while its folder was being written")
		}
	})

	t.Run("both servers hold every snapshot", func(t *testing.T) {
		want, err := listShas(ctx, adminA.Client, bucket, "snaps/")
		if err != nil {
			t.Fatal(err)
		}
		if n := len(want); n != 2*snapshots*(files+1) {
			t.Fatalf("server A holds %d snapshot files, want %d", n, 2*snapshots*(files+1))
		}
		took := converge(t, ctx, servers, bucket, "snaps/", want, 2*time.Minute)
		t.Logf("both servers agreed %s after the last snapshot", took.Round(time.Millisecond))
		readAll(t, ctx, servers, bucket, want)
	})

	t.Run("refusals", func(t *testing.T) {
		for _, c := range []struct{ from, to, want string }{
			{bucket + ":projA", bucket + ":snaps/A00", "already exists"},
			{bucket + ":projA", bucket + ":projA/inside", "invalid"},
			{bucket + ":nothing", bucket + ":snaps/none", "not found"},
		} {
			out, err := runKnot(nodeA, adminA, "file", "snapshot", c.from, c.to)
			if err == nil || !strings.Contains(strings.ToLower(out), c.want) {
				t.Errorf("snapshot %s %s: %v: %s, want an error saying %q", c.from, c.to, err, out, c.want)
			}
		}
		if got, _ := listShas(ctx, adminA.Client, bucket, "projA/inside/"); len(got) != 0 {
			t.Errorf("a refused snapshot wrote %d files", len(got))
		}
	})

	checkStorageIntact(t, servers)
}
