package middleware

import (
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

func TestTokenScopeAllows(t *testing.T) {
	cases := []struct {
		name   string
		scopes []string
		path   string
		want   bool
	}{
		// Tunnels scope: the tunnel websockets and the tunnel API.
		{"tunnels web tunnel", []string{model.ScopeTunnels}, "/tunnel/server/myapp", true},
		{"tunnels port tunnel", []string{model.ScopeTunnels}, "/tunnel/spaces/web/8080", true},
		{"tunnels api list", []string{model.ScopeTunnels}, "/api/tunnels", true},
		{"tunnels api server info", []string{model.ScopeTunnels}, "/api/tunnels/server-info", true},
		{"tunnels api delete", []string{model.ScopeTunnels}, "/api/tunnels/myapp", true},
		{"tunnels denies spaces", []string{model.ScopeTunnels}, "/api/spaces", false},
		{"tunnels denies methods", []string{model.ScopeTunnels}, "/api/methods/call", false},
		{"tunnels denies mcp", []string{model.ScopeTunnels}, "/mcp", false},
		{"tunnels denies near-miss tunnel path", []string{model.ScopeTunnels}, "/api/tunnels-extra", false},

		// The pre-existing scopes keep their shape.
		{"methods allows", []string{model.ScopeMethods}, "/api/methods/list", true},
		{"methods denies tunnels", []string{model.ScopeMethods}, "/tunnel/server/myapp", false},
		{"mcp allows", []string{model.ScopeMCP}, "/mcp", true},
		{"mcp denies api", []string{model.ScopeMCP}, "/api/spaces", false},

		// Combined scopes union.
		{"combined tunnels and methods", []string{model.ScopeTunnels, model.ScopeMethods}, "/api/methods/call", true},
		{"combined still denies the rest", []string{model.ScopeTunnels, model.ScopeMethods}, "/api/spaces", false},

		// Unknown scopes cover nothing.
		{"unknown scope allows nothing", []string{"everything"}, "/api/spaces", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := tokenScopeAllows(c.scopes, c.path); got != c.want {
				t.Fatalf("tokenScopeAllows(%v, %q) = %v, want %v", c.scopes, c.path, got, c.want)
			}
		})
	}
}

func TestTunnelsScopeIsKnown(t *testing.T) {
	if !model.IsKnownTokenScope(model.ScopeTunnels) {
		t.Fatal("tunnels must be a known token scope so the create/edit API accepts it")
	}
	// Every scope the middleware can enforce must be known to the model —
	// an enforced-but-unknowable scope could never be granted.
	for scope := range tokenScopeAllowedPaths {
		if !model.IsKnownTokenScope(scope) {
			t.Fatalf("middleware enforces unknown scope %q", scope)
		}
	}
}
