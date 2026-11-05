//go:build integration

package suites

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	mrand "math/rand/v2"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
)

// loadScale is how much work the multi-writer test does: KNOT_TEST_LOAD
// multiplies the writers and the time spent writing (1 when unset).
func loadScale(t *testing.T) int {
	v := os.Getenv("KNOT_TEST_LOAD")
	if v == "" {
		return 1
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		t.Fatalf("KNOT_TEST_LOAD=%q: want a whole number of at least 1", v)
	}
	return n
}

// bootFilePair starts two clustered servers, each with its own database and
// file storage, with the same administrator on both.
func bootFilePair(t *testing.T, root, nameA, nameB string) (a, b *harness.Server, adminA, adminB *harness.User) {
	t.Helper()
	key := "0123456789abcdef0123456789abcdef"
	bindA, bindB := freePorts(t, 2)
	a, err := harness.StartServer(cfg, bins, nameA, "--files-path", filepath.Join(root, nameA),
		"--cluster-key", key,
		"--cluster-bind-addr", fmt.Sprintf("127.0.0.1:%d", bindA),
		"--cluster-advertise-addr", fmt.Sprintf("127.0.0.1:%d", bindA),
		"--cluster-peer", fmt.Sprintf("127.0.0.1:%d", bindB))
	if err != nil {
		t.Fatalf("boot server A: %v", err)
	}
	t.Cleanup(a.Stop)
	b, err = harness.StartServer(cfg, bins, nameB, "--files-path", filepath.Join(root, nameB),
		"--cluster-key", key,
		"--cluster-bind-addr", fmt.Sprintf("127.0.0.1:%d", bindB),
		"--cluster-advertise-addr", fmt.Sprintf("127.0.0.1:%d", bindB),
		"--cluster-peer", fmt.Sprintf("127.0.0.1:%d", bindA))
	if err != nil {
		t.Fatalf("boot server B: %v", err)
	}
	t.Cleanup(b.Stop)
	if adminA, err = harness.ProvisionAdmin(a, "admin", "AdminPassw0rd!"); err != nil {
		t.Fatal(err)
	}
	if !waitForCond(60, func() bool {
		adminB, err = harness.LoginUser(b, "admin", "AdminPassw0rd!")
		return err == nil
	}) {
		t.Fatalf("the admin did not reach server B: %v", err)
	}
	return a, b, adminA, adminB
}

// writer is one server's API, as a client of it.
type writer struct {
	name   string
	server *harness.Server
	user   *harness.User
}

func (w writer) client() *apiclient.ApiClient { return w.user.Client }

// shaOf is the hex SHA-256 of content.
func shaOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// randomBytes is size bytes no other write will repeat.
func randomBytes(size int) []byte {
	b := make([]byte, size)
	rand.Read(b)
	return b
}

// fileSize picks a size: mostly small files, kept inline in the database,
// and some larger than that, kept as files.
func fileSize(i int) int {
	if i%5 == 0 {
		return 100<<10 + mrand.IntN(400<<10)
	}
	return 1 + mrand.IntN(8<<10)
}

// listShas lists a bucket folder on one server: key -> SHA-256. macOS's
// AppleDouble files (._name), which it writes over NFS beside each file to
// hold its extended attributes, are left out: they are not what was written.
func listShas(ctx context.Context, c *apiclient.ApiClient, bucket, prefix string) (map[string]string, error) {
	out := map[string]string{}
	after := ""
	for {
		page, err := c.ListFileObjects(ctx, bucket, prefix, "", after, 1000)
		if err != nil {
			return nil, err
		}
		for _, o := range page.Objects {
			if strings.HasPrefix(path.Base(o.Key), "._") {
				continue
			}
			out[o.Key] = o.SHA256
		}
		if !page.IsTruncated || page.Next == "" {
			return out, nil
		}
		after = page.Next
	}
}

// readSha reads a file from one server and returns the SHA-256 of what it
// holds.
func readSha(ctx context.Context, c *apiclient.ApiClient, bucket, key string) (string, error) {
	resp, err := c.GetFileObject(ctx, bucket, key)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	h := sha256.New()
	if _, err := io.Copy(h, resp.Body); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// diffShas describes how got differs from want, at most a few lines.
func diffShas(got, want map[string]string) string {
	var lines []string
	for k, w := range want {
		if g, ok := got[k]; !ok {
			lines = append(lines, "missing "+k)
		} else if g != w {
			lines = append(lines, "different "+k)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			lines = append(lines, "extra "+k)
		}
	}
	sort.Strings(lines)
	n := len(lines)
	if n > 8 {
		lines = append(lines[:8], fmt.Sprintf("and %d more", n-8))
	}
	return strings.Join(lines, "; ")
}

// converge waits until both servers list exactly want under prefix, and
// reports how long it took.
func converge(t *testing.T, ctx context.Context, servers []writer, bucket, prefix string, want map[string]string, timeout time.Duration) time.Duration {
	t.Helper()
	start := time.Now()
	var last string
	for time.Since(start) < timeout {
		last = ""
		for _, s := range servers {
			got, err := listShas(ctx, s.client(), bucket, prefix)
			if err != nil {
				last = fmt.Sprintf("server %s: %v", s.name, err)
				break
			}
			if d := diffShas(got, want); d != "" {
				last = fmt.Sprintf("server %s: %s", s.name, d)
				break
			}
		}
		if last == "" {
			return time.Since(start)
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("the servers did not agree on %s within %s: %s", prefix, timeout, last)
	return 0
}

// readAll reads every file in want from every server, checking each holds
// the content listed.
func readAll(t *testing.T, ctx context.Context, servers []writer, bucket string, want map[string]string) {
	t.Helper()
	type job struct {
		s   writer
		key string
	}
	jobs := make(chan job)
	var mu sync.Mutex
	var bad []string
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				got, err := readSha(ctx, j.s.client(), bucket, j.key)
				if err != nil || got != want[j.key] {
					mu.Lock()
					bad = append(bad, fmt.Sprintf("%s on %s: %v", j.key, j.s.name, err))
					mu.Unlock()
				}
			}
		}()
	}
	for k := range want {
		for _, s := range servers {
			jobs <- job{s, k}
		}
	}
	close(jobs)
	wg.Wait()
	if len(bad) > 0 {
		sort.Strings(bad)
		if len(bad) > 8 {
			bad = append(bad[:8], fmt.Sprintf("and %d more", len(bad)-8))
		}
		t.Errorf("files that do not read back as written: %s", strings.Join(bad, "; "))
	}
}

// errorList collects the errors of many writers.
type errorList struct {
	mu   sync.Mutex
	errs []string
}

func (e *errorList) add(format string, a ...any) {
	e.mu.Lock()
	e.errs = append(e.errs, fmt.Sprintf(format, a...))
	e.mu.Unlock()
}

func (e *errorList) check(t *testing.T, what string) {
	t.Helper()
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.errs) == 0 {
		return
	}
	n := len(e.errs)
	shown := e.errs
	if n > 8 {
		shown = shown[:8]
	}
	t.Errorf("%d of the %s failed: %s", n, what, strings.Join(shown, "; "))
}

// TestFileMultiWriter writes to one bucket from many writers on two
// clustered servers at once, each server with its own database and storage,
// and checks that nothing is lost or damaged: files written separately all
// arrive on both servers with their content, files written by many at once
// end the same on both, as one of the versions written (the last write
// wins), files sharing content keep it while any of them still holds it,
// and the same holds for writes over NFS to both servers. KNOT_TEST_LOAD
// scales the load.
func TestFileMultiWriter(t *testing.T) {
	harness.Feature(t, "file-storage")
	root := filesRoot(t)
	scale := loadScale(t)
	nodeA, nodeB, adminA, adminB := bootFilePair(t, root, "loada", "loadb")
	servers := []writer{{"A", nodeA, adminA}, {"B", nodeB, adminB}}
	ctx, cancel := testCtx(1800)
	defer cancel()
	if _, err := adminA.Client.CreateFileBucket(ctx, "load"); err != nil {
		t.Fatal(err)
	}
	if !waitForCond(60, func() bool { _, err := adminB.Client.GetFileBucket(ctx, "load"); return err == nil }) {
		t.Fatal("the bucket did not reach server B")
	}
	writersPerServer := 6 * scale
	settle := 2 * time.Minute

	// Each writer writes files of its own, rewrites some and deletes some.
	t.Run("files of their own", func(t *testing.T) {
		const files = 40
		var mu sync.Mutex
		want := map[string]string{}
		var errs errorList
		var ops, bytesOut atomic.Int64
		var wg sync.WaitGroup
		start := time.Now()
		for _, s := range servers {
			for w := 0; w < writersPerServer; w++ {
				wg.Add(1)
				go func(s writer, w int) {
					defer wg.Done()
					put := func(key string, size int) {
						data := randomBytes(size)
						if _, err := s.client().PutFileObject(ctx, "load", key, bytes.NewReader(data), int64(len(data)), "", apiclient.FileAttrs{}); err != nil {
							errs.add("put %s on %s: %v", key, s.name, err)
							return
						}
						ops.Add(1)
						bytesOut.Add(int64(len(data)))
						mu.Lock()
						want[key] = shaOf(data)
						mu.Unlock()
					}
					for i := 0; i < files; i++ {
						put(fmt.Sprintf("own/%s%02d/f%03d", s.name, w, i), fileSize(i))
					}
					for i := 0; i < files; i += 3 {
						put(fmt.Sprintf("own/%s%02d/f%03d", s.name, w, i), fileSize(i+1))
					}
					for i := 1; i < files; i += 7 {
						key := fmt.Sprintf("own/%s%02d/f%03d", s.name, w, i)
						if err := s.client().DeleteFileObject(ctx, "load", key); err != nil {
							errs.add("delete %s on %s: %v", key, s.name, err)
							continue
						}
						ops.Add(1)
						mu.Lock()
						delete(want, key)
						mu.Unlock()
					}
				}(s, w)
			}
		}
		wg.Wait()
		took := time.Since(start)
		errs.check(t, "writes")
		t.Logf("%d writers made %d changes, %s, in %s (%.0f a second)", 2*writersPerServer, ops.Load(),
			formatSize(bytesOut.Load()), took.Round(time.Millisecond), float64(ops.Load())/took.Seconds())
		t.Logf("both servers agreed %s after the last write", converge(t, ctx, servers, "load", "own/", want, settle).Round(time.Millisecond))
		readAll(t, ctx, servers, "load", want)
	})

	// Every writer on both servers writes the same few files for a while.
	t.Run("the same files at once", func(t *testing.T) {
		const keys = 8
		duration := time.Duration(10*scale) * time.Second
		written := make([]map[string]bool, keys)
		for i := range written {
			written[i] = map[string]bool{}
		}
		var mu sync.Mutex
		var errs errorList
		var ops atomic.Int64
		var wg sync.WaitGroup
		stop := time.Now().Add(duration)
		for _, s := range servers {
			for w := 0; w < writersPerServer; w++ {
				wg.Add(1)
				go func(s writer) {
					defer wg.Done()
					for time.Now().Before(stop) {
						k := mrand.IntN(keys)
						data := randomBytes(fileSize(mrand.IntN(10)))
						key := fmt.Sprintf("same/k%d", k)
						mu.Lock()
						written[k][shaOf(data)] = true
						mu.Unlock()
						if _, err := s.client().PutFileObject(ctx, "load", key, bytes.NewReader(data), int64(len(data)), "", apiclient.FileAttrs{}); err != nil {
							errs.add("put %s on %s: %v", key, s.name, err)
							continue
						}
						ops.Add(1)
					}
				}(s)
			}
		}
		wg.Wait()
		errs.check(t, "writes")
		t.Logf("%d writers made %d writes to %d files in %s", 2*writersPerServer, ops.Load(), keys, duration)

		// Both servers end with the same version of each file, one of those
		// written, which reads back whole.
		var final map[string]string
		start := time.Now()
		if !waitForCond(int(settle.Seconds()), func() bool {
			a, errA := listShas(ctx, adminA.Client, "load", "same/")
			b, errB := listShas(ctx, adminB.Client, "load", "same/")
			if errA != nil || errB != nil || len(a) != keys || diffShas(a, b) != "" {
				return false
			}
			final = a
			return true
		}) {
			a, _ := listShas(ctx, adminA.Client, "load", "same/")
			b, _ := listShas(ctx, adminB.Client, "load", "same/")
			t.Fatalf("the servers did not agree on the files written at once: %s", diffShas(a, b))
		}
		t.Logf("both servers agreed %s after the last write", time.Since(start).Round(time.Millisecond))
		for k := 0; k < keys; k++ {
			key := fmt.Sprintf("same/k%d", k)
			if !written[k][final[key]] {
				t.Errorf("%s holds content no writer wrote", key)
			}
		}
		readAll(t, ctx, servers, "load", final)
	})

	// Files sharing their content, empty ones too, written on both servers
	// while the other deletes some: what is left keeps its content.
	t.Run("shared content", func(t *testing.T) {
		contents := [][]byte{nil, []byte("the same licence text\n"), randomBytes(200 << 10)}
		const each = 30
		var mu sync.Mutex
		want := map[string]string{}
		var errs errorList
		var wg sync.WaitGroup
		for si, s := range servers {
			other := servers[1-si]
			for w := 0; w < writersPerServer; w++ {
				wg.Add(1)
				go func(s, other writer, w int) {
					defer wg.Done()
					for i := 0; i < each; i++ {
						c := contents[i%len(contents)]
						key := fmt.Sprintf("shared/%s%02d/f%03d", s.name, w, i)
						if _, err := s.client().PutFileObject(ctx, "load", key, bytes.NewReader(c), int64(len(c)), "", apiclient.FileAttrs{}); err != nil {
							errs.add("put %s on %s: %v", key, s.name, err)
							continue
						}
						mu.Lock()
						want[key] = shaOf(c)
						mu.Unlock()
						// The other server deletes every fourth, once it has it.
						if i%4 == 3 {
							if waitForCond(30, func() bool { _, err := other.client().HeadFileObject(ctx, "load", key); return err == nil }) {
								if err := other.client().DeleteFileObject(ctx, "load", key); err != nil {
									errs.add("delete %s on %s: %v", key, other.name, err)
									continue
								}
								mu.Lock()
								delete(want, key)
								mu.Unlock()
							} else {
								errs.add("%s did not reach %s", key, other.name)
							}
						}
					}
				}(s, other, w)
			}
		}
		wg.Wait()
		errs.check(t, "writes and deletes")
		t.Logf("both servers agreed %s after the last write", converge(t, ctx, servers, "load", "shared/", want, settle).Round(time.Millisecond))
		readAll(t, ctx, servers, "load", want)
	})

	// Over NFS, many writers make the same new folders at once, as several
	// processes making one tree do, and every one finishes. macOS keeps an
	// AppleDouble "._" file beside each new folder and locks it; with
	// processes racing, one is removed while open and locked, renamed to
	// ".nfs...", and its locks there were refused, which hung the mount.
	t.Run("NFS folders made at once", func(t *testing.T) {
		mnt := mountNFS(t, nodeA, adminA, filepath.Join(root, "race-mnt"))
		const rounds = 20
		want := map[string]string{}
		var errs errorList
		var wg sync.WaitGroup
		for r := 0; r < rounds; r++ {
			for g := 0; g < 8*scale; g++ {
				want[fmt.Sprintf("race/r%02d/g%02d/", r, g)] = ""
			}
		}
		for g := 0; g < 8*scale; g++ {
			wg.Add(1)
			go func(g int) {
				defer wg.Done()
				for r := 0; r < rounds; r++ {
					// The round's folder is new: every writer makes it.
					if err := os.MkdirAll(filepath.Join(mnt, "load", "race", fmt.Sprintf("r%02d", r), fmt.Sprintf("g%02d", g)), 0755); err != nil {
						errs.add("mkdir: %v", err)
					}
				}
			}(g)
		}
		nfsWatchdog(t, &wg, time.Minute, servers, map[string]string{"A": mnt})
		errs.check(t, "folders made over NFS")
		got, err := listShas(ctx, adminA.Client, "load", "race/")
		if err != nil {
			t.Fatal(err)
		}
		for k := range want {
			if _, ok := got[k]; !ok {
				t.Errorf("folder %s made over NFS is not stored", k)
			}
		}
	})

	// The nfs-server restarts, as a crash or an upgrade has it, while a file
	// is open and locked over it: the client takes back its open and its
	// lock, and the mount goes on working. Told the server had no state to
	// take back, the Mac's client tried again every second, forever, and
	// every process using the mount hung.
	t.Run("NFS server restarted", func(t *testing.T) {
		mnt, restart := mountNFSRestartable(t, nodeA, adminA, filepath.Join(root, "restart-mnt"))
		dir := filepath.Join(mnt, "load", "restart")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		held, err := os.OpenFile(filepath.Join(dir, "held"), os.O_CREATE|os.O_RDWR, 0644)
		if err != nil {
			t.Fatal(err)
		}
		defer held.Close()
		if _, err := held.WriteString("before "); err != nil {
			t.Fatal(err)
		}
		// Saved: what is written and not yet saved is lost with the
		// nfs-server, as a local file's is in a crash before fsync.
		if err := held.Sync(); err != nil {
			t.Fatal(err)
		}
		if err := lockFile(held, true, true); err != nil {
			t.Fatal(err)
		}

		restart()

		var errs errorList
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := held.WriteString("after"); err != nil {
				errs.add("write to the file held open: %v", err)
			}
			if err := held.Sync(); err != nil {
				errs.add("sync the file held open: %v", err)
			}
			if err := unlockFile(held); err != nil {
				errs.add("unlock: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "new"), []byte("new"), 0644); err != nil {
				errs.add("write a new file: %v", err)
			}
			// The lock is free for another opener. Open for writing: over
			// NFS, Linux takes an flock as a POSIX lock, and an exclusive
			// one needs a file open for writing.
			other, err := os.OpenFile(filepath.Join(dir, "held"), os.O_RDWR, 0)
			if err != nil {
				errs.add("open again: %v", err)
				return
			}
			defer other.Close()
			if err := lockFile(other, true, false); err != nil {
				errs.add("lock again after unlocking: %v", err)
			}
		}()
		nfsWatchdog(t, &wg, 90*time.Second, servers, map[string]string{"A": mnt})
		errs.check(t, "after the nfs-server restarted")
		if err := held.Close(); err != nil {
			t.Errorf("close the file held open: %v", err)
		}
		if got, err := os.ReadFile(filepath.Join(dir, "held")); err != nil || string(got) != "before after" {
			t.Errorf("the file held open reads %q, %v; want %q", got, err, "before after")
		}
	})

	// Over NFS, mounted from each server, with writers on both mounts, of
	// files of their own and of the same files.
	t.Run("NFS on both servers", func(t *testing.T) {
		mounts := map[string]string{
			"A": mountNFS(t, nodeA, adminA, filepath.Join(root, "load-mnt-a")),
			"B": mountNFS(t, nodeB, adminB, filepath.Join(root, "load-mnt-b")),
		}
		const files = 20
		var mu sync.Mutex
		want := map[string]string{}
		var errs errorList
		var wg sync.WaitGroup
		for name, mnt := range mounts {
			for w := 0; w < 3*scale; w++ {
				wg.Add(1)
				go func(name, mnt string, w int) {
					defer wg.Done()
					dir := filepath.Join(mnt, "load", "nfs", fmt.Sprintf("%s%02d", name, w))
					if err := os.MkdirAll(dir, 0755); err != nil {
						errs.add("mkdir on %s: %v", name, err)
						return
					}
					for i := 0; i < files; i++ {
						data := randomBytes(fileSize(i))
						if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%03d", i)), data, 0644); err != nil {
							errs.add("write on %s: %v", name, err)
							continue
						}
						mu.Lock()
						want[fmt.Sprintf("nfs/%s%02d/f%03d", name, w, i)] = shaOf(data)
						mu.Unlock()
					}
				}(name, mnt, w)
			}
		}
		// Both mounts write the same file too; one of their versions wins.
		sharedVersions := map[string]bool{}
		for name, mnt := range mounts {
			wg.Add(1)
			go func(name, mnt string) {
				defer wg.Done()
				os.MkdirAll(filepath.Join(mnt, "load", "nfs-same"), 0755)
				for i := 0; i < 10; i++ {
					data := randomBytes(fileSize(i))
					mu.Lock()
					sharedVersions[shaOf(data)] = true
					mu.Unlock()
					if err := os.WriteFile(filepath.Join(mnt, "load", "nfs-same", "both"), data, 0644); err != nil {
						errs.add("write the shared file on %s: %v", name, err)
					}
				}
			}(name, mnt)
		}
		nfsWatchdog(t, &wg, 3*time.Minute, servers, mounts)
		errs.check(t, "NFS writes")

		// The folders made over NFS have markers; the files are as written.
		got, err := listShas(ctx, adminA.Client, "load", "nfs/")
		if err == nil {
			for k, v := range got {
				if strings.HasSuffix(k, "/") {
					want[k] = v
				}
			}
		}
		t.Logf("both servers agreed %s after the last write", converge(t, ctx, servers, "load", "nfs/", want, settle).Round(time.Millisecond))
		files2 := map[string]string{}
		for k, v := range want {
			if !strings.HasSuffix(k, "/") {
				files2[k] = v
			}
		}
		readAll(t, ctx, servers, "load", files2)

		var final string
		if !waitForCond(int(settle.Seconds()), func() bool {
			a, errA := readSha(ctx, adminA.Client, "load", "nfs-same/both")
			b, errB := readSha(ctx, adminB.Client, "load", "nfs-same/both")
			final = a
			return errA == nil && errB == nil && a == b
		}) {
			t.Fatal("the servers did not agree on the file both mounts wrote")
		}
		if !sharedVersions[final] {
			t.Error("the file both mounts wrote holds content neither wrote")
		}
	})

	// Neither server's storage is damaged, and neither logged an error about
	// files, NFS or the cluster.
	t.Run("storage intact", func(t *testing.T) {
		for _, s := range servers {
			out, err := runKnot(s.server, s.user, "admin", "file", "fsck")
			if err != nil || !strings.Contains(out, "No problems found") {
				t.Errorf("fsck on server %s: %v\n%s", s.name, err, out)
			}
		}
		errLine := regexp.MustCompile(`\bERR\b`)
		about := regexp.MustCompile(`(?i)file|nfs|cluster|gossip|store`)
		for _, s := range servers {
			data, err := os.ReadFile(s.server.LogPath)
			if err != nil {
				t.Fatal(err)
			}
			for _, line := range strings.Split(string(data), "\n") {
				line = plain(line)
				if errLine.MatchString(line) && about.MatchString(line) {
					t.Errorf("server %s logged: %s", s.name, line)
				}
			}
		}
	})
}

// nfsWatchdog waits for wg. Writes over NFS that have not finished after d
// are a hang to look into: it has every server and nfs-server dump its
// goroutines to its log, stops the nfs-servers so the stuck writes fail
// rather than wait forever, frees the mounts and fails the test.
func nfsWatchdog(t *testing.T, wg *sync.WaitGroup, d time.Duration, servers []writer, mounts map[string]string) {
	t.Helper()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
		return
	case <-time.After(d):
	}
	nfsServers := "^" + regexp.QuoteMeta(bins.Server) + " nfs-server"
	exec.Command("pkill", "-QUIT", "-f", nfsServers).Run()
	for _, s := range servers {
		exec.Command("sh", "-c", fmt.Sprintf("kill -QUIT $(lsof -ti tcp:%d -sTCP:LISTEN)", s.server.HTTPPort)).Run()
	}
	time.Sleep(2 * time.Second)
	exec.Command("pkill", "-KILL", "-f", nfsServers).Run()
	for _, m := range mounts {
		go unmount(m)
	}
	select {
	case <-done:
	case <-time.After(30 * time.Second):
	}
	t.Fatalf("writes over NFS did not finish in %s: goroutine dumps are in %s", d, filepath.Dir(servers[0].server.LogPath))
}

// runKnot runs the knot command against a server as a user.
func runKnot(s *harness.Server, u *harness.User, args ...string) (string, error) {
	cmd := exec.Command(bins.Server, append(args, "--server", s.BaseURL, "--token", u.Token)...)
	out, err := cmd.CombinedOutput()
	return plain(string(out)), err
}

// formatSize renders a byte count.
func formatSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%d KB", n>>10)
}
