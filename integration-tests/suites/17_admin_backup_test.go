//go:build integration

package suites

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
	"github.com/paularlott/knot/internal/config"
	driver_badgerdb "github.com/paularlott/knot/internal/database/drivers/badgerdb"
	"github.com/paularlott/knot/internal/database/model"
)

// TestAdminBackupRestore proves `knot admin backup` / `knot admin restore`
// round-trip template and space job definitions: the backup file carries
// them and a restore into a fresh database makes them readable again. The
// dedicated server is killed (not stopped) after the data is written so its
// badger dir survives for the CLI commands to work on.
func TestAdminBackupRestore(t *testing.T) {
	harness.Feature(t, "admin")

	s, err := harness.StartServer(cfg, bins, "adminbak")
	if err != nil {
		t.Fatalf("boot adminbak server: %v", err)
	}
	admin, err := harness.ProvisionAdmin(s, "admin", "AdminPassw0rd!")
	if err != nil {
		t.Fatalf("provision adminbak admin: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(s.DataDir) })

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

	// The badger lock is released when the process dies, so kill (not stop)
	// — Stop would delete the data dir.
	s.Kill()

	badgerPath := filepath.Join(s.DataDir, "badger")
	backupFile := filepath.Join(s.DataDir, "backup.json")

	runCLI := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(bins.Server, args...)
		cmd.Dir = s.DataDir
		cmd.Env = append(os.Environ(), "HOME="+s.DataDir)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("knot %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}

	runCLI("admin", "backup",
		"--badgerdb-enabled",
		"--badgerdb-path", badgerPath,
		backupFile,
	)

	// The backup file carries the job definitions on both templates and
	// spaces.
	type backupUser struct {
		Spaces []*model.Space `json:"spaces"`
	}
	var backup struct {
		Templates []*model.Template `json:"templates"`
		Users     []backupUser      `json:"users"`
	}
	data, err := os.ReadFile(backupFile)
	if err != nil {
		t.Fatalf("read backup file: %v", err)
	}
	if err := json.Unmarshal(data, &backup); err != nil {
		t.Fatalf("parse backup file: %v", err)
	}
	var tplJobs []model.SpaceJob
	for _, tpl := range backup.Templates {
		if tpl.Name == templateName {
			tplJobs = tpl.Jobs
		}
	}
	if len(tplJobs) != 1 || tplJobs[0].Name != "tmpljob" || tplJobs[0].Schedule != "0 3 * * *" {
		t.Fatalf("template jobs missing from backup: %+v", tplJobs)
	}
	var spaceJobs []model.SpaceJob
	spaceFound := false
	for _, u := range backup.Users {
		for _, sp := range u.Spaces {
			if sp.Id == spaceId {
				spaceFound = true
				spaceJobs = sp.Jobs
				if !sp.JobsEnabled {
					t.Fatalf("space jobs runner state lost in backup: %+v", sp)
				}
			}
		}
	}
	if !spaceFound {
		t.Fatal("space missing from backup")
	}
	if len(spaceJobs) != 2 {
		t.Fatalf("space jobs missing from backup: %+v", spaceJobs)
	}

	// Restore into a fresh database at the same path.
	if err := os.RemoveAll(badgerPath); err != nil {
		t.Fatalf("wipe badger dir: %v", err)
	}
	runCLI("admin", "restore",
		"--badgerdb-enabled",
		"--badgerdb-path", badgerPath,
		backupFile,
	)

	// Read the restored database back through the badger driver.
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: badgerPath},
	})
	db := &driver_badgerdb.BadgerDbDriver{}
	if err := db.Connect(); err != nil {
		t.Fatalf("open restored badger: %v", err)
	}

	tpl, err := db.GetTemplateByName(templateName)
	if err != nil || tpl == nil {
		t.Fatalf("restored template not found: %v", err)
	}
	if len(tpl.Jobs) != 1 || tpl.Jobs[0].Name != "tmpljob" || !tpl.Jobs[0].Enabled {
		t.Fatalf("restored template jobs mismatch: %+v", tpl.Jobs)
	}

	space, err := db.GetSpace(spaceId)
	if err != nil || space == nil {
		t.Fatalf("restored space not found: %v", err)
	}
	if len(space.Jobs) != 2 || !space.JobsEnabled {
		t.Fatalf("restored space jobs mismatch: %+v (enabled=%v)", space.Jobs, space.JobsEnabled)
	}
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
