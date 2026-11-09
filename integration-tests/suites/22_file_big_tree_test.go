//go:build integration

package suites

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/paularlott/knot/integration-tests/harness"
)

// bigTreeFiles is how many files the big tree test makes, from
// KNOT_TEST_BIG_TREE ("1" for 180,000); without it the test is skipped, as
// it takes minutes.
func bigTreeFiles(t *testing.T) int {
	v := os.Getenv("KNOT_TEST_BIG_TREE")
	switch v {
	case "":
		t.Skip("set KNOT_TEST_BIG_TREE to run (1 for 180,000 files, or a number)")
	case "1":
		return 180000
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 100 {
		t.Fatalf("KNOT_TEST_BIG_TREE=%q: want 1 or a number of files of at least 100", v)
	}
	return n
}

// filesPerFolder is how the big tree is spread, about as a vendor tree is.
const filesPerFolder = 12

var checksumsRe = regexp.MustCompile(`files read for their checksum.*count=(\d+)`)

// checksumCount totals what a sync logged reading for checksums.
func checksumCount(log string) int {
	n := 0
	for _, m := range checksumsRe.FindAllStringSubmatch(log, -1) {
		c, _ := strconv.Atoi(m[1])
		n += c
	}
	return n
}

// TestFileSyncBigTree syncs a large tree with the knot command built for
// this machine, and for Linux inside a space: the first sync, a second that
// reads no file for its checksum, and a watch that sees a change and a
// folder moved in quickly, without walking the whole tree.
func TestFileSyncBigTree(t *testing.T) {
	harness.Feature(t, "file-storage")
	n := bigTreeFiles(t)
	root := filesRoot(t)
	s, admin := bootDedicated(t, "bigtree", "--files-path", filepath.Join(root, "store"))
	ctx, cancel := testCtx(3600)
	defer cancel()
	if _, err := admin.Client.CreateFileBucket(ctx, "big"); err != nil {
		t.Fatal(err)
	}
	// Each folder synced has a marker in the bucket, as well as its files.
	folders := (n + filesPerFolder - 1) / filesPerFolder
	bucketCount := func(t *testing.T) int {
		t.Helper()
		b, err := admin.Client.GetFileBucket(ctx, "big")
		if err != nil {
			t.Fatal(err)
		}
		return int(b.Count)
	}
	headSHA := func(key string) string {
		o, err := admin.Client.HeadFileObject(ctx, "big", key)
		if err != nil {
			return ""
		}
		return o.SHA256
	}

	t.Run("this machine", func(t *testing.T) {
		tree := filepath.Join(root, "tree")
		start := time.Now()
		for i := 0; i < n; i++ {
			p := filepath.Join(tree, fmt.Sprintf("d%05d", i/filesPerFolder), fmt.Sprintf("f%02d.php", i%filesPerFolder))
			if i%filesPerFolder == 0 {
				os.MkdirAll(filepath.Dir(p), 0755)
			}
			if err := os.WriteFile(p, []byte(fmt.Sprintf("<?php // file %d\n", i)), 0644); err != nil {
				t.Fatal(err)
			}
		}
		t.Logf("made %d files in %d folders in %s", n, (n+filesPerFolder-1)/filesPerFolder, time.Since(start).Round(time.Millisecond))

		knot := func(args ...string) (string, time.Duration) {
			t.Helper()
			cmd := exec.Command(bins.Server, append(args, "--server", s.BaseURL, "--token", admin.Token)...)
			cmd.Dir, cmd.Env = root, append(os.Environ(), "HOME="+root)
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			start := time.Now()
			if err := cmd.Run(); err != nil {
				t.Fatalf("knot %s: %v\n%s", strings.Join(args, " "), err, tailLines(out.String(), 20))
			}
			return plain(out.String()), time.Since(start)
		}

		_, took := knot("--log-level", "debug", "file", "sync", tree, "big:tree")
		if c := bucketCount(t); c != n+folders {
			t.Fatalf("bucket holds %d files and folders after the first sync, want %d", c, n+folders)
		}
		t.Logf("first sync: %s", took.Round(time.Millisecond))

		out, took := knot("--log-level", "debug", "file", "sync", tree, "big:tree")
		if c := checksumCount(out); c != 0 {
			t.Errorf("a second sync read %d files for their checksum, want none", c)
		}
		if !strings.Contains(out, "uploaded=0") {
			t.Errorf("a second sync uploaded files:\n%s", tailLines(out, 5))
		}
		t.Logf("second sync, nothing changed: %s", took.Round(time.Millisecond))

		// Watching.
		logPath := filepath.Join(root, "watch.log")
		logFile, _ := os.Create(logPath)
		watch := exec.Command(bins.Server, "--log-level", "debug", "file", "sync", "--watch", tree, "big:tree", "--server", s.BaseURL, "--token", admin.Token)
		watch.Dir, watch.Env = root, append(os.Environ(), "HOME="+root)
		watch.Stdout, watch.Stderr = logFile, logFile
		if err := watch.Start(); err != nil {
			t.Fatal(err)
		}
		stopped := false
		stop := func() {
			if !stopped {
				stopped = true
				watch.Process.Signal(syscall.SIGINT)
				done := make(chan struct{})
				go func() { watch.Wait(); close(done) }()
				select {
				case <-done:
				case <-time.After(30 * time.Second):
					watch.Process.Kill()
					t.Error("the watch did not stop on SIGINT")
				}
				logFile.Close()
			}
		}
		t.Cleanup(stop)
		readLog := func() string { data, _ := os.ReadFile(logPath); return plain(string(data)) }
		started := time.Now()
		if !waitForCond(600, func() bool { return strings.Contains(readLog(), "watching for changes") }) {
			t.Fatalf("the watch did not start:\n%s", tailLines(readLog(), 20))
		}
		t.Logf("watch ready in %s", time.Since(started).Round(time.Millisecond))
		time.Sleep(2 * time.Second) // let the first pass settle

		// A change deep in the tree.
		changed := filepath.Join(tree, "d00007", "f03.php")
		os.WriteFile(changed, []byte("<?php // changed\n"), 0644)
		sum := sha256Hex([]byte("<?php // changed\n"))
		start = time.Now()
		if !pollUntil(400*time.Second, func() bool { return headSHA("tree/d00007/f03.php") == sum }) {
			t.Errorf("a change was not synced:\n%s", tailLines(readLog(), 20))
		}
		t.Logf("change synced in %s", time.Since(start).Round(time.Millisecond))

		// A folder moved in, as an unpack or a package install makes.
		staging := filepath.Join(root, "staging", "newpkg")
		for i := 0; i < 200; i++ {
			p := filepath.Join(staging, fmt.Sprintf("s%02d", i/20), fmt.Sprintf("n%03d.php", i))
			os.MkdirAll(filepath.Dir(p), 0755)
			os.WriteFile(p, []byte(fmt.Sprintf("<?php // new %d\n", i)), 0644)
		}
		if err := os.Rename(staging, filepath.Join(tree, "newpkg")); err != nil {
			t.Fatal(err)
		}
		start = time.Now()
		if !pollUntil(400*time.Second, func() bool {
			return headSHA("tree/newpkg/s09/n199.php") != "" && headSHA("tree/newpkg/s00/n000.php") != ""
		}) {
			t.Errorf("a folder moved in was not synced:\n%s", tailLines(readLog(), 20))
		}
		t.Logf("folder of 200 files moved in, synced in %s", time.Since(start).Round(time.Millisecond))

		stop()
		log := readLog()
		if strings.Contains(log, "looking at every local file") {
			t.Error("the watch walked the whole tree for a change")
		}
		if strings.Contains(log, "cannot watch every folder") {
			t.Errorf("the watch could not watch every folder:\n%s", grepLines(log, "cannot watch"))
		}

		// Two ways: the state of every file is written once, and a change
		// to one adds a record rather than writing it all again.
		state := filepath.Join(root, "state.bin")
		_, took = knot("file", "sync", "--two-way", tree, "big:tree", "--state-file", state)
		full := sizeOf(t, state)
		t.Logf("first two-way sync: %s, state %.1f MB", took.Round(time.Millisecond), float64(full)/1e6)
		_, took = knot("file", "sync", "--two-way", tree, "big:tree", "--state-file", state)
		if grown := sizeOf(t, state) - full; grown != 0 {
			t.Errorf("a two-way sync with nothing changed wrote %d bytes of state", grown)
		}
		t.Logf("second two-way sync, nothing changed: %s", took.Round(time.Millisecond))
		os.WriteFile(filepath.Join(tree, "d00009", "f04.php"), []byte("<?php // changed again\n"), 0644)
		_, took = knot("file", "sync", "--two-way", tree, "big:tree", "--state-file", state)
		grown := sizeOf(t, state) - full
		if grown <= 0 || grown > 200 {
			t.Errorf("one change wrote %d bytes of a %d byte state, want one record", grown, full)
		}
		t.Logf("two-way sync of one change: %s, wrote %d bytes of state", took.Round(time.Millisecond), grown)
		caches, _ := filepath.Glob(filepath.Join(root, "*", "*", "knot", "sync", "hashes-*.bin"))
		more, _ := filepath.Glob(filepath.Join(root, ".config", "knot", "sync", "hashes-*.bin"))
		for _, c := range append(caches, more...) {
			t.Logf("checksum cache %s: %.1f MB", filepath.Base(c), float64(sizeOf(t, c))/1e6)
		}
	})

	t.Run("linux space", func(t *testing.T) {
		templateId, err := harness.CreateTemplate(s, admin.Client, uniqueName("it-big"), harness.TemplateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		spaceId := harness.CreateSpace(t, admin.Client, uniqueName("it-big"), templateId, admin.Id)
		t.Cleanup(func() { harness.DeleteSpaceAndWait(t, admin.Client, spaceId) })
		harness.WaitForSpaceReady(t, s, admin.Client, spaceId)
		sh := func(timeout int, script string) string {
			t.Helper()
			return plain(harness.RunCommand(t, admin.Client, spaceId, timeout, "sh", "-c", script))
		}
		t.Log(strings.TrimSpace(sh(30, "uname -m; id; echo HOME=$HOME; pwd; knot --version 2>&1; cat /proc/sys/fs/inotify/max_user_watches")))

		start := time.Now()
		made := sh(1200, fmt.Sprintf(`set -e; cd ~; rm -rf t; seq -f 't/d%%05g' 0 %d | xargs mkdir -p
awk -v n=%d -v per=%d 'BEGIN { for (i = 0; i < n; i++) { f = sprintf("t/d%%05d/f%%02d.php", int(i/per), i%%per); printf("<?php // file %%d\n", i) > f; close(f) } }'
echo "$(pwd): $(find t -type f | wc -l) files"`,
			(n-1)/filesPerFolder, n, filesPerFolder))
		t.Logf("made %d files in the space in %s: %s", n, time.Since(start).Round(time.Millisecond), strings.TrimSpace(made))

		before := bucketCount(t)
		start = time.Now()
		// The output, a line a file, goes to a file: run-command returns a
		// command's output in one piece.
		if out := sh(1800, "cd ~ && knot file sync t big:linux > /tmp/first.log 2>&1; echo EXIT=$?; tail -3 /tmp/first.log"); !strings.Contains(out, "EXIT=0") {
			t.Fatalf("first sync in the space failed:\n%s", tailLines(out, 20))
		}
		t.Logf("first sync: %s", time.Since(start).Round(time.Millisecond))
		if got := bucketCount(t) - before; got != n+folders {
			t.Fatalf("the bucket gained %d files and folders from the space, want %d", got, n+folders)
		}

		start = time.Now()
		out := sh(1800, "cd ~ && knot --log-level debug file sync t big:linux > /tmp/second.log 2>&1; grep -E 'checksum|synced' /tmp/second.log")
		if c := checksumCount(out); c != 0 {
			t.Errorf("a second sync in the space read %d files for their checksum, want none", c)
		}
		if !strings.Contains(out, "uploaded=0") {
			t.Errorf("a second sync in the space uploaded files:\n%s", tailLines(out, 5))
		}
		t.Logf("second sync, nothing changed: %s", time.Since(start).Round(time.Millisecond))

		// Watching, in the background of the space.
		sh(30, "cd ~ && rm -f /tmp/w.log && (nohup knot --log-level debug file sync --watch t big:linux > /tmp/w.log 2>&1 &) ; sleep 1")
		readLog := func() string { return sh(30, "cat /tmp/w.log") }
		started := time.Now()
		if !pollUntil(600*time.Second, func() bool { return strings.Contains(readLog(), "watching for changes") }) {
			t.Fatalf("the watch in the space did not start:\n%s", tailLines(readLog(), 20))
		}
		t.Logf("watch ready in %s", time.Since(started).Round(time.Millisecond))
		time.Sleep(2 * time.Second)

		sh(30, `printf '<?php // changed\n' > ~/t/d00007/f03.php`)
		sum := sha256Hex([]byte("<?php // changed\n"))
		start = time.Now()
		if !pollUntil(400*time.Second, func() bool { return headSHA("linux/d00007/f03.php") == sum }) {
			t.Errorf("a change in the space was not synced:\n%s", tailLines(readLog(), 20))
		}
		t.Logf("change synced in %s", time.Since(start).Round(time.Millisecond))

		sh(60, `set -e; rm -rf /tmp/newpkg; for d in 0 1 2 3 4 5 6 7 8 9; do mkdir -p /tmp/newpkg/s0$d; for f in $(seq -w 0 19); do printf 'x\n' > /tmp/newpkg/s0$d/n$f.php; done; done; mv /tmp/newpkg ~/t/newpkg`)
		start = time.Now()
		if !pollUntil(400*time.Second, func() bool {
			return headSHA("linux/newpkg/s09/n19.php") != "" && headSHA("linux/newpkg/s00/n00.php") != ""
		}) {
			t.Errorf("a folder moved in in the space was not synced:\n%s", tailLines(readLog(), 20))
		}
		t.Logf("folder of 200 files moved in, synced in %s", time.Since(start).Round(time.Millisecond))

		sh(30, "pkill -INT -f '[f]ile sync --watch' || true; sleep 2")
		log := readLog()
		if strings.Contains(log, "looking at every local file") {
			t.Error("the watch in the space walked the whole tree for a change")
		}
		if strings.Contains(log, "cannot watch every folder") {
			t.Errorf("the watch in the space could not watch every folder:\n%s", grepLines(log, "cannot watch"))
		}
	})
}

// pollUntil checks cond every 100ms until it holds or timeout passes.
func pollUntil(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
	return true
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// plain is log output without its colours.
func plain(s string) string { return ansiRe.ReplaceAllString(s, "") }

func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func grepLines(s, sub string) string {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		if strings.Contains(sc.Text(), sub) {
			out = append(out, sc.Text())
		}
	}
	return strings.Join(out, "\n")
}

func sizeOf(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}
