//go:build integration

package suites

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	mrand "math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
)

// versionSet is every version written to each of a set of files, with its
// content.
type versionSet struct {
	mu   sync.Mutex
	m    map[string]map[string]bool
	data map[string][]byte
}

// write records a version of key about to be written.
func (v *versionSet) write(key string, b []byte) {
	sha := shaOf(b)
	v.add(key, sha)
	v.mu.Lock()
	if v.data == nil {
		v.data = map[string][]byte{}
	}
	v.data[sha] = b
	v.mu.Unlock()
}

// segmentsOfVersions says whether got is runs of bytes each the same bytes,
// at the same place, of some version of key (zeros past a version's end):
// what a client reads of a file replaced as it reads it. It took the size
// from one version and fetches the bytes in requests, and learns of a newer
// version between them, so what it returns can be one version cut short, one
// followed by zeros, or the start of one and the rest of another, split at
// any read. Runs shorter than segmentMin, but for the last, are not taken
// for one: by chance, short runs match some version anywhere.
func (v *versionSet) segmentsOfVersions(key string, got []byte) bool {
	_, ok := v.segments(key, got)
	return ok
}

const segmentMin = 64

// segments splits got into the longest runs of versions' bytes, from the
// start; ok says it is wholly made of them, else the offset it stops at.
func (v *versionSet) segments(key string, got []byte) (stop int, ok bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	pos := 0
	for pos < len(got) {
		best := 0
		for sha := range v.m[key] {
			d := v.data[sha]
			n := 0
			for pos+n < len(got) {
				var want byte
				if pos+n < len(d) {
					want = d[pos+n]
				}
				if got[pos+n] != want {
					break
				}
				n++
			}
			best = max(best, n)
		}
		if best < segmentMin && pos+best < len(got) {
			return pos, false
		}
		pos += best
	}
	return pos, true
}

// explain describes where got stops being runs of versions' bytes.
func (v *versionSet) explain(key string, got []byte) string {
	stop, ok := v.segments(key, got)
	if ok {
		return "it is runs of versions"
	}
	return fmt.Sprintf("no version has its bytes at %d", stop)
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

func (v *versionSet) add(key, sha string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.m == nil {
		v.m = map[string]map[string]bool{}
	}
	if v.m[key] == nil {
		v.m[key] = map[string]bool{}
	}
	v.m[key][sha] = true
}

func (v *versionSet) has(key, sha string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.m[key][sha]
}

// fileSet is the files a test expects: key -> SHA-256.
type fileSet struct {
	mu sync.Mutex
	m  map[string]string
}

func (f *fileSet) set(key, sha string) {
	f.mu.Lock()
	f.m[key] = sha
	f.mu.Unlock()
}

func (f *fileSet) remove(key string) {
	f.mu.Lock()
	delete(f.m, key)
	f.mu.Unlock()
}

func (f *fileSet) copy() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]string, len(f.m))
	for k, v := range f.m {
		out[k] = v
	}
	return out
}

// nfsClient is a machine with knot's files mounted over NFS.
type nfsClient struct {
	name string
	mnt  string
}

// bigFile is a large file changed in place over NFS, each of its writers
// changing parts of its own: part r is writer r%len(writers)'s.
type bigFile struct {
	key     string
	writers []nfsClient
	// merged says every writer's last change to each part is kept: the
	// writers are clients of one server.
	merged bool
	start  []byte // its content to begin with

	mu      sync.Mutex
	last    []string          // each part's last change
	written []map[string]bool // every version of each part, the first too
	changes atomic.Int64
	// deferred counts changes whose close reported the save not yet made.
	deferred atomic.Int64
}

const (
	bigPart  = 1 << 20
	bigParts = 24 // 24MB: well past the size changed in place
)

func newBigFile(key string, writers []nfsClient, merged bool) *bigFile {
	f := &bigFile{key: key, writers: writers, merged: merged, start: randomBytes(bigParts * bigPart),
		last: make([]string, bigParts), written: make([]map[string]bool, bigParts)}
	for r := range f.last {
		f.last[r] = shaOf(f.start[r*bigPart : (r+1)*bigPart])
		f.written[r] = map[string]bool{f.last[r]: true}
	}
	return f
}

// write has writer wi change its parts in turn, opening, writing and closing
// the file each time, until running says to stop.
func (f *bigFile) write(wi int, bucket string, running func() bool, errs *errorList) {
	c := f.writers[wi]
	p := filepath.Join(c.mnt, bucket, f.key)
	n := len(f.writers)
	for i := 0; running(); i++ {
		r := wi + n*(i%(bigParts/n))
		data := randomBytes(bigPart)
		sha := shaOf(data)
		f.mu.Lock()
		f.written[r][sha] = true
		f.mu.Unlock()
		fh, err := os.OpenFile(p, os.O_WRONLY, 0)
		if err != nil {
			errs.add("open %s on %s: %v", f.key, c.name, err)
			continue
		}
		_, err = fh.WriteAt(data, int64(r*bigPart))
		if cerr := fh.Close(); err == nil {
			err = cerr
		}
		if err != nil && !f.merged && errors.Is(err, syscall.EIO) {
			// Changed through both servers without pause, the save can
			// wait longer than the client does for the other server's
			// version; the nfs-server keeps it and saves it once the file
			// is still, which the checks after see.
			f.deferred.Add(1)
			continue
		}
		if err != nil {
			errs.add("write part %d of %s on %s: %v", r, f.key, c.name, err)
			continue
		}
		f.mu.Lock()
		f.last[r] = sha
		f.mu.Unlock()
		f.changes.Add(1)
	}
}

// check describes what is wrong with data as the file's content (a part
// holding bytes no one wrote there, or, where changes are merged, a part
// without its last change), and counts the parts with their last change.
func (f *bigFile) check(data []byte) (problems []string, kept int) {
	if len(data) != bigParts*bigPart {
		return []string{fmt.Sprintf("size %d, want %d", len(data), bigParts*bigPart)}, 0
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var lost []string
	for r := 0; r < bigParts; r++ {
		sha := shaOf(data[r*bigPart : (r+1)*bigPart])
		switch {
		case sha == f.last[r]:
			kept++
		case !f.written[r][sha]:
			problems = append(problems, fmt.Sprintf("part %d holds bytes no one wrote there", r))
		case f.merged:
			lost = append(lost, fmt.Sprintf("%d (%s's: holds %.8s, last written %.8s)", r, f.writers[r%len(f.writers)].name, sha, f.last[r]))
		}
	}
	if len(lost) > 0 {
		problems = append(problems, "the last change is lost from parts "+strings.Join(lost, ", "))
	}
	return problems, kept
}

// compareBig says how a large file read over NFS at p differs from what
// a server holds: its size and time, and which parts differ.
func compareBig(ctx context.Context, s writer, bucket, key, p string) string {
	local, err := os.ReadFile(p)
	if err != nil {
		return err.Error()
	}
	fi, _ := os.Stat(p)
	resp, err := s.client().GetFileObject(ctx, bucket, key)
	if err != nil {
		return err.Error()
	}
	defer resp.Body.Close()
	remote, err := io.ReadAll(resp.Body)
	if err != nil {
		return err.Error()
	}
	var parts []string
	for r := 0; r*bigPart < max(len(local), len(remote)); r++ {
		a, b := local[min(r*bigPart, len(local)):min((r+1)*bigPart, len(local))], remote[min(r*bigPart, len(remote)):min((r+1)*bigPart, len(remote))]
		if !bytes.Equal(a, b) {
			parts = append(parts, strconv.Itoa(r))
		}
	}
	return fmt.Sprintf("the mount has %d bytes, time %s; the server %d bytes, time %s, sha %.12s; parts differing: %s",
		len(local), fi.ModTime().UTC().Format(time.RFC3339Nano), len(remote), resp.Header.Get(apiclient.FileMtimeHeader), resp.Header.Get("X-Knot-Sha256"), strings.Join(parts, " "))
}

// shaOfFile is the hex SHA-256 of a local file.
func shaOfFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// TestFileSharedFolder uses one folder of a bucket as shared data, written
// and read from everywhere at once: four NFS clients, two on each of two
// clustered servers and each with its own nfs-server, as four machines
// would be, the API on both servers, and knot file cp on both. Each writes
// files of its own there, all of them write the same few files (over NFS
// both in place and by writing a temporary file and renaming it over), the
// NFS clients change their own parts of large files in place, and readers
// read throughout.
//
// It checks that nothing hangs or fails; that readers through the API only
// ever see a whole version of a file, and readers over NFS a whole version
// or, when the file was replaced as they read it, pages of the versions
// written (see segmentsOfVersions), never bytes no version had there, or not
// found or a stale handle (counted); that both servers end with every
// file as last written, the files written by all as one of the versions
// written; that a large file changed by the clients of one server keeps
// every client's last change, and one changed through both servers holds in
// each part a change written there (the last save wins, whole; a close that
// reports its save still to make, the file changing on both servers
// without pause, is counted); that every
// NFS client then sees what the servers hold; and that storage is intact.
func TestFileSharedFolder(t *testing.T) {
	harness.Feature(t, "file-storage")
	skipUnlessNFS(t)
	root := filesRoot(t)
	scale := loadScale(t)
	nodeA, nodeB, adminA, adminB := bootFilePair(t, root, "shareda", "sharedb")
	servers := []writer{{"A", nodeA, adminA}, {"B", nodeB, adminB}}
	ctx, cancel := testCtx(1800)
	defer cancel()
	const bucket = "data"
	if _, err := adminA.Client.CreateFileBucket(ctx, bucket); err != nil {
		t.Fatal(err)
	}
	if !waitForCond(60, func() bool { _, err := adminB.Client.GetFileBucket(ctx, bucket); return err == nil }) {
		t.Fatal("the bucket did not reach server B")
	}
	settle := 2 * time.Minute
	duration := time.Duration(15*scale) * time.Second

	// The files everyone writes exist before anyone starts.
	const contested = 8
	var versions versionSet
	sameKey := func(k int) string { return fmt.Sprintf("same/k%d", k) }
	for k := 0; k < contested; k++ {
		data := randomBytes(fileSize(k))
		versions.write(sameKey(k), data)
		if _, err := adminA.Client.PutFileObject(ctx, bucket, sameKey(k), bytes.NewReader(data), int64(len(data)), "", apiclient.FileAttrs{}); err != nil {
			t.Fatal(err)
		}
	}

	// Four NFS clients, each with its own nfs-server.
	var clients []nfsClient
	mounts := map[string]string{}
	for _, s := range servers {
		for i := 1; i <= 2; i++ {
			name := fmt.Sprintf("%s%d", s.name, i)
			mnt := mountNFSAt(t, startNFS(t, s.server, s.user), filepath.Join(root, "shared-mnt-"+strings.ToLower(name)))
			clients = append(clients, nfsClient{name, mnt})
			mounts[name] = mnt
		}
	}
	// The folders written into exist before readers list them.
	for _, c := range clients {
		for _, dir := range []string{"tmp", "own"} {
			if err := os.MkdirAll(filepath.Join(c.mnt, bucket, dir), 0755); err != nil {
				t.Fatalf("mkdir %s on %s: %v", dir, c.name, err)
			}
		}
	}

	// Large files changed in place: one by the two clients of each server,
	// whose changes to different parts are all kept, and one by all four,
	// which across servers ends as one server's version or the other's.
	bigs := []*bigFile{
		newBigFile("bigA.bin", clients[:2], true),
		newBigFile("bigB.bin", clients[2:], true),
		newBigFile("big.bin", clients, false),
	}
	for _, f := range bigs {
		if _, err := adminA.Client.PutFileObject(ctx, bucket, f.key, bytes.NewReader(f.start), int64(len(f.start)), "", apiclient.FileAttrs{}); err != nil {
			t.Fatal(err)
		}
	}
	if !waitForCond(60, func() bool {
		got, err := listShas(ctx, adminB.Client, bucket, "")
		if err != nil {
			return false
		}
		for _, f := range bigs {
			if got[f.key] != shaOf(f.start) {
				return false
			}
		}
		for k := 0; k < contested; k++ {
			if got[sameKey(k)] == "" {
				return false
			}
		}
		return true
	}) {
		t.Fatal("the starting files did not reach server B")
	}

	own := fileSet{m: map[string]string{}}
	var errs errorList
	var nfsWrites, apiWrites, cpWrites, renames atomic.Int64
	var apiReads, nfsReads, nfsTorn, nfsMissing, replacedWriting atomic.Int64
	var wg sync.WaitGroup
	stop := time.Now().Add(duration)
	running := func() bool { return time.Now().Before(stop) && ctx.Err() == nil }

	// Files of their own, written, rewritten and some deleted, over NFS.
	for _, c := range clients {
		for w := 0; w < 2; w++ {
			wg.Add(1)
			go func(c nfsClient, w int) {
				defer wg.Done()
				sub := fmt.Sprintf("own/%s-%d", c.name, w)
				dir := filepath.Join(c.mnt, bucket, filepath.FromSlash(sub))
				if err := os.MkdirAll(dir, 0755); err != nil {
					errs.add("mkdir on %s: %v", c.name, err)
					return
				}
				have := map[int]bool{}
				for i := 0; running(); i++ {
					n := i % 30
					key := fmt.Sprintf("%s/f%03d", sub, n)
					p := filepath.Join(dir, fmt.Sprintf("f%03d", n))
					if i%7 == 6 && have[n] {
						if err := os.Remove(p); err != nil {
							errs.add("remove %s on %s: %v", key, c.name, err)
							continue
						}
						delete(have, n)
						own.remove(key)
						continue
					}
					data := randomBytes(fileSize(i))
					if err := os.WriteFile(p, data, 0644); err != nil {
						errs.add("write %s on %s: %v", key, c.name, err)
						continue
					}
					have[n] = true
					own.set(key, shaOf(data))
					nfsWrites.Add(1)
				}
			}(c, w)
		}
	}

	// Files of their own through the API.
	for _, s := range servers {
		for w := 0; w < 2; w++ {
			wg.Add(1)
			go func(s writer, w int) {
				defer wg.Done()
				have := map[int]bool{}
				for i := 0; running(); i++ {
					n := i % 30
					key := fmt.Sprintf("own/api%s-%d/f%03d", s.name, w, n)
					if i%7 == 6 && have[n] {
						if err := s.client().DeleteFileObject(ctx, bucket, key); err != nil {
							errs.add("delete %s on %s: %v", key, s.name, err)
							continue
						}
						delete(have, n)
						own.remove(key)
						continue
					}
					data := randomBytes(fileSize(i))
					if _, err := s.client().PutFileObject(ctx, bucket, key, bytes.NewReader(data), int64(len(data)), "", apiclient.FileAttrs{}); err != nil {
						errs.add("put %s on %s: %v", key, s.name, err)
						continue
					}
					have[n] = true
					own.set(key, shaOf(data))
					apiWrites.Add(1)
				}
			}(s, w)
		}
	}

	// knot file cp, of files of its own and of the files everyone writes.
	for _, s := range servers {
		wg.Add(1)
		go func(s writer) {
			defer wg.Done()
			local := t.TempDir()
			for i := 0; running(); i++ {
				data := randomBytes(fileSize(i))
				p := filepath.Join(local, fmt.Sprintf("c%d", i))
				if err := os.WriteFile(p, data, 0644); err != nil {
					errs.add("write a local file: %v", err)
					return
				}
				var key string
				if i%3 == 2 {
					key = sameKey(mrand.IntN(contested))
					versions.write(key, data)
				} else {
					key = fmt.Sprintf("own/cp%s/f%03d", s.name, i%20)
				}
				if out, err := runKnot(s.server, s.user, "file", "cp", p, bucket+":"+key); err != nil {
					errs.add("cp %s on %s: %v: %s", key, s.name, err, strings.TrimSpace(out))
					continue
				}
				if !strings.HasPrefix(key, "same/") {
					own.set(key, shaOf(data))
				}
				cpWrites.Add(1)
				os.Remove(p)
			}
		}(s)
	}

	// The files everyone writes, through the API.
	for _, s := range servers {
		for w := 0; w < 2; w++ {
			wg.Add(1)
			go func(s writer) {
				defer wg.Done()
				for running() {
					key := sameKey(mrand.IntN(contested))
					data := randomBytes(fileSize(mrand.IntN(10)))
					versions.write(key, data)
					if _, err := s.client().PutFileObject(ctx, bucket, key, bytes.NewReader(data), int64(len(data)), "", apiclient.FileAttrs{}); err != nil {
						errs.add("put %s on %s: %v", key, s.name, err)
						continue
					}
					apiWrites.Add(1)
				}
			}(s)
		}
	}

	// The files everyone writes, over NFS from the first client on each
	// server: in place, and by a temporary file renamed over.
	for _, c := range clients {
		if !strings.HasSuffix(c.name, "1") {
			continue
		}
		wg.Add(1)
		go func(c nfsClient) {
			defer wg.Done()
			for i := 0; running(); i++ {
				key := sameKey(mrand.IntN(contested))
				dst := filepath.Join(c.mnt, bucket, filepath.FromSlash(key))
				data := randomBytes(fileSize(mrand.IntN(10)))
				versions.write(key, data)
				if i%2 == 0 {
					if err := os.WriteFile(dst, data, 0644); errors.Is(err, syscall.ESTALE) {
						// Another client renamed a file over this one as
						// it was written, as with any NFS server.
						replacedWriting.Add(1)
						continue
					} else if err != nil {
						errs.add("write %s on %s: %v", key, c.name, err)
						continue
					}
					nfsWrites.Add(1)
					continue
				}
				tmp := filepath.Join(c.mnt, bucket, "tmp", fmt.Sprintf("%s-%d", c.name, i))
				if err := os.WriteFile(tmp, data, 0644); err != nil {
					errs.add("write %s on %s: %v", tmp, c.name, err)
					continue
				}
				if err := os.Rename(tmp, dst); err != nil {
					errs.add("rename over %s on %s: %v", key, c.name, err)
					os.Remove(tmp)
					continue
				}
				renames.Add(1)
			}
		}(c)
	}

	// Every NFS client changes its own parts of the large files in place.
	for _, f := range bigs {
		for wi := range f.writers {
			wg.Add(1)
			go func(f *bigFile, wi int) {
				defer wg.Done()
				f.write(wi, bucket, running, &errs)
			}(f, wi)
		}
	}

	// Readers through the API: always a whole version, never missing.
	for _, s := range servers {
		for w := 0; w < 2; w++ {
			wg.Add(1)
			go func(s writer) {
				defer wg.Done()
				for running() {
					key := sameKey(mrand.IntN(contested))
					resp, err := s.client().GetFileObject(ctx, bucket, key)
					if err != nil {
						errs.add("read %s on %s: %v", key, s.name, err)
						continue
					}
					data, err := io.ReadAll(resp.Body)
					resp.Body.Close()
					if err != nil {
						errs.add("read %s on %s: %v", key, s.name, err)
						continue
					}
					if got := shaOf(data); !versions.has(key, got) {
						said := resp.Header.Get("X-Knot-Sha256")
						errs.add("read %s on %s: content no one wrote, %d bytes (runs of versions: %v), said to be %s (written: %v, size %s): %s",
							key, s.name, len(data), versions.segmentsOfVersions(key, data), said, versions.has(key, said), resp.Header.Get("Content-Length"), versions.explain(key, data))
					}
					apiReads.Add(1)
				}
			}(s)
		}
	}

	// Readers over NFS on the second client of each server, which writes
	// none of the files everyone writes, listing the folder as they go.
	for _, c := range clients {
		if !strings.HasSuffix(c.name, "2") {
			continue
		}
		wg.Add(1)
		go func(c nfsClient) {
			defer wg.Done()
			for i := 0; running(); i++ {
				key := sameKey(mrand.IntN(contested))
				data, err := os.ReadFile(filepath.Join(c.mnt, bucket, filepath.FromSlash(key)))
				switch {
				case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ESTALE):
					// Replaced as it was opened, or renamed over as it
					// was read: an NFS client's file handle is then of
					// a file gone, as with any NFS server.
					nfsMissing.Add(1)
				case err != nil:
					errs.add("read %s on %s: %v", key, c.name, err)
				case versions.has(key, shaOf(data)):
				case versions.segmentsOfVersions(key, data):
					nfsTorn.Add(1)
				default:
					errs.add("read %s on %s: %d bytes, not runs of versions written: %s", key, c.name, len(data), versions.explain(key, data))
				}
				nfsReads.Add(1)
				if i%10 == 0 {
					if _, err := os.ReadDir(filepath.Join(c.mnt, bucket, "same")); err != nil {
						errs.add("list same/ on %s: %v", c.name, err)
					}
					if _, err := os.ReadDir(filepath.Join(c.mnt, bucket, "own")); err != nil {
						errs.add("list own/ on %s: %v", c.name, err)
					}
				}
			}
		}(c)
	}

	nfsWatchdog(t, &wg, duration+3*time.Minute, servers, mounts)
	errs.check(t, "writes and reads")
	var bigWrites int64
	for _, f := range bigs {
		bigWrites += f.changes.Load()
	}
	t.Logf("in %s: %d NFS writes, %d renames over, %d API writes, %d by cp, %d changes to large files in place; %d API reads, %d NFS reads",
		duration, nfsWrites.Load(), renames.Load(), apiWrites.Load(), cpWrites.Load(), bigWrites, apiReads.Load(), nfsReads.Load())
	t.Logf("NFS reads of a file as it was replaced: %d of more than one version, or cut short or padded, %d not found or stale; %d NFS writes stale, the file renamed over", nfsTorn.Load(), nfsMissing.Load(), replacedWriting.Load())

	// Both servers have every file of its own as last written.
	wantOwn := own.copy()
	if got, err := listShas(ctx, adminA.Client, bucket, "own/"); err == nil {
		for k, v := range got {
			if strings.HasSuffix(k, "/") {
				wantOwn[k] = v // a folder made over NFS
			}
		}
	}
	t.Logf("both servers agreed on the files of their own %s after the last write",
		converge(t, ctx, servers, bucket, "own/", wantOwn, settle).Round(time.Millisecond))
	ownFiles := map[string]string{}
	for k, v := range wantOwn {
		if !strings.HasSuffix(k, "/") {
			ownFiles[k] = v
		}
	}
	readAll(t, ctx, servers, bucket, ownFiles)

	// Both servers agree on the files everyone wrote, each one of the
	// versions written.
	final := map[string]string{}
	if !waitForCond(int(settle.Seconds()), func() bool {
		a, errA := listShas(ctx, adminA.Client, bucket, "same/")
		b, errB := listShas(ctx, adminB.Client, bucket, "same/")
		if errA != nil || errB != nil || diffShas(a, b) != "" {
			return false
		}
		final = map[string]string{}
		for k, v := range a {
			if !strings.HasSuffix(k, "/") {
				final[k] = v
			}
		}
		return len(final) == contested
	}) {
		a, _ := listShas(ctx, adminA.Client, bucket, "same/")
		b, _ := listShas(ctx, adminB.Client, bucket, "same/")
		t.Fatalf("the servers did not agree on the files everyone wrote: %s", diffShas(a, b))
	}
	for k, sha := range final {
		if !versions.has(k, sha) {
			t.Errorf("%s holds content no one wrote: %.12s", k, sha)
		}
	}
	readAll(t, ctx, servers, bucket, final)
	if got, err := listShas(ctx, adminA.Client, bucket, "tmp/"); err == nil {
		for k := range got {
			if !strings.HasSuffix(k, "/") {
				t.Errorf("a temporary file renamed over another is still there: %s", k)
			}
		}
	}

	// Both servers agree on each large file, every part of it holding a
	// change written there, and the last change to every part where only
	// one server's clients changed it.
	// A save deferred while the file kept changing lands once it is still,
	// so a file is taken as final once both servers have held the same
	// version of it for a while.
	finalBig := map[string]string{}
	for _, f := range bigs {
		var since time.Time
		if !waitForCond(int(settle.Seconds()), func() bool {
			a, errA := listShas(ctx, adminA.Client, bucket, f.key)
			b, errB := listShas(ctx, adminB.Client, bucket, f.key)
			if errA != nil || errB != nil || a[f.key] == "" || a[f.key] != b[f.key] {
				since = time.Time{}
				return false
			}
			if a[f.key] != finalBig[f.key] || since.IsZero() {
				finalBig[f.key], since = a[f.key], time.Now()
			}
			return time.Since(since) > 6*time.Second
		}) {
			t.Fatalf("the servers did not agree on %s", f.key)
		}
		for _, s := range servers {
			resp, err := s.client().GetFileObject(ctx, bucket, f.key)
			if err != nil {
				t.Fatalf("read %s on %s: %v", f.key, s.name, err)
			}
			data, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				t.Fatalf("read %s on %s: %v", f.key, s.name, err)
			}
			problems, kept := f.check(data)
			for _, p := range problems {
				t.Errorf("%s on %s: %s", f.key, s.name, p)
			}
			if !f.merged && s.name == "A" {
				t.Logf("%s, changed by clients of both servers: %d of its %d parts hold their last change, the rest an earlier one (the last save wins); %d closes reported the save still to make", f.key, kept, bigParts, f.deferred.Load())
			}
		}
	}

	// Every NFS client sees the same, once its caches have caught up.
	for _, c := range clients {
		var problem, first string
		start := time.Now()
		if !waitForCond(90, func() bool {
			if first == "" {
				first = problem
			}
			problem = ""
			want := map[string]string{}
			for k, v := range ownFiles {
				want[k] = v
			}
			for k, v := range final {
				want[k] = v
			}
			// What it lists is what the servers hold.
			listed := map[string]bool{}
			for _, dir := range []string{"own", "same"} {
				filepath.WalkDir(filepath.Join(c.mnt, bucket, dir), func(p string, d fs.DirEntry, err error) error {
					if err != nil || d.IsDir() || strings.HasPrefix(d.Name(), "._") {
						return nil
					}
					rel, _ := filepath.Rel(filepath.Join(c.mnt, bucket), p)
					listed[filepath.ToSlash(rel)] = true
					return nil
				})
			}
			var diff []string
			for k := range want {
				if !listed[k] {
					diff = append(diff, "missing "+k)
				}
			}
			for k := range listed {
				if _, ok := want[k]; !ok {
					diff = append(diff, "extra "+k)
				}
			}
			if len(diff) > 0 {
				sort.Strings(diff)
				if len(diff) > 6 {
					diff = append(diff[:6], fmt.Sprintf("and %d more", len(diff)-6))
				}
				problem = "lists " + strings.Join(diff, "; ")
				return false
			}
			// What it reads is what the servers hold.
			for k, v := range want {
				got, err := shaOfFile(filepath.Join(c.mnt, bucket, filepath.FromSlash(k)))
				if err != nil || got != v {
					problem = fmt.Sprintf("reads %s as other than written: %v", k, err)
					return false
				}
			}
			for k, v := range finalBig {
				p := filepath.Join(c.mnt, bucket, k)
				got, err := shaOfFile(p)
				if err != nil || got != v {
					problem = fmt.Sprintf("reads %s as other than the servers hold it: %v; %s", k, err, compareBig(ctx, servers[0], bucket, k, p))
					return false
				}
			}
			return true
		}) {
			t.Errorf("NFS client %s did not catch up: it %s", c.name, problem)
			continue
		}
		took := time.Since(start).Round(time.Millisecond)
		if first != "" && took > 2*time.Second {
			t.Logf("NFS client %s matched the servers %s after they agreed; until then it %s", c.name, took, first)
		} else {
			t.Logf("NFS client %s matched the servers %s after they agreed", c.name, took)
		}
	}

	checkStorageIntact(t, servers)
}
