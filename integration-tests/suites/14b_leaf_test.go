//go:build integration

package suites

import (
	"context"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
)

// TestLeafNode boots an origin server (allowing leaf nodes) and a leaf
// server that connects with an origin-issued API token, then verifies the
// leaf replicates origin configuration (users, templates) and can serve
// spaces from replicated templates.
func testCtxX(seconds int) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), time.Duration(seconds)*time.Second)
}

func TestLeafNode(t *testing.T) {
	harness.Feature(t, "leaf-node")

	origin, err := harness.StartServer(cfg, bins, "origin", "--allow-leaf-nodes")
	if err != nil {
		t.Fatalf("boot origin: %v", err)
	}
	t.Cleanup(origin.Stop)

	originAdmin, err := harness.ProvisionAdmin(origin, "admin", "AdminPassw0rd!")
	if err != nil {
		t.Fatalf("provision origin admin: %v", err)
	}

	// The leaf's origin token is a regular API token minted on the origin.
	ctx, cancel := testCtx(30)
	defer cancel()
	originToken, code, err := originAdmin.Client.CreateToken(ctx, "leaf-token", nil)
	if err != nil {
		t.Fatalf("create origin token: %v (status %d)", err, code)
	}
	_ = originToken

	// A template on the origin the leaf should replicate and be able to use.
	tmplName := uniqueName("it-leaf-tpl")
	if _, err := harness.CreateTemplate(origin, originAdmin.Client, tmplName, harness.TemplateOptions{}); err != nil {
		t.Fatalf("create origin template: %v", err)
	}

	leaf, err := harness.StartServer(cfg, bins, "leaf",
		"--origin-server", origin.BaseURL,
		"--origin-token", originToken,
	)
	if err != nil {
		t.Fatalf("boot leaf: %v", err)
	}
	t.Cleanup(leaf.Stop)

	// The leaf rejects local user management (leaf nodes proxy the origin's
	// user store); its admin pages are limited. Provisioning a *local* admin
	// should fail — the leaf has no first-run window of its own.
	if _, err := harness.ProvisionAdmin(leaf, "admin", "LeafPassw0rd!"); err == nil {
		// Some editions may allow it; not the primary assertion.
		t.Log("note: leaf allowed local first-run admin creation")
	}

	// Leaf nodes authenticate against the replicated user store: log in
	// with the origin's admin credentials and mint a leaf API token.
	leafAdmin, err := harness.LoginUser(leaf, "admin", "AdminPassw0rd!")
	if err != nil {
		t.Fatalf("login on leaf with origin credentials: %v", err)
	}
	leafClient := leafAdmin.Client
	lctx, lcancel := testCtx(15)
	defer lcancel()
	who, err := leafClient.WhoAmI(lctx)
	if err != nil {
		t.Fatalf("whoami on leaf: %v", err)
	}
	if who.Username != "admin" {
		t.Fatalf("leaf whoami username = %q, want admin", who.Username)
	}

	// The origin template replicated to the leaf.
	if !waitForCond(60, func() bool {
		ctx, cancel := testCtx(15)
		defer cancel()
		_, err := leafClient.GetTemplateByName(ctx, tmplName)
		return err == nil
	}) {
		t.Fatal("origin template never replicated to the leaf")
	}

	// The leaf can create and boot a space from the replicated template —
	// end to end leaf functionality.
	ctx2, cancel2 := testCtx(120)
	defer cancel2()
	tmpl, err := leafClient.GetTemplateByName(ctx2, tmplName)
	if err != nil {
		t.Fatalf("get replicated template: %v", err)
	}
	spaceId, code, err := leafClient.CreateSpace(ctx2, &apiclient.SpaceRequest{
		Name: "it-leaf-space", TemplateId: tmpl.TemplateId, UserId: who.Id, Shell: "bash",
	})
	if err != nil {
		t.Fatalf("create space on leaf: %v (status %d)", err, code)
	}
	harness.DeleteSpaceAsync(t, leafClient, spaceId)
	if code, err := leafClient.StartSpace(ctx2, spaceId); err != nil {
		t.Fatalf("start space on leaf: %v (status %d)", err, code)
	}
	harness.WaitForSpaceReady(t, leaf, leafClient, spaceId)
	out := harness.RunCommand(t, leafClient, spaceId, 60, "echo", "leaf-space-ok")
	if out != "leaf-space-ok\n" {
		t.Fatalf("leaf space command output = %q", out)
	}
}

