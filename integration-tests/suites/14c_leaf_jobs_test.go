//go:build integration

package suites

import (
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
	"github.com/paularlott/knot/internal/database/model"
)

// TestLeafNodeJobsPermission proves leaf nodes imply the Edit Space Jobs
// permission for their local spaces: a user whose roles lack it is rejected
// on a regular server (see the jobs suite) but can edit the jobs of their
// own spaces on a leaf connected with their token. A leaf only replicates
// the user whose token it connects with, so the leaf here uses a restricted
// user's token rather than an admin's.
func TestLeafNodeJobsPermission(t *testing.T) {
	harness.Feature(t, "leaf-node")

	origin, err := harness.StartServer(cfg, bins, "leafjoborigin", "--allow-leaf-nodes")
	if err != nil {
		t.Fatalf("boot origin: %v", err)
	}
	t.Cleanup(origin.Stop)

	originAdmin, err := harness.ProvisionAdmin(origin, "admin", "AdminPassw0rd!")
	if err != nil {
		t.Fatalf("provision origin admin: %v", err)
	}

	// A standard tester minus Edit Space Jobs; the same permission set gets
	// 403 on a regular server.
	testerPerms := harness.TesterPermissions()
	restrictedPerms := make([]uint16, 0, len(testerPerms))
	for _, p := range testerPerms {
		if p != model.PermissionEditSpaceJobs {
			restrictedPerms = append(restrictedPerms, p)
		}
	}
	owner, err := harness.CreateUser(origin, originAdmin, uniqueName("it-leafjob"), restrictedPerms)
	if err != nil {
		t.Fatalf("create restricted user on origin: %v", err)
	}
	if owner.Token == "" {
		t.Fatal("restricted user's API token is empty — cannot connect the leaf")
	}

	tmplName := uniqueName("it-leafjob-tpl")
	if _, err := harness.CreateTemplate(origin, originAdmin.Client, tmplName, harness.TemplateOptions{}); err != nil {
		t.Fatalf("create origin template: %v", err)
	}

	leaf, err := harness.StartServer(cfg, bins, "leafjob",
		"--origin-server", origin.BaseURL,
		"--origin-token", owner.Token,
	)
	if err != nil {
		t.Fatalf("boot leaf with restricted token: %v", err)
	}
	t.Cleanup(leaf.Stop)

	leafOwner, err := harness.LoginUser(leaf, owner.Username, "Passw0rd!test")
	if err != nil {
		t.Fatalf("login restricted user on leaf: %v", err)
	}

	ctx, cancel := testCtx(120)
	defer cancel()
	if !waitForCond(60, func() bool {
		cctx, ccancel := testCtx(15)
		defer ccancel()
		_, err := leafOwner.Client.GetTemplateByName(cctx, tmplName)
		return err == nil
	}) {
		t.Fatal("origin template never replicated to the leaf")
	}
	tmpl, err := leafOwner.Client.GetTemplateByName(ctx, tmplName)
	if err != nil {
		t.Fatalf("get replicated template: %v", err)
	}

	spaceId, code, err := leafOwner.Client.CreateSpace(ctx, &apiclient.SpaceRequest{
		Name: "it-leafjob", TemplateId: tmpl.TemplateId, UserId: leafOwner.Id, Shell: "bash",
	})
	if err != nil {
		t.Fatalf("create space on leaf: %v (status %d)", err, code)
	}
	harness.DeleteSpaceAsync(t, leafOwner.Client, spaceId)
	if code, err := leafOwner.Client.StartSpace(ctx, spaceId); err != nil {
		t.Fatalf("start space on leaf: %v (status %d)", err, code)
	}
	harness.WaitForSpaceReady(t, leaf, leafOwner.Client, spaceId)

	// The leaf implies Edit Space Jobs: the definition update succeeds
	// despite the user's roles lacking the permission.
	if _, code, err := leafOwner.Client.UpdateSpaceJobs(ctx, spaceId, &apiclient.SpaceJobsRequest{
		Jobs:    []model.SpaceJob{{Name: "leafjob", Command: "true", Schedule: "0 4 * * *", Enabled: true}},
		Enabled: true,
	}); err != nil {
		t.Fatalf("edit jobs on leaf without the permission: %v (status %d)", err, code)
	}
	defs, code, err := leafOwner.Client.GetSpaceJobs(ctx, spaceId)
	if err != nil {
		t.Fatalf("get jobs on leaf: %v (status %d)", err, code)
	}
	if len(defs.Jobs) != 1 || defs.Jobs[0].Name != "leafjob" || defs.Jobs[0].Schedule != "0 4 * * *" {
		t.Fatalf("leaf job definitions: %+v", defs.Jobs)
	}
}
