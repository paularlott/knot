//go:build integration

package suites

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
	"github.com/paularlott/knot/internal/backupfile"
	"github.com/paularlott/knot/internal/database/model"
)

// TestAdminBackupRestore proves the backup and restore round trip through the
// API, between two running servers: knot admin backup takes everything from
// the first, including the template and space job definitions and a bucket
// of files, and knot admin restore, given no token, rebuilds it on a new
// server that has no users. It also checks who may do either.
func TestAdminBackupRestore(t *testing.T) {
	harness.Feature(t, "admin")

	filesA, filesB := t.TempDir(), t.TempDir()
	s, err := harness.StartServer(cfg, bins, "adminbak", "--files-path", filesA)
	if err != nil {
		t.Fatalf("boot adminbak server: %v", err)
	}
	admin, err := harness.ProvisionAdmin(s, "admin", "AdminPassw0rd!")
	if err != nil {
		t.Fatalf("provision adminbak admin: %v", err)
	}
	t.Cleanup(s.Stop)

	templateName := uniqueName("it-bak-tmpl")
	templateId, err := harness.CreateTemplate(s, admin.Client, templateName, harness.TemplateOptions{
		Jobs: []model.SpaceJob{
			{Name: "tmpljob", Command: "knot run-script backup", Schedule: "0 3 * * *", Enabled: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// A space inherits the template's jobs at creation; give it one more of
	// its own so both copy and per-space definitions are covered.
	spaceId := harness.CreateSpace(t, admin.Client, uniqueName("it-bak"), templateId, admin.Id)
	harness.WaitForSpaceReady(t, s, admin.Client, spaceId)
	ctx, cancel := testCtx(60)
	defer cancel()
	if _, code, err := admin.Client.UpdateSpaceJobs(ctx, spaceId, &apiclient.SpaceJobsRequest{
		Jobs: append(inheritedJobs(t, admin.Client, spaceId),
			model.SpaceJob{Name: "spacejob", Command: "true", Schedule: "15 4 * * *", Enabled: true},
		),
		Enabled: true,
	}); err != nil {
		t.Fatalf("update space jobs: %v (status %d)", err, code)
	}
	harness.StopSpaceAndWait(t, admin.Client, spaceId)

	// File storage: a bucket with a small and a large file.
	if _, err := admin.Client.CreateFileBucket(ctx, "it-bak-files"); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	small, large := []byte("small file"), bytes.Repeat([]byte("L"), 200*1024)
	for key, data := range map[string][]byte{"dir/small.txt": small, "large.bin": large} {
		if _, err := admin.Client.PutFileObject(ctx, "it-bak-files", key, bytes.NewReader(data), int64(len(data)), "application/octet-stream", time.Time{}); err != nil {
			t.Fatalf("put %s: %v", key, err)
		}
	}

	run := func(args ...string) (string, error) {
		t.Helper()
		cmd := exec.Command(bins.Server, args...)
		cmd.Dir = s.DataDir
		cmd.Env = append(os.Environ(), "HOME="+s.DataDir)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	// Who may back up: the first user holds both the Admin and Backup User
	// roles; an administrator without the Backup User role cannot.
	get := func(token, path string) int {
		req, _ := http.NewRequest("GET", s.BaseURL+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.StatusCode
	}
	plain, err := harness.CreateUser(s, admin, "it-bak-plain", []uint16{model.PermissionManageUsers, model.PermissionManageFiles})
	if err != nil {
		t.Fatal(err)
	}
	only, err := harness.CreateUser(s, admin, "it-bak-only", []uint16{model.PermissionBackup})
	if err != nil {
		t.Fatal(err)
	}
	if code := get(admin.Token, "/api/backup/info"); code != 200 {
		t.Errorf("the first user cannot back up: %d", code)
	}
	// A new server made a Backup User role in its database, and gave it to
	// the first user; it is an ordinary role, so it can be removed.
	roles, _, err := admin.Client.GetRoles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	backupRole := ""
	for _, r := range roles.Roles {
		if r.Name == model.BackupRoleName {
			backupRole = r.Id
		}
	}
	if backupRole == "" {
		t.Fatalf("a new server has no Backup User role: %+v", roles.Roles)
	}
	if detail, _, err := admin.Client.GetRole(ctx, backupRole); err != nil || len(detail.Permissions) != 1 || detail.Permissions[0] != model.PermissionBackup {
		t.Errorf("the Backup User role holds %+v: %v", detail, err)
	}
	if code := get(plain.Token, "/api/backup/info"); code != 403 {
		t.Errorf("a user without the backup permission got %d", code)
	}
	if code := get(only.Token, "/api/backup/info"); code != 200 {
		t.Errorf("a user with the backup permission got %d", code)
	}
	if code := get(only.Token, "/api/users"); code != 403 {
		t.Errorf("a backup user listed users: %d", code)
	}
	if code := get(plain.Token, "/api/backup/users"); code != 403 {
		t.Errorf("a user without the backup permission read users: %d", code)
	}
	if out, err := run("admin", "backup", filepath.Join(s.DataDir, "refused"), "--server", s.BaseURL, "--token", plain.Token); err == nil {
		t.Errorf("a backup by a user without the permission worked:\n%s", out)
	}

	// Back up with the backup-only user: it needs nothing else.
	backupDir := filepath.Join(s.DataDir, "backup")
	if out, err := run("admin", "backup", backupDir, "--server", s.BaseURL, "--token", only.Token, "--encrypt-key", "0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatalf("knot admin backup: %v\n%s", err, out)
	}
	m, err := backupfile.ReadManifest(backupDir)
	if err != nil || !m.Encrypted || !m.Content || m.Counts["users"] < 3 || m.Counts["file-objects"] != 2 || m.Counts["file-buckets"] != 1 {
		t.Fatalf("manifest %+v: %v", m, err)
	}
	raw, _ := os.ReadFile(backupfile.RecordPath(backupDir, "users"))
	if strings.Contains(string(raw), "it-bak-plain") {
		t.Error("the backup of users is not encrypted")
	}

	// Listing and restoring one file from the backup.
	key := "0123456789abcdef0123456789abcdef"
	if out, err := run("admin", "file", "ls", backupDir, "--encrypt-key", key); err != nil || !strings.Contains(out, "admin--it-bak-files") {
		t.Errorf("file ls of the backup: %v\n%s", err, out)
	}
	if out, err := run("admin", "file", "ls", backupDir, "admin--it-bak-files:dir/", "--encrypt-key", key); err != nil || !strings.Contains(out, "dir/small.txt") {
		t.Errorf("file ls of a folder: %v\n%s", err, out)
	}
	if err := admin.Client.DeleteFileObject(ctx, "it-bak-files", "dir/small.txt"); err != nil {
		t.Fatalf("delete file: %v", err)
	}
	if out, err := run("admin", "file", "restore", backupDir, "admin--it-bak-files:dir/small.txt", "--server", s.BaseURL, "--token", admin.Token, "--encrypt-key", key); err != nil {
		t.Fatalf("file restore: %v\n%s", err, out)
	}
	if got := fetchFile(t, admin.Client, "it-bak-files", "dir/small.txt"); !bytes.Equal(got, small) {
		t.Errorf("restored file reads %q", got)
	}
	// An existing file is left alone without --overwrite.
	if out, err := run("admin", "file", "restore", backupDir, "admin--it-bak-files:large.bin", "--server", s.BaseURL, "--token", admin.Token, "--encrypt-key", key); err != nil || !strings.Contains(out, "exists, skipped") {
		t.Errorf("file restore of an existing file: %v\n%s", err, out)
	}
	if out, err := run("admin", "file", "restore", backupDir, "admin--it-bak-files:large.bin", "--overwrite", "--server", s.BaseURL, "--token", admin.Token, "--encrypt-key", key); err != nil || !strings.Contains(out, "1 files restored") {
		t.Errorf("file restore with --overwrite: %v\n%s", err, out)
	}

	// A new server has no one to hold the Backup Server permission: a restore
	// is refused until its first user exists, and is then done with that
	// user's token.
	s2, err := harness.StartServer(cfg, bins, "adminbak2", "--files-path", filesB)
	if err != nil {
		t.Fatalf("boot the new server: %v", err)
	}
	t.Cleanup(s2.Stop)
	if out, err := run("admin", "restore", backupDir, "--server", s2.BaseURL, "--encrypt-key", key); err == nil {
		t.Fatalf("a restore into a server with no users, and no token, worked:\n%s", out)
	}
	if out, err := run("admin", "restore", backupDir, "--server", s2.BaseURL, "--token", "tk_notatoken", "--encrypt-key", key); err == nil {
		t.Fatalf("a restore into a server with no users, with a bad token, worked:\n%s", out)
	}
	boot, err := harness.ProvisionAdmin(s2, "bootstrap", "BootPassw0rd!")
	if err != nil {
		t.Fatalf("create the first user: %v", err)
	}
	if out, err := run("admin", "restore", backupDir, "--server", s2.BaseURL, "--token", boot.Token, "--encrypt-key", key); err != nil {
		t.Fatalf("knot admin restore: %v\n%s", err, out)
	}
	if out, err := run("admin", "restore", backupDir, "--server", s2.BaseURL, "--encrypt-key", key); err == nil {
		t.Errorf("a restore without a token worked:\n%s", out)
	}

	restored, err := harness.LoginUser(s2, "admin", "AdminPassw0rd!")
	if err != nil {
		t.Fatalf("log in to the restored server: %v", err)
	}
	tpl, err := restored.Client.GetTemplateByName(ctx, templateName)
	if err != nil || tpl == nil {
		t.Fatalf("restored template not found: %v", err)
	}
	if len(tpl.Jobs) != 1 || tpl.Jobs[0].Name != "tmpljob" || !tpl.Jobs[0].Enabled {
		t.Fatalf("restored template jobs mismatch: %+v", tpl.Jobs)
	}
	defs, code, err := restored.Client.GetSpaceJobs(ctx, spaceId)
	if err != nil {
		t.Fatalf("restored space jobs: %v (status %d)", err, code)
	}
	if len(defs.Jobs) != 2 || !defs.Enabled {
		t.Fatalf("restored space jobs mismatch: %+v (enabled=%v)", defs.Jobs, defs.Enabled)
	}
	if got := fetchFile(t, restored.Client, "it-bak-files", "dir/small.txt"); !bytes.Equal(got, small) {
		t.Errorf("restored small file reads %q", got)
	}
	if got := fetchFile(t, restored.Client, "it-bak-files", "large.bin"); !bytes.Equal(got, large) {
		t.Errorf("restored large file is %d bytes", len(got))
	}
	// The backup user's token came back with the users.
	if code := getOn(t, s2, only.Token, "/api/backup/info"); code != 200 {
		t.Errorf("the backup user's token after the restore: %d", code)
	}

	// The role came back too, and removing it takes the permission away.
	if code, err := restored.Client.DeleteRole(ctx, backupRole); err != nil {
		t.Fatalf("remove the Backup User role: %v (status %d)", err, code)
	}
	if code := getOn(t, s2, restored.Token, "/api/backup/info"); code != 403 {
		t.Errorf("an admin whose backup role was removed got %d", code)
	}
}

func getOn(t *testing.T, s *harness.Server, token, path string) int {
	t.Helper()
	req, _ := http.NewRequest("GET", s.BaseURL+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode
}

func fetchFile(t *testing.T, c *apiclient.ApiClient, bucket, key string) []byte {
	t.Helper()
	ctx, cancel := testCtx(60)
	defer cancel()
	resp, err := c.GetFileObject(ctx, bucket, key)
	if err != nil {
		t.Fatalf("get %s/%s: %v", bucket, key, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return data
}

// inheritedJobs returns the job definitions a space copied from its template.
func inheritedJobs(t *testing.T, c *apiclient.ApiClient, spaceId string) []model.SpaceJob {
	t.Helper()
	ctx, cancel := testCtx(30)
	defer cancel()
	defs, code, err := c.GetSpaceJobs(ctx, spaceId)
	if err != nil {
		t.Fatalf("get inherited jobs: %v (status %d)", err, code)
	}
	if len(defs.Jobs) != 1 || defs.Jobs[0].Name != "tmpljob" {
		t.Fatalf("template jobs not copied to space: %+v", defs.Jobs)
	}
	return defs.Jobs
}
