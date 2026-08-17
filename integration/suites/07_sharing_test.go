//go:build integration

package suites

import (
	"testing"

	"github.com/paularlott/knot/integration/harness"
)

func TestSpaceSharing(t *testing.T) {
	harness.Feature(t, "sharing")
	id := spaceFixture(t, "it-share", user1.Id, user1.Client)

	ctx, cancel := testCtx(60)
	defer cancel()

	// Not visible to user2 before sharing.
	if _, _, err := user2.Client.GetSpace(ctx, id); err == nil {
		t.Fatal("space visible to user2 before share")
	}

	// Share with user2.
	if code, err := user1.Client.AddShare(ctx, id, user2.Id); err != nil {
		t.Fatalf("add share: %v (status %d)", err, code)
	}

	space, _, err := user2.Client.GetSpace(ctx, id)
	if err != nil {
		t.Fatalf("shared space not readable by user2: %v", err)
	}
	if len(space.Shares) != 1 || space.Shares[0] != user2.Id {
		t.Fatalf("shares = %v", space.Shares)
	}

	// user2 can run commands in the shared space.
	out, err := harness.TryRunCommand(user2.Client, id, 60, "echo", "shared-access")
	if err != nil {
		t.Fatalf("user2 run command in shared space: %v", err)
	}
	if out != "shared-access\n" {
		t.Fatalf("shared command output = %q", out)
	}

	// Appears in user2's own space list.
	spaces, _, err := user2.Client.GetSpaces(ctx, user2.Id, false)
	if err != nil {
		t.Fatalf("user2 list spaces: %v", err)
	}
	found := false
	for _, s := range spaces.Spaces {
		if s.Id == id {
			found = true
		}
	}
	if !found {
		t.Fatal("shared space missing from user2's list")
	}

	// Unshare.
	if code, err := user1.Client.RemoveShare(ctx, id); err != nil {
		t.Fatalf("remove share: %v (status %d)", err, code)
	}
	if _, _, err := user2.Client.GetSpace(ctx, id); err == nil {
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
