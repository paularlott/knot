//go:build integration

package suites

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
)

// TestFileSyncProjectScript runs sync projects from scripts in a space,
// with knot.files.sync: a script the agent runs starts one from a snapshot
// and waits for it, starting it again unchanged leaves it running, and a
// script run with knot run-script, which reaches the agent over its
// socket, reads its status and stops it, its last change sent first. The
// same scripts snapshot, move and stat files with knot.files.
func TestFileSyncProjectScript(t *testing.T) {
	harness.Feature(t, "file-storage")
	s, adminUser := bootDedicated(t, "syncscript", "--files-path", t.TempDir())
	ctx, cancel := testCtx(600)
	defer cancel()
	if _, err := adminUser.Client.CreateFileBucket(ctx, "code"); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	for key, content := range map[string]string{"templates/py/app.py": "print('hi')\n", "templates/py/lib/util.py": "X = 1\n"} {
		if _, err := adminUser.Client.PutFileObject(ctx, "code", key, strings.NewReader(content), int64(len(content)), "", apiclient.FileAttrs{}); err != nil {
			t.Fatalf("put %s: %v", key, err)
		}
	}
	read := func(key string) (string, bool) {
		resp, err := adminUser.Client.GetFileObject(ctx, "code", key)
		if err != nil {
			return "", false
		}
		defer resp.Body.Close()
		var b bytes.Buffer
		io.Copy(&b, resp.Body)
		return b.String(), true
	}

	tmpl, err := harness.CreateTemplate(s, adminUser.Client, uniqueName("it_sync_script"), harness.TemplateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	space := harness.CreateSpace(t, adminUser.Client, "it-sync-script", tmpl, adminUser.Id)
	harness.DeleteSpaceAsync(t, adminUser.Client, space)
	harness.WaitForSpaceReady(t, s, adminUser.Client, space)

	// A script the agent runs, as a startup script is.
	out, exit, err := adminUser.Client.ExecuteScriptContent(ctx, space, `
import knot.files as files
import knot.files.sync as sync

snap = files.snapshot("code", "templates/py", "code", "snaps/py-1")
print("snapshot", snap["files"])
print("moved", files.move("code", "snaps/py-1/lib", "snaps/py-1/pkg"))
print("stat", files.stat("code", "snaps/py-1/pkg")["type"], files.stat("code", "snaps/py-1/nothing"))

project = {"name": "app", "sync": [{"local": "~/app", "remote": "code:apps/demo/", "snapshot": "code:templates/py/"}]}
st = sync.start(project, wait=True)
print("state", st["jobs"][0]["state"], st["jobs"][0]["passes"] >= 1)
again = sync.start(project)
print("again", again["jobs"][0]["state"])
print("names", ",".join(sorted([p["name"] for p in sync.list()])))
`, nil)
	if err != nil || exit != 0 {
		t.Fatalf("the agent's script: exit %d, %v\n%s", exit, err, out)
	}
	for _, want := range []string{"snapshot 2", "moved 1", "stat folder None", "state watching True", "again watching", "names app"} {
		if !strings.Contains(out, want) {
			t.Errorf("the agent's script did not print %q:\n%s", want, out)
		}
	}
	if got := harness.RunCommand(t, adminUser.Client, space, 30, "sh", "-c", "cat ~/app/app.py ~/app/lib/util.py"); !strings.Contains(got, "print('hi')") || !strings.Contains(got, "X = 1") {
		t.Errorf("the project's snapshot did not come down: %q", got)
	}

	// A script run with knot run-script, in its own process, reaches the
	// agent over its socket.
	writeSpaceFileAs(t, adminUser, space, "/tmp/sync-stop.py", `
import knot.files.sync as sync

st = sync.status("app")
print("status", st["name"], st["jobs"][0]["state"], st["template"])
print("stopped", sync.stop("app"))
print("after", sync.status("app"))
try:
    sync.stop("template")
except Exception as e:
    print("refused", "runs for as long as the space does" in str(e))
`)
	// The last change is made just before the script stops the project.
	got, err := harness.TryRunCommand(adminUser.Client, space, 120, "sh", "-c", "printf 'last word' > ~/app/last.txt && knot run-script /tmp/sync-stop.py")
	if err != nil {
		t.Fatalf("knot run-script: %v\n%s", err, got)
	}
	for _, want := range []string{"status app watching False", "stopped True", "after None", "refused True"} {
		if !strings.Contains(got, want) {
			t.Errorf("knot run-script did not print %q:\n%s", want, got)
		}
	}
	if c, ok := read("apps/demo/last.txt"); !ok || c != "last word" {
		t.Errorf("a change made just before stop was not sent: %q %v", c, ok)
	}
}
