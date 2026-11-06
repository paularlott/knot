package command_files

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/syncproject"
)

func putProjectFile(t *testing.T, client *apiclient.ApiClient, key, content string) {
	t.Helper()
	if _, err := client.PutFileObject(context.Background(), "sync-test", key, strings.NewReader(content), int64(len(content)), "", apiclient.FileAttrs{}); err != nil {
		t.Fatalf("put %s: %v", key, err)
	}
}

func parseProject(t *testing.T, toml string) *syncproject.Project {
	t.Helper()
	p, err := syncproject.Parse([]byte(toml), "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := checkProject(p); err != nil {
		t.Fatal(err)
	}
	return p
}

func fixedClient(c *apiclient.ApiClient) clientFunc {
	return func() (*apiclient.ApiClient, error) { return c, nil }
}

// A job with a snapshot starts its folder from the snapshot the first time,
// and syncs the folder as it is from then on: a new space gets the work,
// not the starter again.
func TestSyncProjectSnapshotOnlyWhenTheFolderIsNew(t *testing.T) {
	client, _ := newFilesServer(t)
	ctx := context.Background()
	putProjectFile(t, client, "templates/go/main.go", "package main")
	putProjectFile(t, client, "templates/go/README.md", "starter")

	first := t.TempDir()
	project := func(dir string) *syncproject.Project {
		return parseProject(t, `
[[sync]]
local = "`+filepath.ToSlash(dir)+`"
remote = "sync-test:projects/foo/"
snapshot = "sync-test:templates/go/"
watch = false
`)
	}
	if err := runProject(ctx, fixedClient(client), project(first), nil); err != nil {
		t.Fatal(err)
	}
	if got := readLocal(t, filepath.Join(first, "main.go")); got != "package main" {
		t.Fatalf("main.go = %q", got)
	}
	if _, err := os.Stat(filepath.Join(first, snapshotManifest)); !os.IsNotExist(err) {
		t.Fatalf("the snapshot's manifest came down: %v", err)
	}
	if !slices.Contains(remoteKeys(t, client), "projects/foo/"+snapshotManifest) {
		t.Fatal("no snapshot made of the folder")
	}

	// Work in the space, sent up; the starter changes after.
	writeFile(t, filepath.Join(first, "main.go"), "package main // edited")
	if err := runProject(ctx, fixedClient(client), project(first), nil); err != nil {
		t.Fatal(err)
	}
	putProjectFile(t, client, "templates/go/main.go", "package main // new starter")

	// A new space: the work comes back, not the starter.
	second := t.TempDir()
	if err := runProject(ctx, fixedClient(client), project(second), nil); err != nil {
		t.Fatal(err)
	}
	if got := readLocal(t, filepath.Join(second, "main.go")); got != "package main // edited" {
		t.Fatalf("a new space got main.go = %q, not the work", got)
	}
}

// A job that fails leaves the project's other jobs to run; the run reports
// the failure.
func TestSyncProjectJobFailsAlone(t *testing.T) {
	client, _ := newFilesServer(t)
	good, bad := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(good, "a.txt"), "a")
	p := parseProject(t, `
[[sync]]
name = "good"
local = "`+filepath.ToSlash(good)+`"
remote = "sync-test:good/"
watch = false

[[sync]]
name = "bad"
local = "`+filepath.ToSlash(bad)+`"
remote = "sync-test:bad/"
snapshot = "sync-test:no-such-folder/"
watch = false
`)
	err := runProject(context.Background(), fixedClient(client), p, nil)
	if err == nil {
		t.Fatal("a failed job was not reported")
	}
	if !slices.Contains(remoteKeys(t, client), "good/a.txt") {
		t.Fatal("the good job did not sync")
	}
}

// exclude_from reads the file from the directory, or from the bucket folder
// when it has not come down yet.
func TestSyncProjectExcludeFrom(t *testing.T) {
	client, _ := newFilesServer(t)
	putProjectFile(t, client, "app/.gitignore", "# build output\n/build/\n*.log\n!keep.log\n")
	putProjectFile(t, client, "app/src.go", "src")
	putProjectFile(t, client, "app/build/out", "binary")
	putProjectFile(t, client, "app/debug.log", "log")
	dir := t.TempDir()
	p := parseProject(t, `
[[sync]]
local = "`+filepath.ToSlash(dir)+`"
remote = "sync-test:app/"
mode = "download"
watch = false
exclude_from = ".gitignore"
`)
	if err := runProject(context.Background(), fixedClient(client), p, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "src.go")); err != nil {
		t.Fatal("src.go did not come down")
	}
	for _, f := range []string{"build/out", "debug.log"} {
		if _, err := os.Stat(filepath.Join(dir, f)); !os.IsNotExist(err) {
			t.Errorf("%s came down though the .gitignore leaves it out", f)
		}
	}
}

func jobState(m *ProjectManager, project, job string) syncproject.JobStatus {
	for _, p := range m.List() {
		if p.Name != project {
			continue
		}
		for _, j := range p.Jobs {
			if j.Name == job {
				return j
			}
		}
	}
	return syncproject.JobStatus{}
}

// The agent's projects: started ones run and are kept to start again; the
// template's runs too, is not restarted for an unchanged registration, and
// cannot be stopped.
func TestProjectManager(t *testing.T) {
	client, _ := newFilesServer(t)
	ev := &fakeEvents{}
	keep := t.TempDir()
	newManager := func(ctx context.Context) *ProjectManager {
		m := NewProjectManager(ctx, keep, fixedClient(client), (&logBuf{}).logger())
		m.follow = ev.follow
		return m
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := newManager(ctx)
	defer m.StopAll()

	work := t.TempDir()
	content := `
name = "work"
[[sync]]
name = "src"
local = "` + filepath.ToSlash(work) + `"
remote = "sync-test:work/"
`
	if _, err := m.Start(content, "ignored"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the job watching", func() bool { return jobState(m, "work", "src").State == "watching" })
	writeFile(t, filepath.Join(work, "a.txt"), "a")
	waitUntil(t, "a.txt sent", func() bool { return slices.Contains(remoteKeys(t, client), "work/a.txt") })
	if _, err := os.Stat(filepath.Join(keep, "work.toml")); err != nil {
		t.Fatal("the started project was not kept")
	}

	// A second project syncing the same directory is refused.
	if _, err := m.Start(strings.Replace(content, `"work"`, `"other"`, 1), ""); err == nil {
		t.Fatal("a second project syncing the same directory was started")
	}
	// The template's name is the template's.
	if _, err := m.Start(strings.Replace(content, `"work"`, `"template"`, 1), ""); err == nil {
		t.Fatal("a project named template was started")
	}

	// The template's project.
	tmplDir := t.TempDir()
	tp := &syncproject.Project{Name: syncproject.TemplateProject, Sync: []syncproject.Job{{Name: "t", Local: tmplDir, Remote: "sync-test:tmpl/"}}}
	if err := tp.Validate(); err != nil {
		t.Fatal(err)
	}
	m.SetTemplate(tp)
	waitUntil(t, "the template's job watching", func() bool { return jobState(m, "template", "t").State == "watching" })
	m.mu.Lock()
	run := m.runs[syncproject.TemplateProject]
	m.mu.Unlock()
	same := *tp
	m.SetTemplate(&same)
	m.mu.Lock()
	again := m.runs[syncproject.TemplateProject]
	m.mu.Unlock()
	if run != again {
		t.Fatal("an unchanged template project was started again")
	}
	if err := m.Stop(syncproject.TemplateProject); err == nil {
		t.Fatal("the template's project was stopped")
	}

	// The agent stops and starts: the started project runs again, from
	// where it was kept.
	m.StopAll()
	m2 := newManager(ctx)
	defer m2.StopAll()
	m2.Resume()
	waitUntil(t, "the kept project watching again", func() bool { return jobState(m2, "work", "src").State == "watching" })
	writeFile(t, filepath.Join(work, "b.txt"), "b")
	waitUntil(t, "b.txt sent", func() bool { return slices.Contains(remoteKeys(t, client), "work/b.txt") })

	// Stopped, it is forgotten.
	if err := m2.Stop("work"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(keep, "work.toml")); !os.IsNotExist(err) {
		t.Fatal("a stopped project was kept")
	}
	if err := m2.Stop("work"); err == nil {
		t.Fatal("stopping a project not running did not fail")
	}
}

// A watching job that cannot start (the agent not yet registered) starts
// once it can.
func TestSyncProjectRetriesUntilItCanStart(t *testing.T) {
	client, _ := newFilesServer(t)
	defer func(min time.Duration) { jobRetryMin = min }(jobRetryMin)
	jobRetryMin = 50 * time.Millisecond
	var ready = make(chan struct{})
	cf := func() (*apiclient.ApiClient, error) {
		select {
		case <-ready:
			return client, nil
		default:
			return nil, os.ErrNotExist
		}
	}
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "a")
	p := parseProject(t, `
[[sync]]
local = "`+filepath.ToSlash(dir)+`"
remote = "sync-test:later/"
`)
	ev := &fakeEvents{}
	r := startProject(context.Background(), cf, p, false, (&logBuf{}).logger(), ev.follow)
	defer r.stop()
	waitUntil(t, "the job failing", func() bool { return r.status().Jobs[0].State == "failed" })
	close(ready)
	waitUntil(t, "a.txt sent", func() bool { return slices.Contains(remoteKeys(t, client), "later/a.txt") })
}

// As knot file sync, a project leaves out .git and editor scratch files
// unless told to sync everything but its own patterns, in a job or for the
// whole project.
func TestSyncProjectDefaultExcludes(t *testing.T) {
	client, _ := newFilesServer(t)
	for name, settings := range map[string][2]string{
		"defaults": {"", ""},
		"job":      {"", "no_default_excludes = true\n"},
		"project":  {"no_default_excludes = true\n", ""},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "main.go"), "m")
			writeFile(t, filepath.Join(dir, ".git", "HEAD"), "ref")
			writeFile(t, filepath.Join(dir, "build", "out"), "b")
			p := parseProject(t, settings[0]+`exclude = ["build/"]
[[sync]]
local = "`+filepath.ToSlash(dir)+`"
remote = "sync-test:`+name+`/"
watch = false
`+settings[1])
			if err := runProject(context.Background(), fixedClient(client), p, nil); err != nil {
				t.Fatal(err)
			}
			keys := remoteKeys(t, client)
			if !slices.Contains(keys, name+"/main.go") {
				t.Fatal("main.go not synced")
			}
			if slices.Contains(keys, name+"/build/out") {
				t.Error("the project's own exclusion was synced")
			}
			if got, want := slices.Contains(keys, name+"/.git/HEAD"), name != "defaults"; got != want {
				t.Errorf(".git synced: %v, want %v", got, want)
			}
		})
	}
}

// A watching sync stopped straight after a change sends it, rather than
// leaving it behind.
func TestSyncWatchSendsChangesWhenStopped(t *testing.T) {
	client, _ := newFilesServer(t)
	dir := t.TempDir()
	p := parseProject(t, `
[[sync]]
local = "`+filepath.ToSlash(dir)+`"
remote = "sync-test:stopped/"
`)
	r := startProject(context.Background(), fixedClient(client), p, false, (&logBuf{}).logger(), (&fakeEvents{}).follow)
	waitUntil(t, "watching", func() bool { return r.status().Jobs[0].State == "watching" })
	writeFile(t, filepath.Join(dir, "last.txt"), "last edit")
	r.stop()
	if !slices.Contains(remoteKeys(t, client), "stopped/last.txt") {
		t.Fatal("a change made just before the sync stopped was not sent")
	}
}

// A file written while the first pass runs is sent at once, not left for
// the next look at everything minutes later, and without walking the
// directory a second time to find it: the watcher starts first.
func TestSyncWatchSeesChangesMadeWhileStarting(t *testing.T) {
	client, _ := newFilesServer(t)
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	var once sync.Once
	var out logBuf
	opt := syncOptions{
		mode: modeTwoWay, dir: dir, bucket: "sync-test", prefix: "starting/",
		statePath: filepath.Join(t.TempDir(), "state"), log: out.logger(), watching: true,
		onPass: func(passCounts, error) {
			once.Do(func() { writeFile(t, filepath.Join(dir, "early.txt"), "early") })
		},
	}
	done := make(chan error, 1)
	go func() { done <- runEngine(ctx, client, opt, (&fakeEvents{}).follow) }()
	defer func() { cancel(); <-done }()
	waitUntil(t, "early.txt sent", func() bool { return slices.Contains(remoteKeys(t, client), "starting/early.txt") })
	if logged(out.String(), "looking at every local file") {
		t.Errorf("the directory was walked again after the first pass:\n%s", out.String())
	}
}

// Asked to flush, as a space is about to stop, every watching job sends its
// local changes at once, before it would have noticed them.
func TestProjectManagerFlush(t *testing.T) {
	client, _ := newFilesServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := NewProjectManager(ctx, t.TempDir(), fixedClient(client), (&logBuf{}).logger())
	m.follow = (&fakeEvents{}).follow
	defer m.StopAll()
	a, b, down := t.TempDir(), t.TempDir(), t.TempDir()
	if _, err := m.Start(`
name = "p"
[[sync]]
name = "a"
local = "`+filepath.ToSlash(a)+`"
remote = "sync-test:a/"
[[sync]]
name = "b"
local = "`+filepath.ToSlash(b)+`"
remote = "sync-test:b/"
mode = "upload"
[[sync]]
name = "down"
local = "`+filepath.ToSlash(down)+`"
remote = "sync-test:down/"
mode = "download"
`, ""); err != nil {
		t.Fatal(err)
	}
	for _, j := range []string{"a", "b", "down"} {
		waitUntil(t, j+" watching", func() bool { return jobState(m, "p", j).State == "watching" })
	}
	writeFile(t, filepath.Join(a, "x.txt"), "x")
	writeFile(t, filepath.Join(b, "sub", "y.txt"), "y")
	fctx, fcancel := context.WithTimeout(ctx, 10*time.Second)
	defer fcancel()
	if err := m.Flush(fctx); err != nil {
		t.Fatal(err)
	}
	keys := remoteKeys(t, client)
	for _, k := range []string{"a/x.txt", "b/sub/y.txt"} {
		if !slices.Contains(keys, k) {
			t.Errorf("%s not sent by the flush", k)
		}
	}
	// The jobs carry on watching after.
	writeFile(t, filepath.Join(a, "later.txt"), "l")
	waitUntil(t, "later.txt sent", func() bool { return slices.Contains(remoteKeys(t, client), "a/later.txt") })
}

func snapshotLocation(t *testing.T, remote string) location {
	t.Helper()
	l, err := requireRemote(remote)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// Two spaces starting at once on the same new folder: one snapshot is made
// and both sync it.
func TestEnsureSnapshotRace(t *testing.T) {
	client, _ := newFilesServer(t)
	for i := range 20 {
		putProjectFile(t, client, fmt.Sprintf("starter/f%02d", i), "s")
	}
	ctx := context.Background()
	errs := make(chan error, 4)
	for range 4 {
		go func() {
			errs <- ensureSnapshot(ctx, client, "sync-test:starter/", snapshotLocation(t, "sync-test:race/"), (&logBuf{}).logger())
		}()
	}
	for range 4 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	keys := remoteKeys(t, client)
	n := 0
	for _, k := range keys {
		if strings.HasPrefix(k, "race/") && !strings.HasSuffix(k, "/") {
			n++
		}
	}
	if n != 21 {
		t.Fatalf("race/ holds %d files, want the 20 and the manifest", n)
	}
}

// A folder without a manifest: one whose manifest comes is synced once it
// does; one that goes, its snapshot having failed, is made again; one that
// never gets one is synced as it is after a while.
func TestEnsureSnapshotWaits(t *testing.T) {
	client, _ := newFilesServer(t)
	defer func(w, p time.Duration) { snapshotWaitFor, snapshotWaitPoll = w, p }(snapshotWaitFor, snapshotWaitPoll)
	snapshotWaitFor, snapshotWaitPoll = 3*time.Second, 50*time.Millisecond
	putProjectFile(t, client, "starter/a", "a")
	ctx := context.Background()
	l := (&logBuf{}).logger()

	// The manifest comes.
	putProjectFile(t, client, "coming/a", "a")
	go func() {
		time.Sleep(300 * time.Millisecond)
		putProjectFile(t, client, "coming/"+snapshotManifest, "{}")
	}()
	start := time.Now()
	if err := ensureSnapshot(ctx, client, "sync-test:starter/", snapshotLocation(t, "sync-test:coming/"), l); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("waited %v for a manifest that came after 300ms", took)
	}

	// The folder goes: it is made from the snapshot.
	putProjectFile(t, client, "going/half", "h")
	go func() {
		time.Sleep(300 * time.Millisecond)
		client.DeleteFileObject(ctx, "sync-test", "going/half")
	}()
	if err := ensureSnapshot(ctx, client, "sync-test:starter/", snapshotLocation(t, "sync-test:going/"), l); err != nil {
		t.Fatal(err)
	}
	if keys := remoteKeys(t, client); !slices.Contains(keys, "going/a") || !slices.Contains(keys, "going/"+snapshotManifest) {
		t.Fatalf("a folder that went was not made from the snapshot: %v", keys)
	}

	// None comes: the folder is synced as it is, after the wait.
	putProjectFile(t, client, "plain/own", "o")
	buf := &logBuf{}
	start = time.Now()
	if err := ensureSnapshot(ctx, client, "sync-test:starter/", snapshotLocation(t, "sync-test:plain/"), buf.logger()); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took < snapshotWaitFor {
		t.Fatalf("gave up after %v, before the wait", took)
	}
	if !strings.Contains(buf.String(), "no snapshot manifest") {
		t.Fatal("no warning that the folder was not made by a snapshot")
	}
	if slices.Contains(remoteKeys(t, client), "plain/a") {
		t.Fatal("a snapshot was made over a folder that existed")
	}
}

// A changed template project replaces the one running; none stops it.
func TestProjectManagerTemplateChanges(t *testing.T) {
	client, _ := newFilesServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := NewProjectManager(ctx, t.TempDir(), fixedClient(client), (&logBuf{}).logger())
	m.follow = (&fakeEvents{}).follow
	defer m.StopAll()
	one, two := t.TempDir(), t.TempDir()
	tp := func(dir, remote string) *syncproject.Project {
		p := &syncproject.Project{Name: syncproject.TemplateProject, Sync: []syncproject.Job{{Local: dir, Remote: remote}}}
		if err := p.Validate(); err != nil {
			t.Fatal(err)
		}
		return p
	}
	m.SetTemplate(tp(one, "sync-test:one/"))
	waitUntil(t, "the first watching", func() bool { return jobState(m, "template", filepath.Base(one)).State == "watching" })
	m.SetTemplate(tp(two, "sync-test:two/"))
	waitUntil(t, "the second watching", func() bool { return jobState(m, "template", filepath.Base(two)).State == "watching" })
	if st := jobState(m, "template", filepath.Base(one)); st.Name != "" {
		t.Fatalf("the old template job is still listed: %+v", st)
	}
	writeFile(t, filepath.Join(one, "old.txt"), "o")
	writeFile(t, filepath.Join(two, "new.txt"), "n")
	waitUntil(t, "new.txt sent", func() bool { return slices.Contains(remoteKeys(t, client), "two/new.txt") })
	time.Sleep(time.Second)
	if slices.Contains(remoteKeys(t, client), "one/old.txt") {
		t.Fatal("the replaced template job still syncs")
	}
	m.SetTemplate(nil)
	if len(m.List()) != 0 {
		t.Fatalf("the template project runs on with none: %+v", m.List())
	}
}
