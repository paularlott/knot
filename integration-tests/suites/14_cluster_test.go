//go:build integration

package suites

import (
	"fmt"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
)

// TestClusterGossip boots two servers joined by TCP gossip with a shared
// cluster key, then verifies configuration replicates: the second node
// appears in cluster info and a user created on node A shows up on node B.
func TestClusterGossip(t *testing.T) {
	harness.Feature(t, "cluster-gossip")

	key := "0123456789abcdef0123456789abcdef" // 32 bytes
	bindA, bindB := freePorts(t, 2)

	nodeA, err := harness.StartServer(cfg, bins, "clustera",
		"--cluster-key", key,
		"--cluster-bind-addr", fmt.Sprintf("127.0.0.1:%d", bindA),
		"--cluster-advertise-addr", fmt.Sprintf("127.0.0.1:%d", bindA),
		"--cluster-peer", fmt.Sprintf("127.0.0.1:%d", bindB),
	)
	if err != nil {
		t.Fatalf("boot cluster node A: %v", err)
	}
	t.Cleanup(nodeA.Stop)

	nodeB, err := harness.StartServer(cfg, bins, "clusterb",
		"--cluster-key", key,
		"--cluster-bind-addr", fmt.Sprintf("127.0.0.1:%d", bindB),
		"--cluster-advertise-addr", fmt.Sprintf("127.0.0.1:%d", bindB),
		"--cluster-peer", fmt.Sprintf("127.0.0.1:%d", bindA),
	)

	if err != nil {
		t.Fatalf("boot cluster node B: %v", err)
	}
	t.Cleanup(nodeB.Stop)

	adminA, err := harness.ProvisionAdmin(nodeA, "admin", "AdminPassw0rd!")
	if err != nil {
		t.Fatalf("provision admin on node A: %v", err)
	}

	// Both nodes should appear in cluster info once gossip settles.
	waitFor(t, 60, func() bool {
		ctx, cancel := testCtx(15)
		defer cancel()
		nodes, _, err := adminA.Client.GetClusterInfo(ctx)
		if err != nil {
			return false
		}
		return len(*nodes) >= 2
	})
	ctx, cancel := testCtx(15)
	defer cancel()
	nodes, code, err := adminA.Client.GetClusterInfo(ctx)
	if err != nil {
		t.Fatalf("cluster info on A: %v (status %d)", err, code)
	}
	if len(*nodes) < 2 {
		t.Fatalf("node A sees %d cluster nodes, want >= 2", len(*nodes))
	}

	// The admin user replicated to B, so B has no first-run window; log
	// in with the same credentials.
	adminB, err := harness.LoginUser(nodeB, "admin", "AdminPassw0rd!")
	if err != nil {
		t.Fatalf("login on node B: %v", err)
	}

	// A user created on A replicates to B.
	name := uniqueName("it-cluster-user")
	ctx2, cancel2 := testCtx(30)
	defer cancel2()
	if _, code, err := adminA.Client.CreateUser(ctx2, &apiclient.CreateUserRequest{
		Username: name, Password: "Passw0rd!cluster", Email: name + "@knot.test",
		Roles: []string{harness.RoleAdminUUID}, Active: true,
		MaxSpaces: 5, ComputeUnits: 5, StorageUnits: 5, MaxTunnels: 5,
		PreferredShell: "bash", Timezone: "UTC",
	}); err != nil {
		t.Fatalf("create user on A: %v (status %d)", err, code)
	}

	if !waitForCond(60, func() bool {
		ctx, cancel := testCtx(15)
		defer cancel()
		users, err := adminB.Client.GetUsers(ctx, "", "")
		if err != nil {
			return false
		}
		for _, u := range users.Users {
			if u.Username == name {
				return true
			}
		}
		return false
	}) {
		t.Fatal("user created on node A never appeared on node B")
	}

	// And a template created on B replicates to A.
	tmplName := uniqueName("it-cluster-tpl")
	tmplId, err := harness.CreateTemplate(nodeB, adminB.Client, tmplName, harness.TemplateOptions{})
	if err != nil {
		t.Fatalf("create template on B: %v", err)
	}
	if !waitForCond(60, func() bool {
		ctx, cancel := testCtx(15)
		defer cancel()
		_, err := adminA.Client.GetTemplateByName(ctx, tmplName)
		return err == nil
	}) {
		t.Fatal("template created on node B never appeared on node A")
	}
	_ = tmplId
}

func freePorts(t *testing.T, n int) (int, int) {
	t.Helper()
	a, err := harness.FreePort()
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	b, err := harness.FreePort()
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	return a, b
}
