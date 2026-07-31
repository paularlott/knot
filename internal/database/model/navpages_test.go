package model

import (
	"testing"

	"github.com/paularlott/knot/internal/config"
)

// allPermsUser uses the admin role (seeded into the cache) so every gate passes.
func allPermsUser(t *testing.T) *User {
	t.Helper()
	if !RoleExists(RoleAdminUUID) {
		SetRoleCache(nil)
	}
	return &User{Id: "u1", Username: "admin", Roles: []string{RoleAdminUUID}}
}

func pageURLs(pages []NavPage) []string {
	out := make([]string, len(pages))
	for i, p := range pages {
		out[i] = p.URL
	}
	return out
}

func TestVisibleNavPages_AdminNonLeaf(t *testing.T) {
	u := allPermsUser(t)
	cfg := &config.ServerConfig{} // non-leaf, no ListenTunnel, no cluster advertise

	got := pageURLs(VisibleNavPages(u, cfg, true))

	// Admin sees the full set in sidebar order; no tunnels (no ListenTunnel)
	// and no cluster-info (no advertise addr) on a non-leaf.
	want := []string{
		"/spaces", "/api-tokens", "/volumes", "/templates", "/variables",
		"/stacks", "/scripts", "/events", "/skills", "/commands",
		"/mcp-servers", "/users", "/groups", "/roles", "/audit-logs",
	}
	assertPageEqual(t, want, got)
}

func TestVisibleNavPages_LeafPromotesTemplatesAndVariables(t *testing.T) {
	u := allPermsUser(t)
	cfg := &config.ServerConfig{LeafNode: true}

	got := pageURLs(VisibleNavPages(u, cfg, true))

	// Leaf hides users/groups/roles/cluster; keeps mcp-servers + audit-logs.
	want := []string{
		"/spaces", "/api-tokens", "/volumes", "/templates", "/variables",
		"/stacks", "/scripts", "/events", "/skills", "/commands",
		"/mcp-servers", "/audit-logs",
	}
	assertPageEqual(t, want, got)
}

func TestVisibleNavPages_AuditGatedByAvailability(t *testing.T) {
	u := allPermsUser(t)
	cfg := &config.ServerConfig{}

	if has := contains(pageURLs(VisibleNavPages(u, cfg, true)), "/audit-logs"); !has {
		t.Fatal("audit-logs should be visible when auditAvailable=true")
	}
	if has := contains(pageURLs(VisibleNavPages(u, cfg, false)), "/audit-logs"); has {
		t.Fatal("audit-logs must be hidden when auditAvailable=false")
	}
}

func TestVisibleNavPages_NoPermissions(t *testing.T) {
	// A user with no roles sees only the always-available pages.
	u := &User{Id: "u2", Username: "nobody", Roles: nil}
	cfg := &config.ServerConfig{}

	got := pageURLs(VisibleNavPages(u, cfg, true))
	// Only API Tokens is unconditional (!HideAPITokens); everything else is
	// permission-gated. Spaces requires useSpaces/manageSpaces which nobody lacks.
	want := []string{"/api-tokens"}
	assertPageEqual(t, want, got)
}

func TestVisibleNavPages_HideAPITokens(t *testing.T) {
	u := allPermsUser(t)
	cfg := &config.ServerConfig{}
	cfg.UI.HideAPITokens = true

	if has := contains(pageURLs(VisibleNavPages(u, cfg, true)), "/api-tokens"); has {
		t.Fatal("/api-tokens must be hidden when HideAPITokens is true")
	}
}

func assertPageEqual(t *testing.T, want, got []string) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("at %d want %q, got %q (full got=%v)", i, want[i], got[i], got)
		}
	}
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
