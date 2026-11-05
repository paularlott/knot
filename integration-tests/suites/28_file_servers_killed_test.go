//go:build integration

package suites

import (
	"bufio"
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	mrand "math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
)

// ackLog is what was written to each of a set of files (or parts of a
// file), and which of it was acknowledged: what a reader may find there.
type ackLog struct {
	mu sync.Mutex
	m  map[string]*acks
}

type acks struct {
	last    []byte          // the last version acknowledged
	hasLast bool            // whether any was
	pending [][]byte        // tried since, not acknowledged (yet)
	earlier map[string]bool // shas of versions acknowledged before last
}

func (l *ackLog) get(key string) *acks {
	if l.m == nil {
		l.m = map[string]*acks{}
	}
	a := l.m[key]
	if a == nil {
		a = &acks{earlier: map[string]bool{}}
		l.m[key] = a
	}
	return a
}

// try records a write about to be made. truncates: it empties the file
// before writing, so an empty file is among what it can leave.
func (l *ackLog) try(key string, data []byte, truncates bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.get(key)
	if truncates {
		a.pending = append(a.pending, nil)
	}
	a.pending = append(a.pending, data)
}

// ok records a write acknowledged: the file holds it, or something written
// after it.
func (l *ackLog) ok(key string, data []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.get(key)
	if a.hasLast {
		a.earlier[shaOf(a.last)] = true
	}
	a.last, a.hasLast, a.pending = data, true, nil
}

func (l *ackLog) keys() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	for k := range l.m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// check says what is wrong with a file holding got (present false: not
// there). A file holds its last acknowledged version or one tried after it;
// one whose writes after that failed can also hold blocks of those writes
// over the version before, as a local file can after a crash before fsync
// (torn).
func (l *ackLog) check(key string, got []byte, present bool) (problem string, torn bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.m[key]
	if a == nil {
		if present {
			return "holds content no one wrote", false
		}
		return "", false
	}
	if !present {
		if a.hasLast {
			return fmt.Sprintf("is missing, though %.8s was acknowledged", shaOf(a.last)), false
		}
		return "", false
	}
	if a.hasLast && bytes.Equal(got, a.last) {
		return "", false
	}
	for _, p := range a.pending {
		if bytes.Equal(got, p) {
			return "", false
		}
	}
	cands := a.pending
	if a.hasLast {
		cands = append([][]byte{a.last}, cands...)
	}
	if len(a.pending) > 0 && blocksOf(got, cands) {
		return "", true
	}
	sha := shaOf(got)
	what := "content no write had"
	if a.earlier[sha] {
		what = "an earlier acknowledged version"
	} else if blocksOf(got, cands) {
		what = "blocks of the last acknowledged version and another"
	}
	last := "nothing"
	if a.hasLast {
		last = fmt.Sprintf("%.8s (%d bytes)", shaOf(a.last), len(a.last))
	}
	return fmt.Sprintf("holds %s, %.8s (%d bytes); last acknowledged %s, %d tried since", what, sha, len(got), last, len(a.pending)), false
}

// blocksOf says whether every 4 KB block of got is that block of one of
// cands, or zeros.
func blocksOf(got []byte, cands [][]byte) bool {
	const block = 4096
	for off := 0; off < len(got); off += block {
		end := min(off+block, len(got))
		b := got[off:end]
		ok := allZero(b)
		for _, c := range cands {
			if ok {
				break
			}
			ok = len(c) >= end && bytes.Equal(c[off:end], b)
		}
		if !ok {
			return false
		}
	}
	return true
}

// crashWriter is one writer's tally.
type crashWriter struct {
	name     string
	ok, fail atomic.Int64
	lastOK   atomic.Int64 // unix nanos
	mu       sync.Mutex
	samples  []string
}

// record counts a write that succeeded (err "") or failed.
func (w *crashWriter) record(err string) {
	if err == "" {
		w.ok.Add(1)
		w.lastOK.Store(time.Now().UnixNano())
		return
	}
	w.fail.Add(1)
	w.mu.Lock()
	if len(w.samples) < 3 {
		w.samples = append(w.samples, err)
	}
	w.mu.Unlock()
}

// writeSynced writes data to p over NFS as a program that must not lose it
// does: it is acknowledged once fsync returns.
func writeSynced(p string, data []byte) (synced bool, err error) {
	f, err := os.OpenFile(p, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return false, err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return false, err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return false, err
	}
	return true, f.Close()
}

// nfsWorkerEnv, set, runs the test binary as the NFS client of
// TestFileServersKilled (see runNFSWorker), configured by its JSON value.
//
// The writes over NFS are made from a process of their own, as a client
// machine's would be: a process holding files open on a mount cannot start
// the nfs-server behind it, as the test does as it restarts one. Starting a
// program closes the files it is not to keep, and closing a file on an NFS
// mount waits until what was written to it is saved, to the nfs-server
// that is being started.
const nfsWorkerEnv = "KNOT_TEST_NFS_WORKER"

// nfsWorkerConfig is what the NFS client writes to.
type nfsWorkerConfig struct {
	Mounts      map[string]string // server name -> mount
	Bucket      string
	SharedFiles int
	Parts       int
	PartSize    int
}

// nfsEvent is one line the NFS client reports: a write about to be made
// ("try", with the content's seed and size), acknowledged ("ok"), a shared
// file version written ("shared"), a write's outcome for its writer
// ("done", with err when it failed), or the client ready to write ("ready").
type nfsEvent struct {
	Ev       string `json:"ev"`
	Writer   string `json:"w,omitempty"`
	Key      string `json:"k,omitempty"`
	Seed     uint64 `json:"seed,omitempty"`
	Size     int    `json:"size,omitempty"`
	Truncate bool   `json:"trunc,omitempty"`
	Err      string `json:"err,omitempty"`
}

// seeded is size bytes made from seed: the NFS client and the test each
// make a write's content from its seed, so only that is reported.
func seeded(seed uint64, size int) []byte {
	var key [32]byte
	binary.LittleEndian.PutUint64(key[:], seed)
	b := make([]byte, size)
	mrand.NewChaCha8(key).Read(b)
	return b
}

// runNFSWorker is the NFS client of TestFileServersKilled: it writes until
// its stdin closes, reporting every write on stdout, and returns the exit
// code.
func runNFSWorker() int {
	var cfg nfsWorkerConfig
	if err := json.Unmarshal([]byte(os.Getenv(nfsWorkerEnv)), &cfg); err != nil {
		fmt.Fprintln(os.Stderr, "nfs worker:", err)
		return 2
	}
	var outMu sync.Mutex
	enc := json.NewEncoder(os.Stdout)
	report := func(e nfsEvent) {
		outMu.Lock()
		enc.Encode(e)
		outMu.Unlock()
	}
	var stopping atomic.Bool
	go func() {
		io.Copy(io.Discard, os.Stdin)
		stopping.Store(true)
	}()
	running := func() bool { return !stopping.Load() }
	newSeed := func() uint64 {
		var b [8]byte
		crand.Read(b[:])
		return binary.LittleEndian.Uint64(b[:])
	}
	done := func(w string, err error) {
		e := nfsEvent{Ev: "done", Writer: w}
		if err != nil {
			e.Err = err.Error()
			time.Sleep(200 * time.Millisecond)
		}
		report(e)
	}

	var wg sync.WaitGroup
	for name, mnt := range cfg.Mounts {
		root := filepath.Join(mnt, cfg.Bucket)
		// Files of their own.
		for n := 0; n < 2; n++ {
			w := fmt.Sprintf("NFS %s-%d", name, n)
			sub := "own/" + strings.ReplaceAll(w, " ", "-")
			if err := os.MkdirAll(filepath.Join(root, sub), 0755); err != nil {
				fmt.Fprintln(os.Stderr, "nfs worker:", err)
				return 2
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; running(); i++ {
					key := fmt.Sprintf("%s/f%02d", sub, i%20)
					seed, size := newSeed(), fileSize(i)
					report(nfsEvent{Ev: "try", Key: key, Seed: seed, Size: size, Truncate: true})
					synced, err := writeSynced(filepath.Join(root, filepath.FromSlash(key)), seeded(seed, size))
					if synced {
						report(nfsEvent{Ev: "ok", Key: key, Seed: seed, Size: size})
					}
					done(w, err)
				}
			}()
		}
		// The shared files, written as a temporary file renamed over them.
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := "NFS shared " + name
			for i := 0; running(); i++ {
				key := fmt.Sprintf("same/k%d", i%cfg.SharedFiles)
				seed, size := newSeed(), fileSize(i)
				report(nfsEvent{Ev: "shared", Key: key, Seed: seed, Size: size})
				tmp := filepath.Join(root, "tmp", fmt.Sprintf("%s-%d", name, i))
				_, err := writeSynced(tmp, seeded(seed, size))
				if err == nil {
					err = os.Rename(tmp, filepath.Join(root, filepath.FromSlash(key)))
				}
				if err != nil {
					os.Remove(tmp)
				}
				done(w, err)
			}
		}()
		// The large file, a part at a time, in place.
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := "NFS large " + name
			p := filepath.Join(root, "big"+name+".bin")
			for running() {
				i := mrand.IntN(cfg.Parts)
				key := fmt.Sprintf("big%s.bin#%02d", name, i)
				seed := newSeed()
				report(nfsEvent{Ev: "try", Key: key, Seed: seed, Size: cfg.PartSize})
				err := func() error {
					f, err := os.OpenFile(p, os.O_RDWR, 0)
					if err != nil {
						return err
					}
					if _, err := f.WriteAt(seeded(seed, cfg.PartSize), int64(i*cfg.PartSize)); err != nil {
						f.Close()
						return err
					}
					if err := f.Sync(); err != nil {
						f.Close()
						return err
					}
					report(nfsEvent{Ev: "ok", Key: key, Seed: seed, Size: cfg.PartSize})
					return f.Close()
				}()
				done(w, err)
			}
		}()
	}
	report(nfsEvent{Ev: "ready"})
	wg.Wait()
	return 0
}

// TestFileServersKilled writes files to two clustered servers, through the
// API of each and an NFS mount of each, while each server and each
// nfs-server in turn is killed (SIGKILL, as a crash) and started again, and
// at the end a server and its nfs-server together. Writers write files of
// their own and rewrite them, write a few files all of them share, and
// change parts of a large file in place over NFS, fsync'ing each write.
//
// It checks that nothing hangs; that writes fail only while what they go
// through is down, every writer's writes succeeding again after the last
// kill; that both servers end with the same files; that every file holds
// its last acknowledged write, or one tried after it (or, after writes that
// failed, blocks of those over it, counted as torn), never an older one or
// none; that the shared files each hold a version written; that each mount
// then reads what the servers hold; and that storage is intact.
func TestFileServersKilled(t *testing.T) {
	harness.Feature(t, "file-storage")
	skipUnlessNFS(t)
	root := filesRoot(t)
	nodeA, nodeB, adminA, adminB := bootFilePair(t, root, "killeda", "killedb")
	servers := []writer{{"A", nodeA, adminA}, {"B", nodeB, adminB}}
	ctx, cancel := testCtx(1200)
	defer cancel()
	const bucket = "data"
	if _, err := adminA.Client.CreateFileBucket(ctx, bucket); err != nil {
		t.Fatal(err)
	}
	if !waitForCond(60, func() bool { _, err := adminB.Client.GetFileBucket(ctx, bucket); return err == nil }) {
		t.Fatal("the bucket did not reach server B")
	}

	portA, restartNFSA := startNFSRestartable(t, nodeA, adminA)
	portB, restartNFSB := startNFSRestartable(t, nodeB, adminB)
	mounts := map[string]string{
		"A": mountNFSAt(t, portA, filepath.Join(root, "killed-mnt-a")),
		"B": mountNFSAt(t, portB, filepath.Join(root, "killed-mnt-b")),
	}
	for _, m := range mounts {
		for _, dir := range []string{"own", "same", "tmp"} {
			if err := os.MkdirAll(filepath.Join(m, bucket, dir), 0755); err != nil {
				t.Fatal(err)
			}
		}
	}

	var log ackLog
	var sharedMu sync.Mutex
	shared := map[string]map[string]bool{} // key -> shas written
	sharedTry := func(key string, data []byte) {
		sharedMu.Lock()
		if shared[key] == nil {
			shared[key] = map[string]bool{}
		}
		shared[key][shaOf(data)] = true
		sharedMu.Unlock()
	}
	const sharedFiles = 4
	sharedKey := func(i int) string { return fmt.Sprintf("same/k%d", i%sharedFiles) }

	// A large file per mount, changed a part at a time over it.
	const parts, partSize = 16, 1 << 20
	bigKey := func(s string) string { return "big" + s + ".bin" }
	partKey := func(s string, i int) string { return fmt.Sprintf("%s#%02d", bigKey(s), i) }
	for _, s := range servers {
		start := randomBytes(parts * partSize)
		if _, err := s.client().PutFileObject(ctx, bucket, bigKey(s.name), bytes.NewReader(start), int64(len(start)), "", apiclient.FileAttrs{}); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < parts; i++ {
			log.ok(partKey(s.name, i), start[i*partSize:(i+1)*partSize])
		}
	}
	if !waitForCond(60, func() bool {
		got, err := listShas(ctx, adminB.Client, bucket, "big")
		return err == nil && len(got) == 2
	}) {
		t.Fatal("the large files did not reach server B")
	}

	var writers []*crashWriter
	newWriter := func(name string) *crashWriter {
		w := &crashWriter{name: name}
		writers = append(writers, w)
		return w
	}
	var stopping atomic.Bool
	running := func() bool { return !stopping.Load() && ctx.Err() == nil }
	apiDone := func(w *crashWriter, err error) {
		if err == nil {
			w.record("")
			return
		}
		w.record(err.Error())
		time.Sleep(200 * time.Millisecond)
	}
	var wg sync.WaitGroup

	for _, s := range servers {
		// Files of their own through the API, each to its own server.
		for n := 0; n < 2; n++ {
			w := newWriter(fmt.Sprintf("API %s-%d", s.name, n))
			wg.Add(1)
			go func(w *crashWriter) {
				defer wg.Done()
				for i := 0; running(); i++ {
					key := fmt.Sprintf("own/%s/f%02d", strings.ReplaceAll(w.name, " ", "-"), i%20)
					data := randomBytes(fileSize(i))
					log.try(key, data, false)
					wctx, cancel := context.WithTimeout(ctx, 30*time.Second)
					_, err := s.client().PutFileObject(wctx, bucket, key, bytes.NewReader(data), int64(len(data)), "", apiclient.FileAttrs{})
					cancel()
					if err == nil {
						log.ok(key, data)
					}
					apiDone(w, err)
				}
			}(w)
		}
		// The shared files through the API.
		w := newWriter("API shared " + s.name)
		wg.Add(1)
		go func(w *crashWriter) {
			defer wg.Done()
			for i := 0; running(); i++ {
				data := randomBytes(fileSize(i))
				sharedTry(sharedKey(i), data)
				wctx, cancel := context.WithTimeout(ctx, 30*time.Second)
				_, err := s.client().PutFileObject(wctx, bucket, sharedKey(i), bytes.NewReader(data), int64(len(data)), "", apiclient.FileAttrs{})
				cancel()
				apiDone(w, err)
			}
		}(w)
	}

	// The NFS client, a process of its own (see nfsWorkerEnv).
	for _, s := range servers {
		for n := 0; n < 2; n++ {
			newWriter(fmt.Sprintf("NFS %s-%d", s.name, n))
		}
		newWriter("NFS shared " + s.name)
		newWriter("NFS large " + s.name)
	}
	byName := map[string]*crashWriter{}
	for _, w := range writers {
		byName[w.name] = w
	}
	workerCfg, _ := json.Marshal(nfsWorkerConfig{Mounts: mounts, Bucket: bucket, SharedFiles: sharedFiles, Parts: parts, PartSize: partSize})
	worker := exec.Command(os.Args[0])
	worker.Env = append(os.Environ(), nfsWorkerEnv+"="+string(workerCfg))
	var workerErr bytes.Buffer
	worker.Stderr = &workerErr
	stdin, err := worker.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := worker.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Start(); err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{})
	var errs errorList
	wg.Add(1)
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			var e nfsEvent
			if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
				errs.add("the NFS client reported %q: %v", sc.Text(), err)
				continue
			}
			switch e.Ev {
			case "ready":
				close(ready)
			case "try":
				log.try(e.Key, seeded(e.Seed, e.Size), e.Truncate)
			case "ok":
				log.ok(e.Key, seeded(e.Seed, e.Size))
			case "shared":
				sharedTry(e.Key, seeded(e.Seed, e.Size))
			case "done":
				if w := byName[e.Writer]; w != nil {
					w.record(e.Err)
				}
			}
		}
		if err := worker.Wait(); err != nil {
			errs.add("the NFS client failed: %v\n%s", err, workerErr.String())
		}
	}()
	select {
	case <-ready:
	case <-time.After(60 * time.Second):
		stdin.Close()
		t.Fatalf("the NFS client did not start:\n%s", workerErr.String())
	}

	// The kills, one at a time with writing between.
	type kill struct {
		what string
		do   func() error
	}
	kills := []kill{
		{"nfs-server A", func() error { restartNFSA(); return nil }},
		{"server B", nodeB.Crash},
		{"server A", nodeA.Crash},
		{"nfs-server B", func() error { restartNFSB(); return nil }},
		{"server A and nfs-server A together", func() error {
			done := make(chan error, 1)
			go func() { done <- nodeA.Crash() }()
			restartNFSA()
			return <-done
		}},
	}
	began := time.Now()
	for _, k := range kills {
		time.Sleep(8 * time.Second)
		at := time.Since(began)
		if err := k.do(); err != nil {
			stopping.Store(true)
			wg.Wait()
			t.Fatalf("restart after killing %s: %v", k.what, err)
		}
		t.Logf("killed %s at %s, back in %s", k.what, at.Round(100*time.Millisecond), (time.Since(began) - at).Round(100*time.Millisecond))
	}
	recovered := time.Now()
	time.Sleep(25 * time.Second)
	stopping.Store(true)
	stdin.Close()
	nfsWatchdog(t, &wg, 3*time.Minute, servers, mounts)
	errs.check(t, "writing")

	var failed, total int64
	for _, w := range writers {
		ok, fail := w.ok.Load(), w.fail.Load()
		total += ok + fail
		failed += fail
		if fail > 0 {
			t.Logf("%s: %d writes, %d failed, as %s", w.name, ok+fail, fail, strings.Join(w.samples, "; "))
		}
		if last := w.lastOK.Load(); last < recovered.UnixNano() {
			t.Errorf("%s made no write in the %s after the last restart (%d made, %d failed: %s)", w.name, 25*time.Second, ok, fail, strings.Join(w.samples, "; "))
		}
	}
	t.Logf("%d writes in %s, %d failed", total, time.Since(began).Round(time.Second), failed)

	// Both servers end with the same files.
	var final map[string]string
	start := time.Now()
	if !waitForCond(180, func() bool {
		a, err := listShas(ctx, adminA.Client, bucket, "")
		if err != nil {
			return false
		}
		b, err := listShas(ctx, adminB.Client, bucket, "")
		if err != nil {
			return false
		}
		// Files only: not the temporary files renamed over the shared
		// ones, nor folders made over NFS (their markers).
		for _, m := range []map[string]string{a, b} {
			for k := range m {
				if strings.HasPrefix(k, "tmp/") || strings.HasSuffix(k, "/") {
					delete(m, k)
				}
			}
		}
		final = a
		return diffShas(a, b) == ""
	}) {
		a, _ := listShas(ctx, adminA.Client, bucket, "")
		b, _ := listShas(ctx, adminB.Client, bucket, "")
		t.Fatalf("the servers did not agree in 3 minutes: %s", diffShas(a, b))
	}
	t.Logf("both servers agreed %s after the writing stopped", time.Since(start).Round(time.Millisecond))

	fetch := func(key string) []byte {
		resp, err := adminA.Client.GetFileObject(ctx, bucket, key)
		if err != nil {
			t.Fatalf("read %s: %v", key, err)
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read %s: %v", key, err)
		}
		return b
	}

	// Every file holds its last acknowledged write, or one tried after it.
	t.Run("acknowledged writes kept", func(t *testing.T) {
		var problems []string
		var torn int
		check := func(key string, got []byte, present bool) {
			if p, isTorn := log.check(key, got, present); p != "" {
				problems = append(problems, key+" "+p)
			} else if isTorn {
				torn++
			}
		}
		bigs := map[string][]byte{}
		for _, key := range log.keys() {
			if name, part, ok := strings.Cut(key, "#"); ok {
				data, ok := bigs[name]
				if !ok {
					data = fetch(name)
					bigs[name] = data
					if len(data) != parts*partSize {
						problems = append(problems, fmt.Sprintf("%s is %d bytes, not %d", name, len(data), parts*partSize))
					}
				}
				var i int
				fmt.Sscanf(part, "%d", &i)
				if len(data) >= (i+1)*partSize {
					check(key, data[i*partSize:(i+1)*partSize], true)
				}
				continue
			}
			_, present := final[key]
			var got []byte
			if present {
				got = fetch(key)
			}
			check(key, got, present)
		}
		for key := range final {
			if strings.HasPrefix(key, "own/") {
				if _, ok := log.m[key]; !ok { // the writers have stopped
					problems = append(problems, key+" is there, though no one wrote it")
				}
			}
		}
		if torn > 0 {
			t.Logf("%d files or parts hold blocks of writes that failed over what was there", torn)
		}
		if len(problems) > 0 {
			sort.Strings(problems)
			t.Errorf("%d of %d files and parts do not hold what was acknowledged:\n%s", len(problems), len(log.keys()), strings.Join(problems[:min(len(problems), 20)], "\n"))
		}
	})

	t.Run("shared files hold a version written", func(t *testing.T) {
		for i := 0; i < sharedFiles; i++ {
			key := sharedKey(i)
			sha, ok := final[key]
			if !ok {
				t.Errorf("%s is missing", key)
				continue
			}
			if !shared[key][sha] {
				t.Errorf("%s holds %.8s, which no one wrote", key, sha)
			}
		}
	})

	// Each mount reads what the servers hold.
	t.Run("mounts read what the servers hold", func(t *testing.T) {
		for name, m := range mounts {
			var bad []string
			start := time.Now()
			if !waitForCond(60, func() bool {
				bad = nil
				for key, want := range final {
					got, err := shaOfFile(filepath.Join(m, bucket, filepath.FromSlash(key)))
					if err != nil {
						bad = append(bad, fmt.Sprintf("%s: %v", key, err))
					} else if got != want {
						bad = append(bad, key+" differs")
					}
				}
				return len(bad) == 0
			}) {
				sort.Strings(bad)
				t.Errorf("mount %s does not read what the servers hold after a minute: %d files, as %s", name, len(bad), strings.Join(bad[:min(len(bad), 8)], "; "))
				continue
			}
			t.Logf("mount %s matched the servers %s after they agreed", name, time.Since(start).Round(time.Millisecond))
		}
	})

	t.Run("storage intact", func(t *testing.T) {
		for _, s := range servers {
			out, err := runKnot(s.server, s.user, "admin", "file", "fsck")
			if err != nil || !strings.Contains(out, "No problems found") {
				t.Errorf("fsck on server %s: %v\n%s", s.name, err, out)
			}
		}
		// Errors logged about files or NFS, other than of the other
		// server being down, are shown, and panics fail the test.
		errLine := regexp.MustCompile(`\bERR\b`)
		about := regexp.MustCompile(`(?i)file|nfs|store`)
		logs, _ := filepath.Glob(filepath.Join(filepath.Dir(nodeA.LogPath), "*killed*.log"))
		for _, p := range logs {
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			var shown int
			for _, line := range strings.Split(string(data), "\n") {
				line = plain(line)
				if strings.Contains(line, "panic") || strings.Contains(line, "fatal error") {
					t.Errorf("%s: %s", filepath.Base(p), line)
				} else if errLine.MatchString(line) && about.MatchString(line) && shown < 10 {
					t.Logf("%s: %s", filepath.Base(p), line)
					shown++
				}
			}
		}
	})
}
