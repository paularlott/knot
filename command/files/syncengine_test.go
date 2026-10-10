package command_files

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/filestore"
	"github.com/paularlott/logger"
	logslog "github.com/paularlott/logger/slog"
)

var admin = &filestore.Principal{IsAdmin: true}

// logBuf collects what a sync logs, as JSON lines, for tests to look at.
type logBuf struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *logBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *logBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func (l *logBuf) logger() logger.Logger {
	return logslog.New(logslog.Config{Level: "debug", Format: "json", Writer: l}).WithGroup("sync")
}

// logged reports whether a line with a message starting msg and each of the
// "key":"value" fields was logged.
func logged(out, msg string, fields ...string) bool {
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, `"msg":"`+msg) {
			continue
		}
		ok := true
		for _, f := range fields {
			if !strings.Contains(line, f) {
				ok = false
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func readLocal(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestSyncExcludes(t *testing.T) {
	client, _ := newFilesServer(t)
	ctx := context.Background()
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "keep.txt"), "k")
	writeFile(t, filepath.Join(src, "debug.log"), "l")
	writeFile(t, filepath.Join(src, "node_modules", "x.js"), "x")
	writeFile(t, filepath.Join(src, ".git", "HEAD"), "ref")

	// A single pass sends everything not excluded, .git included.
	if err := runSyncRequest(ctx, client, src, "sync-test:a", syncRequest{excludes: []string{"*.log", "node_modules/"}}); err != nil {
		t.Fatal(err)
	}
	expectKeys(t, bucketKeys(t, client, "sync-test"), "a/.git/HEAD", "a/keep.txt")

	// Two ways, .git is left out by default, and included on request.
	if err := runSyncRequest(ctx, client, src, "sync-test:b", syncRequest{twoWay: true, statePath: filepath.Join(t.TempDir(), "s.json")}); err != nil {
		t.Fatal(err)
	}
	expectKeysWithPrefixIn(t, client, "sync-test", "b/", "b/debug.log", "b/keep.txt", "b/node_modules/x.js")
	if err := runSyncRequest(ctx, client, src, "sync-test:c", syncRequest{twoWay: true, noDefaultExcludes: true, statePath: filepath.Join(t.TempDir(), "s.json")}); err != nil {
		t.Fatal(err)
	}
	expectKeysWithPrefixIn(t, client, "sync-test", "c/", "c/.git/HEAD", "c/debug.log", "c/keep.txt", "c/node_modules/x.js")

	// An excluded remote file is neither fetched nor deleted.
	putRemote(t, client, "sync-test", "d/skip.log", "remote log")
	putRemote(t, client, "sync-test", "d/get.txt", "g")
	dst := t.TempDir()
	writeFile(t, filepath.Join(dst, "local.log"), "mine")
	if err := runSyncRequest(ctx, client, "sync-test:d", dst, syncRequest{del: true, excludes: []string{"*.log"}}); err != nil {
		t.Fatal(err)
	}
	expectKeys(t, localFiles(t, dst), "get.txt", "local.log")
}

func TestSyncMassDeleteGuard(t *testing.T) {
	client, _ := newFilesServer(t)
	ctx := context.Background()
	src := t.TempDir()
	for i := 0; i < 20; i++ {
		writeFile(t, filepath.Join(src, "f"+string(rune('a'+i))+".txt"), "x")
	}
	if err := runSync(ctx, client, src, "sync-test:m", false, false); err != nil {
		t.Fatal(err)
	}

	// Most of the files going is refused, and nothing changes.
	for i := 0; i < 15; i++ {
		os.Remove(filepath.Join(src, "f"+string(rune('a'+i))+".txt"))
	}
	writeFile(t, filepath.Join(src, "new.txt"), "n")
	err := runSync(ctx, client, src, "sync-test:m", true, false)
	if err == nil || !strings.Contains(err.Error(), "refusing to delete 15") {
		t.Fatalf("mass delete not refused: %v", err)
	}
	if n := len(bucketKeys(t, client, "sync-test")); n != 20 {
		t.Fatalf("refused pass changed the bucket: %d files", n)
	}

	// An empty source deleting everything is refused, even for a few files.
	empty := t.TempDir()
	putRemote(t, client, "sync-test", "few/a", "a")
	putRemote(t, client, "sync-test", "few/b", "b")
	if err := runSync(ctx, client, empty, "sync-test:few", true, false); err == nil {
		t.Fatal("deleting everything from an empty source not refused")
	}

	// Asked for, it goes ahead.
	if err := runSyncRequest(ctx, client, src, "sync-test:m", syncRequest{del: true, allowMassDelete: true}); err != nil {
		t.Fatal(err)
	}
	expectKeysWithPrefixIn(t, client, "sync-test", "m/", "m/fp.txt", "m/fq.txt", "m/fr.txt", "m/fs.txt", "m/ft.txt", "m/new.txt")
}

// A renamed file is not sent again: its content is copied, or reused once
// the old name has gone.
func TestSyncRenameReusesContent(t *testing.T) {
	client, store := newFilesServer(t)
	ctx := context.Background()
	src := t.TempDir()
	big := strings.Repeat("content ", 20000)
	writeFile(t, filepath.Join(src, "old.bin"), big)
	if err := runSync(ctx, client, src, "sync-test:r", false, false); err != nil {
		t.Fatal(err)
	}
	before, _ := store.HeadObject(admin, "tester--sync-test", "r/old.bin")

	os.Rename(filepath.Join(src, "old.bin"), filepath.Join(src, "new.bin"))
	if err := runSync(ctx, client, src, "sync-test:r", true, false); err != nil {
		t.Fatal(err)
	}
	expectKeysWithPrefixIn(t, client, "sync-test", "r/", "r/new.bin")
	after, _ := store.HeadObject(admin, "tester--sync-test", "r/new.bin")
	if after.SHA256 != before.SHA256 {
		t.Fatal("content differs")
	}

	// The content gone from the bucket is reused when it comes back.
	os.Rename(filepath.Join(src, "new.bin"), filepath.Join(src, "back.bin"))
	s := newSyncer(client, syncOptions{mode: modeUp, dir: src, bucket: "sync-test", prefix: "r/", del: true})
	if err := s.prepare(ctx); err != nil {
		t.Fatal(err)
	}
	s.loadRemote(ctx, false)
	s.scanLocal()
	// Delete first, as a watcher seeing the old name go would.
	s.mu.Lock()
	gone := s.remote["new.bin"]
	s.mu.Unlock()
	if err := s.apply(ctx, action{kind: actDeleteRemote, id: "new.bin", r: gone}); err != nil {
		t.Fatal(err)
	}
	var out logBuf
	s.opt.log = out.logger()
	s.log = s.opt.log
	if err := s.pass(ctx, []string{"back.bin"}); err != nil {
		t.Fatal(err)
	}
	if !logged(out.String(), "uploaded", `"content":"reused"`) {
		t.Errorf("content sent again: %q", out.String())
	}
	if c, _ := remoteContent(t, client, "sync-test", "r/back.bin"); c != big {
		t.Error("reused content differs")
	}
}

func TestSyncCaseInsensitiveFileSystem(t *testing.T) {
	dir := t.TempDir()
	if !caseInsensitive(dir) {
		t.Skip("the file system is case-sensitive")
	}
	client, _ := newFilesServer(t)
	ctx := context.Background()
	putRemote(t, client, "sync-test", "ci/Readme.md", "one")
	putRemote(t, client, "sync-test", "ci/README.md", "two")
	putRemote(t, client, "sync-test", "ci/other.txt", "o")
	if err := runSync(ctx, client, "sync-test:ci", dir, true, false); err != nil {
		t.Fatal(err)
	}
	// One of the two is kept; nothing is written over the other.
	files := localFiles(t, dir)
	if len(files) != 2 {
		t.Fatalf("local %v", files)
	}

	// A local file spelled differently is the bucket's file, not another.
	local := t.TempDir()
	writeFile(t, filepath.Join(local, "other.TXT"), "o")
	if err := runSync(ctx, client, "sync-test:ci", local, true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(local, "other.TXT")); err != nil {
		t.Errorf("differently spelled file removed: %v", err)
	}
}

func twoWay(t *testing.T, client *apiclient.ApiClient, dir, remote, state string, del bool) string {
	t.Helper()
	var out logBuf
	opt := syncOptions{mode: modeTwoWay, dir: dir, del: del, statePath: state, log: out.logger()}
	loc, _ := parseLocation(remote)
	opt.bucket, opt.prefix = loc.bucket, dirPrefix(loc.key)
	if err := runEngine(context.Background(), client, opt, nil); err != nil {
		t.Fatalf("two-way: %v\n%s", err, out.String())
	}
	return out.String()
}

func TestSyncTwoWay(t *testing.T) {
	client, _ := newFilesServer(t)
	dir := t.TempDir()
	state := filepath.Join(t.TempDir(), "state.json")

	// First run: each side gets what the other has; differing files are kept both ways.
	writeFile(t, filepath.Join(dir, "local.txt"), "from here")
	writeFile(t, filepath.Join(dir, "both.txt"), "same")
	writeFile(t, filepath.Join(dir, "clash.txt"), "local clash")
	putRemote(t, client, "sync-test", "tw/remote.txt", "from there")
	putRemote(t, client, "sync-test", "tw/both.txt", "same")
	putRemote(t, client, "sync-test", "tw/clash.txt", "remote clash")
	out := twoWay(t, client, dir, "sync-test:tw", state, false)
	if !logged(out, "conflict:", `"file":"clash.txt"`) {
		t.Errorf("no conflict reported:\n%s", out)
	}
	files := localFiles(t, dir)
	if len(files) != 5 {
		t.Fatalf("local after first run %v", files)
	}
	var aside string
	for _, f := range files {
		if strings.HasPrefix(f, "clash.conflict-") && strings.HasSuffix(f, ".txt") {
			aside = f
		}
	}
	if aside == "" || readLocal(t, filepath.Join(dir, aside)) != "local clash" || readLocal(t, filepath.Join(dir, "clash.txt")) != "remote clash" {
		t.Fatalf("conflict not kept both ways: %v", files)
	}
	if c, ok := remoteContent(t, client, "sync-test", "tw/"+aside); !ok || c != "local clash" {
		t.Errorf("conflict copy not uploaded")
	}
	expectKeysWithPrefixIn(t, client, "sync-test", "tw/", "tw/both.txt", "tw/"+aside, "tw/clash.txt", "tw/local.txt", "tw/remote.txt")

	// Nothing to do the second time.
	out = twoWay(t, client, dir, "sync-test:tw", state, false)
	if !logged(out, "synced", `"uploaded":0,"downloaded":0`) {
		t.Errorf("second run did something:\n%s", out)
	}

	// Edits on each side travel to the other.
	writeFile(t, filepath.Join(dir, "local.txt"), "edited here")
	putRemote(t, client, "sync-test", "tw/remote.txt", "edited there")
	twoWay(t, client, dir, "sync-test:tw", state, false)
	if c, _ := remoteContent(t, client, "sync-test", "tw/local.txt"); c != "edited here" {
		t.Errorf("local edit not uploaded: %q", c)
	}
	if c := readLocal(t, filepath.Join(dir, "remote.txt")); c != "edited there" {
		t.Errorf("remote edit not downloaded: %q", c)
	}

	// Without --delete, a deletion is put back from the other side.
	os.Remove(filepath.Join(dir, "both.txt"))
	client.DeleteFileObject(context.Background(), "sync-test", "tw/local.txt")
	twoWay(t, client, dir, "sync-test:tw", state, false)
	if readLocal(t, filepath.Join(dir, "both.txt")) != "same" {
		t.Error("local deletion not put back")
	}
	if c, ok := remoteContent(t, client, "sync-test", "tw/local.txt"); !ok || c != "edited here" {
		t.Error("bucket deletion not put back")
	}

	// With --delete, deletions pass across.
	os.Remove(filepath.Join(dir, "both.txt"))
	client.DeleteFileObject(context.Background(), "sync-test", "tw/local.txt")
	twoWay(t, client, dir, "sync-test:tw", state, true)
	if _, err := os.Stat(filepath.Join(dir, "local.txt")); !os.IsNotExist(err) {
		t.Error("bucket deletion not made locally")
	}
	if _, ok := remoteContent(t, client, "sync-test", "tw/both.txt"); ok {
		t.Error("local deletion not made in the bucket")
	}

	// Deleted on one side and changed on the other: the change wins.
	writeFile(t, filepath.Join(dir, "remote.txt"), "changed here")
	client.DeleteFileObject(context.Background(), "sync-test", "tw/remote.txt")
	twoWay(t, client, dir, "sync-test:tw", state, true)
	if c, ok := remoteContent(t, client, "sync-test", "tw/remote.txt"); !ok || c != "changed here" {
		t.Errorf("changed file lost to a deletion: %q %v", c, ok)
	}

	// Changed on both sides: kept both ways again.
	writeFile(t, filepath.Join(dir, "remote.txt"), "here v2")
	putRemote(t, client, "sync-test", "tw/remote.txt", "there v2")
	out = twoWay(t, client, dir, "sync-test:tw", state, true)
	if !logged(out, "conflict:", `"file":"remote.txt"`) || readLocal(t, filepath.Join(dir, "remote.txt")) != "there v2" {
		t.Errorf("conflict not handled:\n%s", out)
	}

	// The state belongs to this pair only.
	other := t.TempDir()
	opt := syncOptions{mode: modeTwoWay, dir: other, bucket: "sync-test", prefix: "tw/", statePath: state, log: (&logBuf{}).logger()}
	if err := runEngine(context.Background(), client, opt, nil); err == nil || !strings.Contains(err.Error(), "belongs to") {
		t.Errorf("state used for another directory: %v", err)
	}
}

func TestSyncTwoWayDetectsRemoteChangeDuringUpload(t *testing.T) {
	client, _ := newFilesServer(t)
	dir := t.TempDir()
	state := filepath.Join(t.TempDir(), "state.json")
	writeFile(t, filepath.Join(dir, "f.txt"), "v1")
	twoWay(t, client, dir, "sync-test:race", state, false)

	// Plan against the bucket as it was, then change it before the upload.
	writeFile(t, filepath.Join(dir, "f.txt"), "local v2")
	s := newSyncer(client, syncOptions{mode: modeTwoWay, dir: dir, bucket: "sync-test", prefix: "race/", statePath: state, log: (&logBuf{}).logger()})
	if err := s.prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.loadRemote(context.Background(), true)
	s.scanLocal()
	plan := s.plan([]string{"f.txt"})
	if len(plan) != 1 || plan[0].kind != actUpload {
		t.Fatalf("plan %+v", plan)
	}
	putRemote(t, client, "sync-test", "race/f.txt", "remote v2")
	s.apply(context.Background(), plan[0])
	if c, _ := remoteContent(t, client, "sync-test", "race/f.txt"); c != "remote v2" {
		t.Fatalf("upload overwrote a change made meanwhile: %q", c)
	}
	// The next run sees both changed.
	out := twoWay(t, client, dir, "sync-test:race", state, false)
	if !logged(out, "conflict:", `"file":"f.txt"`) {
		t.Errorf("no conflict after the race:\n%s", out)
	}
}

func TestConflictName(t *testing.T) {
	at := time.Date(2026, 10, 10, 15, 30, 0, 0, time.UTC)
	for in, want := range map[string]string{
		"a/notes.txt": "a/notes.conflict-H-20261010-153000.txt",
		"Makefile":    "Makefile.conflict-H-20261010-153000",
		"d/.env":      "d/.env.conflict-H-20261010-153000",
		"x.tar.gz":    "x.tar.conflict-H-20261010-153000.gz",
	} {
		got := conflictName(in, at)
		host, _ := os.Hostname()
		host = strings.Split(host, ".")[0]
		if host == "" {
			host = "local"
		}
		if got != strings.ReplaceAll(want, "-H-", "-"+strings.NewReplacer(" ", "-", ":", "-").Replace(host)+"-") {
			t.Errorf("%s: %s", in, got)
		}
	}
}

// fakeEvents stands in for the server's event stream.
type fakeEvents struct {
	mu   sync.Mutex
	subs []func(apiclient.Event)
}

func (f *fakeEvents) follow(ctx context.Context, onEvent func(apiclient.Event), onState func(bool)) error {
	f.mu.Lock()
	f.subs = append(f.subs, onEvent)
	f.mu.Unlock()
	onState(true)
	<-ctx.Done()
	return ctx.Err()
}

func (f *fakeEvents) filesChanged() {
	payload, _ := json.Marshal(apiclient.FilesChangedPayload{})
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.subs {
		s(apiclient.Event{Type: "files:changed", Payload: payload})
	}
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func startWatch(t *testing.T, client *apiclient.ApiClient, src, dst string, req syncRequest) *fakeEvents {
	t.Helper()
	ev := &fakeEvents{}
	req.watch, req.follow = true, ev.follow
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runSyncRequest(ctx, client, src, dst, req) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("watch did not stop")
		}
	})
	return ev
}

func TestSyncWatchUp(t *testing.T) {
	client, _ := newFilesServer(t)
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "first.txt"), "1")
	startWatch(t, client, src, "sync-test:wu", syncRequest{del: true})
	has := func(key, content string) func() bool {
		return func() bool { c, ok := remoteContent(t, client, "sync-test", key); return ok && c == content }
	}
	waitUntil(t, "initial pass", has("wu/first.txt", "1"))

	writeFile(t, filepath.Join(src, "sub", "new.txt"), "new")
	waitUntil(t, "new file in a new folder", has("wu/sub/new.txt", "new"))
	writeFile(t, filepath.Join(src, "first.txt"), "changed")
	waitUntil(t, "change", has("wu/first.txt", "changed"))
	os.Rename(filepath.Join(src, "first.txt"), filepath.Join(src, "renamed.txt"))
	waitUntil(t, "rename", func() bool {
		_, gone := remoteContent(t, client, "sync-test", "wu/first.txt")
		return has("wu/renamed.txt", "changed")() && !gone
	})
	// Default excludes apply while watching.
	writeFile(t, filepath.Join(src, ".git", "HEAD"), "ref")
	writeFile(t, filepath.Join(src, "after.txt"), "a")
	waitUntil(t, "file after .git", has("wu/after.txt", "a"))
	if _, ok := remoteContent(t, client, "sync-test", "wu/.git/HEAD"); ok {
		t.Error(".git synced while watching")
	}
}

func TestSyncWatchDown(t *testing.T) {
	client, _ := newFilesServer(t)
	dst := t.TempDir()
	putRemote(t, client, "sync-test", "wd/a.txt", "a")
	ev := startWatch(t, client, "sync-test:wd", dst, syncRequest{del: true})
	waitUntil(t, "initial pass", func() bool { _, err := os.Stat(filepath.Join(dst, "a.txt")); return err == nil })

	putRemote(t, client, "sync-test", "wd/b/c.txt", "c")
	ev.filesChanged()
	waitUntil(t, "new file", func() bool { d, _ := os.ReadFile(filepath.Join(dst, "b", "c.txt")); return string(d) == "c" })

	client.DeleteFileObject(context.Background(), "sync-test", "wd/b/c.txt")
	putRemote(t, client, "sync-test", "wd/a.txt", "a2")
	ev.filesChanged()
	waitUntil(t, "change and delete", func() bool {
		d, _ := os.ReadFile(filepath.Join(dst, "a.txt"))
		_, err := os.Stat(filepath.Join(dst, "b"))
		return string(d) == "a2" && os.IsNotExist(err)
	})
}

func TestSyncWatchTwoWay(t *testing.T) {
	client, _ := newFilesServer(t)
	dir := t.TempDir()
	putRemote(t, client, "sync-test", "w2/remote.txt", "r")
	writeFile(t, filepath.Join(dir, "local.txt"), "l")
	ev := startWatch(t, client, dir, "sync-test:w2", syncRequest{twoWay: true, del: true, statePath: filepath.Join(t.TempDir(), "s.json")})
	waitUntil(t, "initial pass", func() bool {
		_, ok := remoteContent(t, client, "sync-test", "w2/local.txt")
		_, err := os.Stat(filepath.Join(dir, "remote.txt"))
		return ok && err == nil
	})

	// Local edit up, bucket edit down, each without echoing back.
	writeFile(t, filepath.Join(dir, "local.txt"), "l2")
	waitUntil(t, "local edit", func() bool { c, _ := remoteContent(t, client, "sync-test", "w2/local.txt"); return c == "l2" })
	putRemote(t, client, "sync-test", "w2/remote.txt", "r2")
	ev.filesChanged()
	waitUntil(t, "bucket edit", func() bool { return readLocal(t, filepath.Join(dir, "remote.txt")) == "r2" })

	// Deletions both ways.
	os.Remove(filepath.Join(dir, "local.txt"))
	waitUntil(t, "local delete", func() bool { _, ok := remoteContent(t, client, "sync-test", "w2/local.txt"); return !ok })
	client.DeleteFileObject(context.Background(), "sync-test", "w2/remote.txt")
	ev.filesChanged()
	waitUntil(t, "bucket delete", func() bool { _, err := os.Stat(filepath.Join(dir, "remote.txt")); return os.IsNotExist(err) })

	// Settled: no stray files or conflicts.
	time.Sleep(time.Second)
	if files := localFiles(t, dir); len(files) != 0 {
		t.Errorf("left %v", files)
	}
}

func TestSyncTwoWayRenameCopies(t *testing.T) {
	client, _ := newFilesServer(t)
	dir := t.TempDir()
	state := filepath.Join(t.TempDir(), "state.json")
	writeFile(t, filepath.Join(dir, "a.bin"), strings.Repeat("A", 100000))
	twoWay(t, client, dir, "sync-test:mv", state, true)
	os.Rename(filepath.Join(dir, "a.bin"), filepath.Join(dir, "b.bin"))
	out := twoWay(t, client, dir, "sync-test:mv", state, true)
	if !logged(out, "uploaded", `"file":"b.bin"`, `"content":"copied in the bucket"`) {
		t.Errorf("rename sent the content:\n%s", out)
	}
	expectKeysWithPrefixIn(t, client, "sync-test", "mv/", "mv/b.bin")

	// The copy is conditional: a file that appeared at the new name meanwhile
	// is not overwritten.
	_, err := client.CopyFileObject(context.Background(), apiclient.FileCopyRequest{SourceBucket: "sync-test", SourceKey: "mv/b.bin", DestBucket: "sync-test", DestKey: "mv/b.bin2", IfNoneMatch: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CopyFileObject(context.Background(), apiclient.FileCopyRequest{SourceBucket: "sync-test", SourceKey: "mv/b.bin", DestBucket: "sync-test", DestKey: "mv/b.bin2", IfNoneMatch: true})
	if !apiclient.IsPreconditionFailed(err) {
		t.Errorf("conditional copy over a file: %v", err)
	}
}

// A file changed in the bucket after a pass planned to delete it is kept,
// and the next pass brings the change down.
func TestSyncTwoWayDeleteRace(t *testing.T) {
	client, _ := newFilesServer(t)
	dir := t.TempDir()
	state := filepath.Join(t.TempDir(), "state.json")
	writeFile(t, filepath.Join(dir, "f.txt"), "v1")
	twoWay(t, client, dir, "sync-test:dr", state, true)

	os.Remove(filepath.Join(dir, "f.txt"))
	s := newSyncer(client, syncOptions{mode: modeTwoWay, dir: dir, bucket: "sync-test", prefix: "dr/", del: true, statePath: state, log: (&logBuf{}).logger()})
	if err := s.prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.loadRemote(context.Background(), true)
	s.scanLocal()
	plan := s.plan([]string{"f.txt"})
	if len(plan) != 1 || plan[0].kind != actDeleteRemote {
		t.Fatalf("plan %+v", plan)
	}
	putRemote(t, client, "sync-test", "dr/f.txt", "changed there")
	if err := s.apply(context.Background(), plan[0]); err != nil {
		t.Fatal(err)
	}
	if c, ok := remoteContent(t, client, "sync-test", "dr/f.txt"); !ok || c != "changed there" {
		t.Fatalf("a change made meanwhile was deleted: %q %v", c, ok)
	}
	twoWay(t, client, dir, "sync-test:dr", state, true)
	if readLocal(t, filepath.Join(dir, "f.txt")) != "changed there" {
		t.Error("the change did not come down")
	}
}
