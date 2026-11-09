//go:build integration

package suites

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/integration-tests/harness"
	"github.com/paularlott/knot/internal/database/model"
)

// TestRestoreFromPreviousRelease restores, with this build, a backup made by
// an earlier release's knot admin backup from an earlier release's server:
// the backup an upgrade starts from. KNOT_TEST_PREV_BIN names that release's
// knot binary; without it the test is skipped. Users, groups, roles,
// template variables, a template with its jobs and a bucket of files come
// back, and a file mode stored in the decimal form knot before 0.42 wrote is
// rewritten by knot admin file fsck --repair, as the upgrade notes say.
func TestRestoreFromPreviousRelease(t *testing.T) {
	harness.Feature(t, "admin")
	prev := os.Getenv("KNOT_TEST_PREV_BIN")
	if prev == "" {
		t.Skip("set KNOT_TEST_PREV_BIN to an earlier release's knot binary")
	}
	if _, err := os.Stat(prev); err != nil {
		t.Fatalf("KNOT_TEST_PREV_BIN: %v", err)
	}
	ctx, cancel := testCtx(120)
	defer cancel()
	const key = "0123456789abcdef0123456789abcdef"

	// The earlier release's server, with something of each kind.
	old, err := harness.StartServer(cfg, &harness.Binaries{Server: prev}, "prevrel", "--files-path", t.TempDir())
	if err != nil {
		t.Fatalf("boot the earlier release: %v", err)
	}
	t.Cleanup(old.Stop)
	// This build's client refuses a server of another version: it fills
	// the earlier release through a proxy that answers as this version.
	// The backup itself is made by the earlier release's own command.
	target, err := url.Parse(old.BaseURL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	director := proxy.Director
	proxy.Director = func(r *http.Request) {
		director(r)
		r.Header.Set("User-Agent", "knot-integration-test") // no version for the server to check
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Set(build.VersionHeader, build.Version)
		return nil
	}
	front := httptest.NewServer(proxy)
	t.Cleanup(front.Close)
	oldFront := &harness.Server{Name: old.Name, Config: old.Config, BaseURL: front.URL, DataDir: old.DataDir, LogPath: old.LogPath}
	admin, err := harness.ProvisionAdmin(oldFront, "admin", "AdminPassw0rd!")
	if err != nil {
		t.Fatalf("provision the earlier release's admin: %v", err)
	}
	if _, err := harness.CreateUser(oldFront, admin, "prev-user", []uint16{model.PermissionManageFiles}); err != nil {
		t.Fatal(err)
	}
	groupName := uniqueName("prev-group")
	if _, code, err := admin.Client.CreateGroup(ctx, &apiclient.GroupRequest{Name: groupName, MaxSpaces: 3}); err != nil {
		t.Fatalf("create group: %v (status %d)", err, code)
	}
	roleName := uniqueName("prev-role")
	if _, code, err := admin.Client.CreateRole(ctx, &apiclient.RoleRequest{Name: roleName, Permissions: []uint16{model.PermissionManageFiles}}); err != nil {
		t.Fatalf("create role: %v (status %d)", err, code)
	}
	varName := strings.ReplaceAll(uniqueName("prev_var"), "-", "_")
	if _, code, err := admin.Client.CreateTemplateVar(ctx, &apiclient.TemplateVarValue{Name: varName, Value: "kept"}); err != nil {
		t.Fatalf("create template variable: %v (status %d)", err, code)
	}
	templateName := uniqueName("prev-tmpl")
	if _, err := harness.CreateTemplate(oldFront, admin.Client, templateName, harness.TemplateOptions{
		Jobs: []model.SpaceJob{{Name: "nightly", Command: "true", Schedule: "0 3 * * *", Enabled: true}},
	}); err != nil {
		t.Fatalf("create template: %v", err)
	}
	if _, err := admin.Client.CreateFileBucket(ctx, "prev-files"); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	small, large := []byte("small file"), bytes.Repeat([]byte("L"), 200*1024)
	if _, err := admin.Client.PutFileObject(ctx, "prev-files", "dir/small.txt", bytes.NewReader(small), int64(len(small)), "", apiclient.FileAttrs{}); err != nil {
		t.Fatal(err)
	}
	// A script with its mode as the earlier release wrote it over NFS:
	// decimal 33261 is 0100755.
	if _, err := admin.Client.PutFileObjectMeta(ctx, "prev-files", "bin/run.sh", bytes.NewReader(large), int64(len(large)), "", map[string]string{apiclient.FileModeMeta: "33261"}); err != nil {
		t.Fatal(err)
	}

	// Backed up by the earlier release's command.
	backupDir := filepath.Join(t.TempDir(), "backup")
	if out, err := runBin(prev, old.DataDir, "admin", "backup", backupDir, "--server", old.BaseURL, "--token", admin.Token, "--encrypt-key", key); err != nil {
		t.Fatalf("the earlier release's knot admin backup: %v\n%s", err, out)
	}

	// This build reads it...
	if out, err := runBin(bins.Server, old.DataDir, "admin", "file", "ls", backupDir, "admin--prev-files:dir/", "--encrypt-key", key); err != nil || !strings.Contains(out, "dir/small.txt") {
		t.Errorf("file ls of the earlier release's backup: %v\n%s", err, out)
	}

	// ...and restores it into a new server.
	s, err := harness.StartServer(cfg, bins, "prevrel-new", "--files-path", t.TempDir())
	if err != nil {
		t.Fatalf("boot this build: %v", err)
	}
	t.Cleanup(s.Stop)
	boot, err := harness.ProvisionAdmin(s, "bootstrap", "BootPassw0rd!")
	if err != nil {
		t.Fatal(err)
	}
	if out, err := runBin(bins.Server, s.DataDir, "admin", "restore", backupDir, "--server", s.BaseURL, "--token", boot.Token, "--encrypt-key", key); err != nil {
		t.Fatalf("knot admin restore of the earlier release's backup: %v\n%s", err, out)
	}

	restored, err := harness.LoginUser(s, "admin", "AdminPassw0rd!")
	if err != nil {
		t.Fatalf("log in as the earlier release's admin: %v", err)
	}
	users, err := restored.Client.GetUsers(ctx, "", "")
	found := false
	if users != nil {
		for _, u := range users.Users {
			found = found || u.Username == "prev-user"
		}
	}
	if err != nil || !found {
		t.Errorf("the earlier release's user was not restored: %v", err)
	}
	groups, _, err := restored.Client.GetGroups(ctx)
	if err != nil || !hasGroup(groups, groupName, 3) {
		t.Errorf("group %s not restored: %+v %v", groupName, groups, err)
	}
	roles, _, err := restored.Client.GetRoles(ctx)
	if err != nil || !hasRole(roles, roleName) {
		t.Errorf("role %s not restored: %v", roleName, err)
	}
	vars, _, err := restored.Client.GetTemplateVars(ctx)
	if err != nil || !hasVar(vars, varName) {
		t.Errorf("template variable %s not restored: %v", varName, err)
	}
	tpl, err := restored.Client.GetTemplateByName(ctx, templateName)
	if err != nil || tpl == nil {
		t.Fatalf("template not restored: %v", err)
	}
	if len(tpl.Jobs) != 1 || tpl.Jobs[0].Name != "nightly" || !tpl.Jobs[0].Enabled {
		t.Errorf("restored template jobs: %+v", tpl.Jobs)
	}
	if got := fetchFile(t, restored.Client, "prev-files", "dir/small.txt"); !bytes.Equal(got, small) {
		t.Errorf("restored small file reads %q", got)
	}
	if got := fetchFile(t, restored.Client, "prev-files", "bin/run.sh"); !bytes.Equal(got, large) {
		t.Errorf("restored script is %d bytes", len(got))
	}

	// The decimal mode is not read until fsck --repair rewrites it.
	modeOf := func() string {
		info, err := restored.Client.HeadFileObject(ctx, "prev-files", "bin/run.sh")
		if err != nil {
			t.Fatal(err)
		}
		return info.Meta[apiclient.FileModeMeta]
	}
	if m := modeOf(); m != "33261" {
		t.Errorf("the restored script's mode is %q, want the earlier release's %q", m, "33261")
	}
	if out, err := runBin(bins.Server, s.DataDir, "admin", "file", "fsck", "--repair", "--server", s.BaseURL, "--token", restored.Token); err != nil {
		t.Fatalf("fsck --repair: %v\n%s", err, out)
	}
	if m := modeOf(); m != "100755" {
		t.Errorf("after fsck --repair the script's mode is %q, want %q", m, "100755")
	}
	if mode, ok := apiclient.ParseFileMode(map[string]string{apiclient.FileModeMeta: modeOf()}); !ok || mode.Perm() != 0755 {
		t.Errorf("the script's mode reads %v %v, want 0755", mode, ok)
	}
}

// runBin runs a knot binary in dir, with dir as its home.
func runBin(bin, dir string, args ...string) (string, error) {
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "HOME="+dir)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func hasGroup(l *apiclient.GroupInfoList, name string, maxSpaces uint32) bool {
	if l == nil {
		return false
	}
	for _, g := range l.Groups {
		if g.Name == name && g.MaxSpaces == maxSpaces {
			return true
		}
	}
	return false
}

func hasRole(l *apiclient.RoleInfoList, name string) bool {
	if l == nil {
		return false
	}
	for _, r := range l.Roles {
		if r.Name == name {
			return true
		}
	}
	return false
}

func hasVar(l *apiclient.TemplateVarList, name string) bool {
	if l == nil {
		return false
	}
	for _, v := range l.TemplateVar {
		if v.Name == name {
			return true
		}
	}
	return false
}
