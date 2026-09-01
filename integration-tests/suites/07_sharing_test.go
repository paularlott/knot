//go:build integration

package suites

import (
	"testing"

	"github.com/paularlott/knot/integration-tests/harness"
)

func TestSpaceSharing(t *testing.T) {
	harness.Feature(t, "sharing")

	// The shared space belongs to the OTHER user (user2 on OSS; the admin
	// on pro) and gets shared with user1, so the access assertions are
	// meaningful from user1's unprivileged side on both editions.
	id := spaceFixture(t, "it-share", user2.Id, user2.Client)

	ctx, cancel := testCtx(60)
	defer cancel()

	// Not visible to user1 before sharing.
	if _, _, err := user1.Client.GetSpace(ctx, id); err == nil {
		t.Fatal("space visible to user1 before share")
	}

	// Share with user1.
	if code, err := user2.Client.AddShare(ctx, id, user1.Id); err != nil {
		t.Fatalf("add share: %v (status %d)", err, code)
	}

	space, _, err := user1.Client.GetSpace(ctx, id)
	if err != nil {
		t.Fatalf("shared space not readable by user1: %v", err)
	}
	if len(space.Shares) != 1 || space.Shares[0] != user1.Id {
		t.Fatalf("shares = %v", space.Shares)
	}

	// user1 can run commands in the shared space.
	out, err := harness.TryRunCommand(user1.Client, id, 60, "echo", "shared-access")
	if err != nil {
		t.Fatalf("user1 run command in shared space: %v", err)
	}
	if out != "shared-access\n" {
		t.Fatalf("shared command output = %q", out)
	}

	// Appears in user1's own space list.
	spaces, _, err := user1.Client.GetSpaces(ctx, user1.Id, false)
	if err != nil {
		t.Fatalf("user1 list spaces: %v", err)
	}
	found := false
	for _, s := range spaces.Spaces {
		if s.Id == id {
			found = true
		}
	}
	if !found {
		t.Fatal("shared space missing from user1's list")
	}

	// Unshare.
	if code, err := user2.Client.RemoveShare(ctx, id); err != nil {
		t.Fatalf("remove share: %v (status %d)", err, code)
	}
	if _, _, err := user1.Client.GetSpace(ctx, id); err == nil {
		t.Fatal("space still readable after unshare")
	}
}

func TestSpaceTransfer(t *testing.T) {
	harness.Feature(t, "transfer")
	id := spaceFixture(t, "it-xfer", user1.Id, user1.Client)

	ctx, cancel := testCtx(60)
	defer cancel()

	// Transfer requires the space to be stopped.
	harness.StopSpaceAndWait(t, user1.Client, id)

	if code, err := user1.Client.TransferSpace(ctx, id, user2.Id); err != nil {
		t.Fatalf("transfer space: %v (status %d)", err, code)
	}
	space, _, err := user2.Client.GetSpace(ctx, id)
	if err != nil {
		t.Fatalf("transferred space not readable by user2: %v", err)
	}
	mustEqual(t, "new owner", space.UserId, user2.Id)

	if _, _, err := user1.Client.GetSpace(ctx, id); err == nil {
		t.Fatal("old owner can still read the space")
	}
}
