package web

import (
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
)

func TestBuildNav_ModeA_FullAdminNonLeaf(t *testing.T) {
	u := adminUser(t)
	cfg := &config.ServerConfig{} // non-leaf, no tunnels, no cluster, hideAPITokens=false

	top, more := buildNav(u, cfg, true)

	topURLs := urls(top)
	wantTop := []string{"/spaces", "/api-tokens", "/volumes"}
	assertEqual(t, wantTop, topURLs, "Mode A top section for full admin (non-leaf)")

	// More is gated by permissions; admin has all, so expect the full set in
	// legacy order, including templates + variables (non-leaf).
	wantMore := []string{
		"/stacks", "/variables", "/templates", "/scripts", "/events",
		"/skills", "/commands", "/mcp-servers", "/users", "/groups",
		"/roles", "/audit-logs",
	}
	assertEqual(t, wantMore, urls(more), "Mode A more section for full admin (non-leaf)")
}

func TestBuildNav_LeafNode_PromotesTemplatesAndVariables(t *testing.T) {
	u := adminUser(t)
	cfg := &config.ServerConfig{LeafNode: true}

	top, more := buildNav(u, cfg, true)

	topURLs := urls(top)
	// Leaf node: Tunnels hidden, Templates + Variables promoted to top.
	wantTop := []string{"/spaces", "/api-tokens", "/volumes", "/templates", "/variables"}
	assertEqual(t, wantTop, topURLs, "leaf-node top section")

	// On a leaf, variables/templates/mcp-servers come from the leaf paths and
	// users/groups/roles/cluster are hidden (non-leaf gated). Audit logs are
	// not leaf-gated, so they still appear when audit storage is available.
	wantMore := []string{"/stacks", "/scripts", "/events", "/skills", "/commands", "/mcp-servers", "/audit-logs"}
	assertEqual(t, wantMore, urls(more), "leaf-node more section")
}

func TestResolveNav_ModeA_NoPins(t *testing.T) {
	u := adminUser(t)
	u.SetNavStarred(nil)
	cfg := &config.ServerConfig{}

	modeB, starred, top, more, moreActive := resolveNav(u, cfg, true, "/spaces")

	if modeB {
		t.Fatalf("expected Mode A with no pins")
	}
	if starred != nil {
		t.Fatalf("expected nil starred in Mode A, got %v", starred)
	}
	if len(top) == 0 || len(more) == 0 {
		t.Fatalf("expected default top/more populated in Mode A")
	}
	// /spaces is a top-level item in Mode A, so More must not auto-expand.
	if moreActive {
		t.Fatalf("/spaces is top-level in Mode A; More must not be active")
	}
}

func TestResolveNav_ModeB_DemotesPrimaryItemsToTopOfMore(t *testing.T) {
	u := adminUser(t)
	u.SetNavStarred([]string{"/scripts", "/spaces"}) // pin Scripts + Spaces
	cfg := &config.ServerConfig{}

	modeB, starred, top, more, _ := resolveNav(u, cfg, true, "/anything")

	if !modeB {
		t.Fatalf("expected Mode B with pins present")
	}

	// Pinned items render in the stored order.
	assertEqual(t, []string{"/scripts", "/spaces"}, urls(starred), "pinned order preserved")

	if top != nil {
		t.Fatalf("Mode B must not populate the default top list")
	}

	// More begins with the demoted primary items (those not pinned) in their
	// primary order, then the rest of the More items. API Tokens + Volumes
	// were demoted (Spaces is pinned so it's absent here).
	moreURLs := urls(more)
	if len(moreURLs) < 2 || (moreURLs[0] != "/api-tokens") || (moreURLs[1] != "/volumes") {
		t.Fatalf("expected demoted primary items (/api-tokens, /volumes) at top of More, got %v", moreURLs)
	}
	// /scripts and /spaces must not appear in More (they're pinned).
	assertContainsNone(t, moreURLs, []string{"/scripts", "/spaces"})
}

func TestResolveNav_StalePinsDropped(t *testing.T) {
	u := adminUser(t)
	// /tunnels is hidden (no ListenTunnel); duplicates and unknown URLs must be
	// filtered out, leaving only the visible pin in stored order.
	u.SetNavStarred([]string{"/scripts", "/tunnels", "/bogus", "/scripts", "/spaces"})
	cfg := &config.ServerConfig{}

	modeB, starred, _, _, _ := resolveNav(u, cfg, true, "/")
	if !modeB {
		t.Fatal("expected Mode B")
	}
	assertEqual(t, []string{"/scripts", "/spaces"}, urls(starred), "stale/hidden/duplicate pins removed")
}

func TestResolveNav_EmptyingPinsReturnsToModeA(t *testing.T) {
	u := adminUser(t)
	u.SetNavStarred([]string{"/scripts"})
	cfg := &config.ServerConfig{}

	if modeB, _, _, _, _ := resolveNav(u, cfg, true, "/"); !modeB {
		t.Fatal("expected Mode B with one pin")
	}

	u.SetNavStarred([]string{}) // clear
	modeB, starred, top, more, _ := resolveNav(u, cfg, true, "/")
	if modeB {
		t.Fatal("expected Mode A after clearing pins")
	}
	if starred != nil || top == nil || more == nil {
		t.Fatal("Mode A should have nil starred and populated top/more")
	}
}

func TestMoreActive_InModeB_DemotedPrimaryPage(t *testing.T) {
	u := adminUser(t)
	u.SetNavStarred([]string{"/scripts"})
	cfg := &config.ServerConfig{}

	// /spaces is normally top-level; in Mode B it lives inside More, so More
	// should auto-expand when the user is on /spaces.
	_, _, _, _, moreActive := resolveNav(u, cfg, true, "/spaces/123")
	if !moreActive {
		t.Fatal("expected More active for a demoted primary path in Mode B")
	}
}

// --- helpers ---

func urls(items []NavItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.URL
	}
	return out
}

func adminUser(t *testing.T) *model.User {
	t.Helper()
	// The admin role (with every permission) is seeded into the cache on
	// server startup; mirror that for tests so HasPermission returns true.
	if !model.RoleExists(model.RoleAdminUUID) {
		model.SetRoleCache(nil)
	}
	u := &model.User{Id: "u1", Username: "admin", Roles: []string{model.RoleAdminUUID}}
	return u
}

func assertEqual(t *testing.T, want, got []string, msg string) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("%s: want %v, got %v", msg, want, got)
	}
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("%s: at index %d want %q, got %q (full got=%v)", msg, i, want[i], got[i], got)
		}
	}
}

func assertContainsNone(t *testing.T, haystack, needles []string) {
	t.Helper()
	has := map[string]bool{}
	for _, h := range haystack {
		has[h] = true
	}
	for _, n := range needles {
		if has[n] {
			t.Fatalf("found %q in %v but it must be absent", n, haystack)
		}
	}
}
