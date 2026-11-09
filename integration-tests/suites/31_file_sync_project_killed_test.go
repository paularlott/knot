//go:build integration

package suites

import (
	"context"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/integration-tests/harness"
)

// syncWriter is the script a space runs to write ~/work as a developer
// would, only faster: new files f00001, f00002, ... in turn, each with its
// number, and counter rewritten after each, until ~/stop-writing exists.
const syncWriter = `#!/bin/sh
mkdir -p ~/work
i=${1:-1}
while [ ! -e ~/stop-writing ]; do
  n=$(printf '%05d' $i)
  echo "file $n" > ~/work/f$n.tmp && mv ~/work/f$n.tmp ~/work/f$n
  echo $i > ~/work/counter
  i=$((i+1))
  sleep 0.05
done
`

// TestFileSyncProjectKilled has a space write files into a folder its
// template keeps in step with a bucket, without pause, while the knot
// server is killed (SIGKILL) and started again, the space's agent is
// killed (SIGKILL), taking its container with it, which its runtime starts
// again, and the server is restarted: once the writing stops, the bucket
// holds every file as the space does, without waiting on the sync's
// five-minute look at everything. Then the space is restarted
// while writing: the new container gets every file written before the
// restart was asked for, with none missing in between.
func TestFileSyncProjectKilled(t *testing.T) {
	harness.Feature(t, "file-storage")
	runtime := cfg.Runtime
	if runtime != "docker" && runtime != "podman" {
		t.Skipf("needs docker or podman to kill the space's agent, not %s", runtime)
	}
	s, adminUser := bootDedicated(t, "synckilled", "--files-path", t.TempDir())
	ctx, cancel := testCtx(1200)
	defer cancel()
	if _, err := adminUser.Client.CreateFileBucket(ctx, "code"); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	tmpl, err := harness.CreateTemplate(s, adminUser.Client, uniqueName("it_sync_killed"), harness.TemplateOptions{
		Volumes: `sync:
  - name: work
    local: ~/work
    remote: code:work/
`,
	})
	if err != nil {
		t.Fatal(err)
	}
	space := harness.CreateSpace(t, adminUser.Client, "it-sync-killed", tmpl, adminUser.Id)
	harness.DeleteSpaceAsync(t, adminUser.Client, space)
	harness.WaitForSpaceReady(t, s, adminUser.Client, space)
	ready := func() {
		t.Helper()
		if !waitForCond(120, func() bool {
			out, err := harness.TryRunCommand(adminUser.Client, space, 10, "echo", "up")
			return err == nil && strings.Contains(out, "up")
		}) {
			t.Fatal("the space's agent did not come back")
		}
	}
	// The writer runs apart from the agent, started in the container as the
	// space's user, so killing the agent leaves it writing.
	startWriter := func(from int) {
		t.Helper()
		writeSpaceFileAs(t, adminUser, space, "/tmp/writer.sh", syncWriter)
		cid := spaceContainer(t, runtime, space)
		if out, err := exec.Command(runtime, "exec", "-d", cid, "sh", "-c",
			`chmod +x /tmp/writer.sh; exec gosu "$KNOT_USER" sh -c 'rm -f ~/stop-writing; exec /tmp/writer.sh `+strconv.Itoa(from)+`'`).CombinedOutput(); err != nil {
			t.Fatalf("start the writer: %v: %s", err, out)
		}
	}
	counter := func() int {
		out, _ := harness.TryRunCommand(adminUser.Client, space, 10, "sh", "-c", "cat ~/work/counter 2>/dev/null")
		n, _ := strconv.Atoi(strings.TrimSpace(out))
		return n
	}
	pause := func() { time.Sleep(4 * time.Second) }

	startWriter(1)
	pause()
	if counter() == 0 {
		t.Fatal("the writer is not writing")
	}

	// The server is killed and comes back.
	t.Log("killing the server")
	if err := s.Crash(); err != nil {
		t.Fatal(err)
	}
	ready()
	pause()

	// The space's agent is killed: the image's entrypoint waits on it, so
	// the container stops with it, abruptly, with no chance to send what
	// changed, and its runtime starts it again, its files as they were.
	// The writer goes with it, and starts again where it stopped.
	t.Log("killing the space's agent, and so its container")
	cid := spaceContainer(t, runtime, space)
	startedAt := func() string {
		out, _ := exec.Command(runtime, "inspect", "--format", "{{.State.StartedAt}}", cid).Output()
		return strings.TrimSpace(string(out))
	}
	startedBefore := startedAt()
	if out, err := exec.Command(runtime, "exec", cid, "pkill", "-9", "-f", "knot agent start").CombinedOutput(); err != nil {
		t.Fatalf("kill the agent: %v: %s", err, out)
	}
	if !waitForCond(60, func() bool { s := startedAt(); return s != "" && s != startedBefore }) {
		t.Fatal("the space's container was not started again")
	}
	ready()
	startWriter(counter() + 1)
	pause()

	// The server is restarted.
	t.Log("restarting the server")
	if err := s.Restart(); err != nil {
		t.Fatal(err)
	}
	ready()
	pause()

	if out, _ := exec.Command(runtime, "exec", cid, "pgrep", "-f", "/tmp/writer.sh").Output(); strings.TrimSpace(string(out)) == "" {
		t.Fatal("the writer stopped during the server's kill or restart")
	}
	harness.RunCommand(t, adminUser.Client, space, 10, "sh", "-c", "touch ~/stop-writing")
	time.Sleep(time.Second)
	written := counter()
	if written < 100 {
		t.Fatalf("the writer wrote %d files: too few to show anything", written)
	}

	// Every file the space holds reaches the bucket, as the space holds it.
	var problem string
	if !waitForCond(90, func() bool {
		problem = compareWork(t, adminUser, space)
		return problem == ""
	}) {
		dumpSyncState(t, adminUser, space)
		t.Fatalf("after the kills, %d files written: %s", written, problem)
	}
	t.Logf("%d files written through the kills, all in the bucket", written)

	// The space restarts while writing: what was written before the
	// restart is all there after, in the new container.
	startWriter(written + 1)
	time.Sleep(3 * time.Second)
	before := counter()
	t.Logf("restarting the space at file %d", before)
	rctx, rcancel := context.WithTimeout(ctx, 60*time.Second)
	if _, err := adminUser.Client.RestartSpace(rctx, space); err != nil {
		rcancel()
		t.Fatal(err)
	}
	rcancel()
	time.Sleep(5 * time.Second)
	harness.WaitForSpaceReady(t, s, adminUser.Client, space)
	ready()
	var got []int
	if !waitForCond(120, func() bool {
		got = workNumbers(adminUser, space)
		return len(got) > 0 && got[len(got)-1] >= before
	}) {
		last := 0
		if len(got) > 0 {
			last = got[len(got)-1]
		}
		t.Fatalf("the restarted space has files up to %d, but %d were written before the restart", last, before)
	}
	for i, n := range got {
		if n != i+1 {
			t.Fatalf("the restarted space is missing file %d of the %d", i+1, got[len(got)-1])
		}
	}
	t.Logf("the restarted space has all %d files", len(got))
}

// spaceContainer is the id of the space's container.
func spaceContainer(t *testing.T, runtime, spaceId string) string {
	t.Helper()
	out, err := exec.Command(runtime, "ps", "-q").Output()
	if err != nil {
		t.Fatalf("%s ps: %v", runtime, err)
	}
	for _, id := range strings.Fields(string(out)) {
		env, err := exec.Command(runtime, "inspect", "--format", "{{range .Config.Env}}{{println .}}{{end}}", id).Output()
		if err == nil && strings.Contains(string(env), "KNOT_SPACEID="+spaceId+"\n") {
			return id
		}
	}
	t.Fatalf("no container for space %s", spaceId)
	return ""
}

// compareWork says how the space's ~/work and the bucket's work/ differ,
// "" when they hold the same files with the same content.
func compareWork(t *testing.T, u *harness.User, space string) string {
	out, err := harness.TryRunCommand(u.Client, space, 60, "sh", "-c", "cd ~/work && sha256sum f* counter")
	if err != nil {
		return "reading the space: " + err.Error()
	}
	local := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 {
			local[f[1]] = f[0]
		}
	}
	ctx, cancel := testCtx(60)
	defer cancel()
	remote := map[string]string{}
	after := ""
	for {
		list, err := u.Client.ListFileObjects(ctx, "code", "work/", "", after, 1000)
		if err != nil {
			return "listing the bucket: " + err.Error()
		}
		for _, o := range list.Objects {
			name := strings.TrimPrefix(o.Key, "work/")
			if name != "" && !strings.Contains(name, "/") {
				remote[name] = o.SHA256
			}
		}
		if !list.IsTruncated {
			break
		}
		after = list.Next
	}
	var diffs []string
	for name, sha := range local {
		if r, ok := remote[name]; !ok {
			diffs = append(diffs, "missing "+name)
		} else if r != sha {
			diffs = append(diffs, "differs "+name)
		}
	}
	for name := range remote {
		if _, ok := local[name]; !ok {
			diffs = append(diffs, "extra "+name)
		}
	}
	if len(diffs) == 0 {
		return ""
	}
	sort.Strings(diffs)
	if len(diffs) > 6 {
		diffs = append(diffs[:6], strconv.Itoa(len(diffs)-6)+" more")
	}
	return strings.Join(diffs, "; ")
}

// workNumbers is the numbers of the files f00001... in the space's ~/work,
// in order.
func workNumbers(u *harness.User, space string) []int {
	out, _ := harness.TryRunCommand(u.Client, space, 30, "sh", "-c", "ls ~/work 2>/dev/null")
	var ns []int
	for _, f := range strings.Fields(out) {
		if strings.HasPrefix(f, "f") && !strings.HasSuffix(f, ".tmp") {
			if n, err := strconv.Atoi(strings.TrimPrefix(f, "f")); err == nil {
				ns = append(ns, n)
			}
		}
	}
	sort.Ints(ns)
	return ns
}

// dumpSyncState logs what a failed sync test needs to be understood: the
// space's processes, the newest files in ~/work, and the sync's own log
// lines from the space's log.
func dumpSyncState(t *testing.T, u *harness.User, space string) {
	t.Helper()
	out, _ := harness.TryRunCommand(u.Client, space, 30, "sh", "-c", "ps -eo pid,ppid,stat,lstart,args; ls -la --full-time ~/work | tail -6; cat ~/.knot/sync-projects/* 2>/dev/null")
	t.Logf("the space:\n%s", out)
	ctx, cancel := testCtx(20)
	defer cancel()
	ws, _, err := wsDial(ctx, u.Token, u.Client.GetWebSocketURL()+"/logs/"+space+"/stream")
	if err != nil {
		t.Logf("no log stream: %v", err)
		return
	}
	defer ws.Close()
	var lines []string
	func() {
		defer func() { recover() }()
		for {
			ws.SetReadDeadline(time.Now().Add(3 * time.Second))
			_, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if m := string(data); strings.Contains(m, "sync") || strings.Contains(m, "f00") {
				lines = append(lines, m)
			}
		}
	}()
	if len(lines) > 60 {
		lines = lines[len(lines)-60:]
	}
	t.Logf("the space's sync log, last %d lines:\n%s", len(lines), strings.Join(lines, "\n"))
}
