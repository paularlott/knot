//go:build integration

package suites

import (
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration/harness"
)

// spaceFixture boots a ready space for the current test and registers cleanup.
func spaceFixture(t *testing.T, name, userId string, client *apiclient.ApiClient) string {
	t.Helper()
	id := harness.CreateSpace(t, client, name, templateId, userId)
	harness.DeleteSpaceAsync(t, admin.Client, id)
	harness.WaitForSpaceReady(t, server, client, id)
	return id
}

func TestSpaceLifecycle(t *testing.T) {
	harness.Feature(t, "space-lifecycle")
	id := spaceFixture(t, "it-life", user1.Id, user1.Client)
	ctx, cancel := testCtx(60)
	defer cancel()

	// Update description via full space request.
	space, _, err := user1.Client.GetSpace(ctx, id)
	if err != nil {
		t.Fatalf("get space: %v", err)
	}
	if code, err := user1.Client.UpdateSpace(ctx, id, &apiclient.SpaceRequest{
		Name:           space.Name,
		Description:    "updated desc",
		TemplateId:     space.TemplateId,
		Shell:          space.Shell,
		SelectedNodeId: space.NodeId,
	}); err != nil {
		t.Fatalf("update space: %v (status %d)", err, code)
	}
	space, _, _ = user1.Client.GetSpace(ctx, id)
	mustEqual(t, "description", space.Description, "updated desc")

	// Custom fields.
	if code, err := user1.Client.SetSpaceCustomField(ctx, id, "env", "staging"); err != nil {
		t.Fatalf("set custom field: %v (status %d)", err, code)
	}
	field, code, err := user1.Client.GetSpaceCustomField(ctx, id, "env")
	if err != nil {
		t.Fatalf("get custom field: %v (status %d)", err, code)
	}
	mustEqual(t, "custom field value", field.Value, "staging")

	// Stop → not deployed → start → deployed again.
	harness.StopSpaceAndWait(t, user1.Client, id)
	space, _, _ = user1.Client.GetSpace(ctx, id)
	if space.IsDeployed {
		t.Fatal("space still deployed after stop")
	}

	if code, err := user1.Client.StartSpace(ctx, id); err != nil {
		t.Fatalf("start space: %v (status %d)", err, code)
	}
	harness.WaitForSpaceReady(t, server, user1.Client, id)

	// Restart → ready again.
	if code, err := user1.Client.RestartSpace(ctx, id); err != nil {
		t.Fatalf("restart space: %v (status %d)", err, code)
	}
	harness.WaitForSpaceReady(t, server, user1.Client, id)
	out := harness.RunCommand(t, user1.Client, id, 60, "echo", "after-restart")
	mustEqual(t, "echo after restart", out, "after-restart\n")
}

func TestSpaceUserIsolation(t *testing.T) {
	harness.Feature(t, "permissions")
	workspace(t) // ensure user1 has a space of their own

	// The other user's space (user2 on OSS; the admin on pro, where the
	// built-in tier caps at two users). All assertions run from user1's
	// unprivileged side.
	theirId := harness.CreateSpace(t, user2.Client, "it-other", templateId, user2.Id)
	harness.DeleteSpaceAsync(t, admin.Client, theirId)
	harness.WaitForSpaceReady(t, server, user2.Client, theirId)

	ctx, cancel := testCtx(30)
	defer cancel()
	if _, _, err := user1.Client.GetSpace(ctx, theirId); err == nil {
		t.Fatal("user1 read another user's space")
	}
	if _, err := harness.TryRunCommand(user1.Client, theirId, 15, "echo", "nope"); err == nil {
		t.Fatal("user1 ran a command in another user's space")
	}

	spaces, _, err := user1.Client.GetSpaces(ctx, user1.Id, false)
	if err != nil {
		t.Fatalf("list user1 spaces: %v", err)
	}
	for _, s := range spaces.Spaces {
		if s.Id == theirId {
			t.Fatal("another user's space leaked into user1's list")
		}
	}
}

func TestSpaceStacks(t *testing.T) {
	harness.Feature(t, "space-stacks")
	ctx, cancel := testCtx(120)
	defer cancel()

	stackName := uniqueName("it-stack")
	mk := func(n string) string {
		id, code, err := user1.Client.CreateSpace(ctx, &apiclient.SpaceRequest{
			Name:   n,
			Stack:  stackName,
			TemplateId: templateId,
			UserId: user1.Id,
			Shell:  "bash",
		})
		if err != nil {
			t.Fatalf("create stack space %s: %v (status %d)", n, err, code)
		}
		t.Cleanup(func() { harness.DeleteSpaceAndWait(t, admin.Client, id) })
		return id
	}
	web := mk("it-stack-web")
	db := mk("it-stack-db")

	exists, err := user1.Client.StackExists(ctx, stackName)
	if err != nil {
		t.Fatalf("stack exists: %v", err)
	}
	if !exists {
		t.Fatal("stack not reported as existing")
	}

	for _, id := range []string{web, db} {
		if code, err := user1.Client.StartSpace(ctx, id); err != nil {
			t.Fatalf("start stack space: %v (status %d)", err, code)
		}
	}
	for _, id := range []string{web, db} {
		harness.WaitForSpaceReady(t, server, user1.Client, id)
	}

	if code, err := user1.Client.StopStack(ctx, stackName); err != nil {
		t.Fatalf("stop stack: %v (status %d)", err, code)
	}
	// One shared poll for both spaces instead of sequential 45s waits.
	deadline := time.Now().Add(150 * time.Second)
	for time.Now().Before(deadline) {
		allStopped := true
		for _, id := range []string{web, db} {
			pctx, pcancel := testCtx(15)
			space, _, err := user1.Client.GetSpace(pctx, id)
			pcancel()
			if err == nil && space != nil && (space.IsDeployed || space.IsPending) {
				allStopped = false
			}
		}
		if allStopped {
			break
		}
		time.Sleep(3 * time.Second)
	}

	// DeleteStack removes every space in the stack.
	user1.Client.SetTimeout(180e9) // stack ops block server-side
	if code, err := user1.Client.DeleteStack(ctx, stackName); err != nil {
		t.Fatalf("delete stack: %v (status %d)", err, code)
	}
	user1.Client.SetTimeout(60e9)
	for _, id := range []string{web, db} {
		deadline := time.Now().Add(60 * time.Second)
		for time.Now().Before(deadline) {
			if _, _, err := user1.Client.GetSpace(ctx, id); err != nil {
				break
			}
			time.Sleep(2 * time.Second)
		}
	}
}
