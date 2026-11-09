//go:build integration

package suites

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
)

// TestFileSyncProject keeps a space's source code in a bucket: the
// template's sync block starts each space's folder from a starter project
// the first time and syncs it both ways; the space is deleted and one of
// the same name made, and the code comes back as it was left. A project
// started in the space with knot file sync-project start runs alongside,
// listed and stopped, and the template's cannot be stopped.
func TestFileSyncProject(t *testing.T) {
	harness.Feature(t, "file-storage")
	s, adminUser := bootDedicated(t, "syncproj", "--files-path", t.TempDir())
	ctx, cancel := testCtx(900)
	defer cancel()
	if _, err := adminUser.Client.CreateFileBucket(ctx, "code"); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	put := func(key, content string) {
		t.Helper()
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
	put("templates/go/main.go", "package main\n")
	put("templates/go/README.md", "starter\n")

	tmpl, err := harness.CreateTemplate(s, adminUser.Client, uniqueName("it_sync_project"), harness.TemplateOptions{
		Volumes: `sync:
  - name: src
    local: ~/src
    remote: code:projects/${{ .space.name }}/
    snapshot: code:templates/go/
    exclude: [node_modules/]
`,
	})
	if err != nil {
		t.Fatal(err)
	}

	// In a space, the shell reads a file, or says it is not there.
	cat := func(space, path string) string {
		out, _ := harness.TryRunCommand(adminUser.Client, space, 30, "sh", "-c", "cat "+path+" 2>/dev/null || echo MISSING")
		return out
	}

	const name = "it-sync-project"
	first := harness.CreateSpace(t, adminUser.Client, name, tmpl, adminUser.Id)
	harness.WaitForSpaceReady(t, s, adminUser.Client, first)

	// The starter comes down; the work goes up.
	if !waitForCond(120, func() bool { return strings.Contains(cat(first, "~/src/main.go"), "package main") }) {
		t.Fatalf("the starter project did not come down: %q", cat(first, "~/src/main.go"))
	}
	if !strings.Contains(cat(first, "~/src/.knot-snapshot.json"), "MISSING") {
		t.Error("the snapshot's manifest came down into the space")
	}
	harness.RunCommand(t, adminUser.Client, first, 30, "sh", "-c",
		"printf 'package main // edited\\n' > ~/src/main.go && printf 'notes\\n' > ~/src/work.txt && mkdir -p ~/src/node_modules && echo x > ~/src/node_modules/dep.js")
	if !waitForCond(60, func() bool {
		got, _ := read("projects/" + name + "/work.txt")
		main, _ := read("projects/" + name + "/main.go")
		return got == "notes\n" && strings.Contains(main, "edited")
	}) {
		t.Fatal("the work in the space did not reach the bucket")
	}
	if _, ok := read("projects/" + name + "/node_modules/dep.js"); ok {
		t.Error("an excluded folder was synced")
	}
	// Changed in the bucket, the change comes down.
	put("projects/"+name+"/from-outside.txt", "outside\n")
	if !waitForCond(60, func() bool { return strings.Contains(cat(first, "~/src/from-outside.txt"), "outside") }) {
		t.Error("a change in the bucket did not come down")
	}

	// A project of the space's own, beside the template's.
	writeSpaceFileAs(t, adminUser, first, "/tmp/notes.toml", `name = "notes"
[[sync]]
local = "~/notes"
remote = "code:notes/"
`)
	harness.RunCommand(t, adminUser.Client, first, 30, "sh", "-c", "mkdir -p ~/notes && echo hello > ~/notes/a.txt")
	out := harness.RunCommand(t, adminUser.Client, first, 90, "sh", "-c", "/usr/local/bin/knot file sync-project start --wait /tmp/notes.toml 2>&1; echo EXIT=$?")
	if !strings.Contains(out, "EXIT=0") {
		t.Fatalf("sync-project start --wait: %s", out)
	}
	if got, _ := read("notes/a.txt"); got != "hello\n" {
		t.Errorf("the started project did not sync once --wait returned: %q", got)
	}
	out = harness.RunCommand(t, adminUser.Client, first, 30, "sh", "-c", "/usr/local/bin/knot file sync-project list 2>&1")
	if !strings.Contains(out, "notes") || !strings.Contains(out, "template (template)") {
		t.Errorf("sync-project list: %s", out)
	}
	out = harness.RunCommand(t, adminUser.Client, first, 30, "sh", "-c", "/usr/local/bin/knot file sync-project status template 2>&1")
	if !strings.Contains(out, "watching") || !strings.Contains(out, "code:projects/"+name+"/") {
		t.Errorf("sync-project status: %s", out)
	}
	out = harness.RunCommand(t, adminUser.Client, first, 30, "sh", "-c", "/usr/local/bin/knot file sync-project stop template 2>&1; echo EXIT=$?")
	if strings.Contains(out, "EXIT=0") {
		t.Errorf("the template's project was stopped: %s", out)
	}
	out = harness.RunCommand(t, adminUser.Client, first, 30, "sh", "-c", "/usr/local/bin/knot file sync-project stop notes 2>&1; echo EXIT=$?")
	if !strings.Contains(out, "EXIT=0") {
		t.Errorf("sync-project stop: %s", out)
	}
	harness.RunCommand(t, adminUser.Client, first, 30, "sh", "-c", "echo later > ~/notes/b.txt")

	// The space goes straight after an edit, before the sync would have
	// noticed it; one of the same name comes back with the work, the last
	// edit included, not the starter.
	harness.RunCommand(t, adminUser.Client, first, 30, "sh", "-c", "printf 'last\\n' > ~/src/last-minute.txt")
	harness.DeleteSpaceAndWait(t, adminUser.Client, first)
	if _, ok := read("notes/b.txt"); ok {
		t.Error("a stopped project kept syncing")
	}
	put("templates/go/main.go", "package main // new starter\n")

	second := harness.CreateSpace(t, adminUser.Client, name, tmpl, adminUser.Id)
	harness.DeleteSpaceAsync(t, adminUser.Client, second)
	harness.WaitForSpaceReady(t, s, adminUser.Client, second)
	if !waitForCond(120, func() bool { return strings.Contains(cat(second, "~/src/work.txt"), "notes") }) {
		t.Fatalf("the work did not come back to a new space: %q", cat(second, "~/src/work.txt"))
	}
	if got := cat(second, "~/src/main.go"); !strings.Contains(got, "edited") {
		t.Errorf("a new space got main.go %q, not the work", got)
	}
	if got := cat(second, "~/src/from-outside.txt"); !strings.Contains(got, "outside") {
		t.Errorf("a new space lacks the bucket's change: %q", got)
	}
	if got := cat(second, "~/src/last-minute.txt"); !strings.Contains(got, "last") {
		t.Errorf("an edit made just before the space was deleted was lost: %q", got)
	}
}

// TestFileSyncProjectShared starts two spaces at once on one new folder,
// made from a snapshot: one snapshot is made and both spaces sync it. A
// template whose spaces may not use file storage runs its sync block to a
// clear failure, and the space works on.
func TestFileSyncProjectShared(t *testing.T) {
	harness.Feature(t, "file-storage")
	s, adminUser := bootDedicated(t, "syncshared", "--files-path", t.TempDir())
	ctx, cancel := testCtx(900)
	defer cancel()
	if _, err := adminUser.Client.CreateFileBucket(ctx, "code"); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	for i := range 30 {
		key := fmt.Sprintf("starter/f%02d.txt", i)
		content := fmt.Sprintf("starter %d\n", i)
		if _, err := adminUser.Client.PutFileObject(ctx, "code", key, strings.NewReader(content), int64(len(content)), "", apiclient.FileAttrs{}); err != nil {
			t.Fatal(err)
		}
	}
	block := `sync:
  - name: shared
    local: ~/shared
    remote: code:together/
    snapshot: code:starter/
`
	tmpl, err := harness.CreateTemplate(s, adminUser.Client, uniqueName("it_sync_shared"), harness.TemplateOptions{Volumes: block})
	if err != nil {
		t.Fatal(err)
	}
	closed, err := harness.CreateTemplate(s, adminUser.Client, uniqueName("it_sync_closed"), harness.TemplateOptions{Volumes: block, DisableFileStorage: true})
	if err != nil {
		t.Fatal(err)
	}
	a := harness.CreateSpace(t, adminUser.Client, "it-sync-a", tmpl, adminUser.Id)
	harness.DeleteSpaceAsync(t, adminUser.Client, a)
	b := harness.CreateSpace(t, adminUser.Client, "it-sync-b", tmpl, adminUser.Id)
	harness.DeleteSpaceAsync(t, adminUser.Client, b)
	c := harness.CreateSpace(t, adminUser.Client, "it-sync-closed", closed, adminUser.Id)
	harness.DeleteSpaceAsync(t, adminUser.Client, c)
	harness.WaitForSpaceReady(t, s, adminUser.Client, a)
	harness.WaitForSpaceReady(t, s, adminUser.Client, b)
	harness.WaitForSpaceReady(t, s, adminUser.Client, c)

	count := func(space string) string {
		out, _ := harness.TryRunCommand(adminUser.Client, space, 30, "sh", "-c", "ls ~/shared 2>/dev/null | grep -c '^f'")
		return strings.TrimSpace(out)
	}
	for _, sp := range []string{a, b} {
		if !waitForCond(120, func() bool { return count(sp) == "30" }) {
			t.Fatalf("space %s has %s of the 30 starter files", sp, count(sp))
		}
	}
	list, err := adminUser.Client.ListFileObjects(ctx, "code", "together/", "", "", 1000)
	if err != nil {
		t.Fatal(err)
	}
	files := 0
	for _, o := range list.Objects {
		if !strings.HasSuffix(o.Key, "/") {
			files++
		}
	}
	if files != 31 {
		t.Fatalf("together/ holds %d files, want the 30 and one manifest", files)
	}
	// What one space writes, the other gets.
	harness.RunCommand(t, adminUser.Client, a, 30, "sh", "-c", "echo from-a > ~/shared/a.txt")
	if !waitForCond(60, func() bool {
		out, _ := harness.TryRunCommand(adminUser.Client, b, 30, "sh", "-c", "cat ~/shared/a.txt 2>/dev/null")
		return strings.Contains(out, "from-a")
	}) {
		t.Fatal("a file written in one space did not reach the other")
	}

	// File storage switched off: the job fails, saying why.
	var status string
	if !waitForCond(60, func() bool {
		status, _ = harness.TryRunCommand(adminUser.Client, c, 30, "sh", "-c", "/usr/local/bin/knot file sync-project status 2>&1")
		return strings.Contains(status, "failed") && strings.Contains(status, "switched off")
	}) {
		t.Fatalf("a template without file storage: want its sync job failed, saying why, got %q", status)
	}
	if out := harness.RunCommand(t, adminUser.Client, c, 30, "sh", "-c", "echo ok"); !strings.Contains(out, "ok") {
		t.Fatal("the space stopped working")
	}
}

func writeSpaceFileAs(t *testing.T, u *harness.User, spaceId, path, content string) {
	t.Helper()
	ctx, cancel := testCtx(30)
	defer cancel()
	if err := u.Client.WriteSpaceFile(ctx, spaceId, path, content); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
