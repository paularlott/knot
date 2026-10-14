//go:build integration

package suites

import (
	"fmt"
	"strings"
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

	// A web session made on A is accepted by B, as when a load balancer
	// moves a user between servers.
	sessionA, err := apiclient.NewClient(nodeA.BaseURL, "", true)
	if err != nil {
		t.Fatalf("client for A: %v", err)
	}
	sessionA.SetContentType("application/json")
	sessionA.UseSessionCookie(true)
	ctxLogin, cancelLogin := testCtx(15)
	defer cancelLogin()
	auth, code, err := sessionA.Login(ctxLogin, "admin@knot.test", "AdminPassw0rd!", "")
	if err != nil {
		t.Fatalf("session login on A: %v (status %d)", err, code)
	}
	sessionB, err := apiclient.NewClient(nodeB.BaseURL, auth.Token, true)
	if err != nil {
		t.Fatalf("client for B: %v", err)
	}
	sessionB.SetContentType("application/json")
	sessionB.UseSessionCookie(true)
	if !waitForCond(60, func() bool {
		ctx, cancel := testCtx(15)
		defer cancel()
		who, err := sessionB.WhoAmI(ctx)
		return err == nil && who.Username == "admin"
	}) {
		t.Fatal("session made on node A was never accepted by node B")
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

	// A space running on A, locked with --no-cli through B: the change
	// reaches its agent, which refuses the space's processes, and lifting
	// it through B lets them back in.
	space := harness.CreateSpace(t, adminA.Client, "it-cluster-nocli", tmplId, adminA.Id)
	harness.DeleteSpaceAsync(t, adminA.Client, space)
	harness.WaitForSpaceReady(t, nodeA, adminA.Client, space)
	setField := func() string {
		out, _ := harness.TryRunCommand(adminA.Client, space, 60, "sh", "-c", "/usr/local/bin/knot agent set-field x y 2>&1; echo EXIT=$?")
		return out
	}
	ctx3, cancel3 := testCtx(60)
	defer cancel3()
	if !waitForCond(60, func() bool {
		_, _, err := adminB.Client.GetSpaceRestrictions(ctx3, space)
		return err == nil
	}) {
		t.Fatal("space created on node A never appeared on node B")
	}
	if _, _, err := adminB.Client.RestrictSpace(ctx3, space, &apiclient.SpaceRestrictRequest{NoCli: true}); err != nil {
		t.Fatalf("restrict through node B: %v", err)
	}
	if !waitForCond(60, func() bool { return strings.Contains(setField(), "restricted") }) {
		t.Fatalf("--no-cli set through node B never reached the space's agent: %q", setField())
	}
	if _, _, err := adminB.Client.ClearSpaceRestrictions(ctx3, space); err != nil {
		t.Fatalf("lift through node B: %v", err)
	}
	if !waitForCond(60, func() bool { return strings.Contains(setField(), "EXIT=0") }) {
		t.Fatalf("lifting --no-cli through node B never reached the space's agent: %q", setField())
	}
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
