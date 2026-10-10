package web

import (
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
)

func TestBuildNav_FullAdminNonLeaf(t *testing.T) {
	u := adminUser(t)
	cfg := &config.ServerConfig{} // non-leaf, no tunnels, no cluster, hideAPITokens=false

	sections := buildNav(u, cfg, true)
	assertEqual(t, []string{navWorkspace, navBuild, navAdmin}, sectionKeys(sections), "sections for full admin")

	assertEqual(t, []string{"/spaces", "/api-tokens"}, urls(section(sections, navWorkspace)), "workspace section")
	assertEqual(t, []string{
		"/templates", "/variables", "/stacks", "/volumes", "/scripts", "/events",
		"/skills", "/commands", "/mcp-servers",
	}, urls(section(sections, navBuild)), "build section")
	assertEqual(t, []string{"/users", "/groups", "/roles", "/audit-logs"}, urls(section(sections, navAdmin)), "admin section")

	for _, s := range sections {
		if s.DefaultOpen != (s.Key == navWorkspace) {
			t.Errorf("section %s: default open %v", s.Key, s.DefaultOpen)
		}
	}
}

func TestBuildNav_LeafNode_TemplatesAndVariablesInWorkspace(t *testing.T) {
	u := adminUser(t)
	cfg := &config.ServerConfig{LeafNode: true}

	sections := buildNav(u, cfg, true)

	// Leaf node: Tunnels hidden, Templates + Variables with the everyday items.
	assertEqual(t, []string{"/spaces", "/api-tokens", "/templates", "/variables"}, urls(section(sections, navWorkspace)), "leaf workspace")
	assertEqual(t, []string{"/stacks", "/volumes", "/scripts", "/events", "/skills", "/commands", "/mcp-servers"}, urls(section(sections, navBuild)), "leaf build")
	// Users/groups/roles/cluster are hidden on a leaf; audit logs are not
	// leaf-gated, so they still appear when audit storage is available.
	assertEqual(t, []string{"/audit-logs"}, urls(section(sections, navAdmin)), "leaf admin")
}

func TestBuildNav_EmptySectionsHidden(t *testing.T) {
	model.SetRoleCache(nil)
	plain := &model.User{Id: "u2", Username: "plain", Active: true}
	cfg := &config.ServerConfig{}

	// No permissions: only API Tokens, so only Workspace.
	sections := buildNav(plain, cfg, true)
	assertEqual(t, []string{navWorkspace}, sectionKeys(sections), "sections for a user with no permissions")

	cfg.UI.HideAPITokens = true
	if sections := buildNav(plain, cfg, true); len(sections) != 0 {
		t.Fatalf("expected no sections, got %v", sectionKeys(sections))
	}
}

func TestResolveNav_NoStars(t *testing.T) {
	u := adminUser(t)
	u.SetNavStarred(nil)
	cfg := &config.ServerConfig{}

	starred, sections := resolveNav(u, cfg, true, "/spaces")

	if starred != nil {
		t.Fatalf("expected nil starred, got %v", starred)
	}
	assertEqual(t, []string{navWorkspace, navBuild, navAdmin}, sectionKeys(sections), "sections")
	ws := sectionByKey(sections, navWorkspace)
	if !ws.Active || !ws.Items[0].Active {
		t.Fatal("/spaces should be the current item, in the Workspace section")
	}
	if sectionByKey(sections, navBuild).Active || sectionByKey(sections, navAdmin).Active {
		t.Fatal("only the section holding the current page is active")
	}
}

func TestResolveNav_StarredLeaveTheirSections(t *testing.T) {
	u := adminUser(t)
	u.SetNavStarred([]string{"/scripts", "/spaces"})
	cfg := &config.ServerConfig{}

	starred, sections := resolveNav(u, cfg, true, "/anything")

	// Starred items render in the stored order.
	assertEqual(t, []string{"/scripts", "/spaces"}, urls(starred), "starred order preserved")
	for _, it := range starred {
		if !it.Starred {
			t.Errorf("%s not marked starred", it.URL)
		}
	}
	assertEqual(t, []string{"/api-tokens"}, urls(section(sections, navWorkspace)), "workspace without starred")
	assertContainsNone(t, urls(section(sections, navBuild)), []string{"/scripts"})
}

func TestResolveNav_SectionHiddenWhenAllStarred(t *testing.T) {
	u := adminUser(t)
	u.SetNavStarred([]string{"/spaces", "/api-tokens"})
	cfg := &config.ServerConfig{}

	starred, sections := resolveNav(u, cfg, true, "/spaces")
	assertEqual(t, []string{"/spaces", "/api-tokens"}, urls(starred), "starred")
	assertEqual(t, []string{navBuild, navAdmin}, sectionKeys(sections), "workspace hidden once empty")
	if !starred[0].Active {
		t.Fatal("the starred copy of the current page should be marked current")
	}
}

func TestResolveNav_StalePinsDropped(t *testing.T) {
	u := adminUser(t)
	// /tunnels is hidden (no ListenTunnel); duplicates and unknown URLs must be
	// filtered out, leaving only the visible pin in stored order.
	u.SetNavStarred([]string{"/scripts", "/tunnels", "/bogus", "/scripts", "/spaces"})
	cfg := &config.ServerConfig{}

	starred, _ := resolveNav(u, cfg, true, "/")
	assertEqual(t, []string{"/scripts", "/spaces"}, urls(starred), "stale/hidden/duplicate pins removed")
}

func TestResolveNav_NestedPathOpensItsSection(t *testing.T) {
	u := adminUser(t)
	u.SetNavStarred(nil)
	cfg := &config.ServerConfig{}

	_, sections := resolveNav(u, cfg, true, "/scripts/123")
	if !sectionByKey(sections, navBuild).Active {
		t.Fatal("expected Build to open for a page under /scripts")
	}
}

// --- helpers ---

func sectionKeys(sections []NavSection) []string {
	out := make([]string, len(sections))
	for i, s := range sections {
		out[i] = s.Key
	}
	return out
}

func sectionByKey(sections []NavSection, key string) NavSection {
	for _, s := range sections {
		if s.Key == key {
			return s
		}
	}
	return NavSection{}
}

func section(sections []NavSection, key string) []NavItem {
	return sectionByKey(sections, key).Items
}

func allItems(sections []NavSection) []NavItem {
	var out []NavItem
	for _, s := range sections {
		out = append(out, s.Items...)
	}
	return out
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
