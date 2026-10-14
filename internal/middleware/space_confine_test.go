package middleware

import (
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

func TestSpaceConfinementAllows(t *testing.T) {
	space := &model.Space{Id: "11111111-1111-1111-1111-111111111111", Name: "dev"}
	const other = "22222222-2222-2222-2222-222222222222"

	cases := []struct {
		method, path string
		want         bool
	}{
		// Its own files, tunnels, scripts and identity.
		{"GET", "/api/files/buckets", true},
		{"PUT", "/api/files/objects/notes/a.txt", true},
		{"GET", "/api/users/whoami", true},
		{"GET", "/api/scripts/name/setup", true},
		{"GET", "/tunnel/spaces/web/8080", true},
		{"GET", "/api/forward-target", true},

		// Itself, by id or name.
		{"POST", "/api/spaces/" + space.Id + "/restrict", true},
		{"GET", "/api/spaces/dev/restrictions", true},
		{"POST", "/api/spaces/dev/run-command", true},
		{"GET", "/space-io/" + space.Id + "/run", true},
		{"GET", "/logs/" + space.Id + "/stream", true},
		{"DELETE", "/api/spaces/dev/restrictions", false},

		// Other spaces: details, start, stop and restart only.
		{"GET", "/api/spaces", true},
		{"GET", "/api/spaces/" + other, true},
		{"POST", "/api/spaces/" + other + "/start", true},
		{"POST", "/api/spaces/" + other + "/restart", true},
		{"POST", "/api/spaces/stacks/web/stop", true},
		{"POST", "/api/spaces/" + other + "/run-command", false},
		{"POST", "/api/spaces/" + other + "/execute-script", false},
		{"POST", "/api/spaces/" + other + "/files/read", false},
		{"GET", "/api/spaces/" + other + "/custom-field/x", false},
		{"PUT", "/api/spaces/" + other, false},
		{"DELETE", "/api/spaces/" + other, false},
		{"POST", "/api/spaces", false},
		{"GET", "/space-io/" + other + "/run", false},
		{"GET", "/proxy/spaces/" + other + "/terminal/bash", false},
		{"GET", "/proxy/spaces/web/port/8080", true},

		// Ways out: tokens, the owner's account, MCP, methods, writes elsewhere.
		{"POST", "/api/tokens", false},
		{"GET", "/api/tokens", false},
		{"PUT", "/api/users/u1", false},
		{"PUT", "/api/users/whoami/ssh-public-key", false},
		{"POST", "/mcp", false},
		{"POST", "/api/methods/call", false},
		{"POST", "/api/skill", false},
		{"PUT", "/api/scripts/s1", false},
		{"POST", "/api/pools/p1/acquire", false},
		{"GET", "/api/filesystem", false},
	}
	for _, c := range cases {
		if got := spaceConfinementAllows(space, c.method, c.path); got != c.want {
			t.Errorf("%s %s: got %v, want %v", c.method, c.path, got, c.want)
		}
	}
}

func TestSpaceCliAllows(t *testing.T) {
	space := &model.Space{Id: "11111111-1111-1111-1111-111111111111", Name: "dev"}
	cases := []struct {
		method, path string
		want         bool
	}{
		{"GET", "/api/spaces/dev/restrictions", true},
		{"GET", "/api/spaces/" + space.Id + "/restrictions", true},
		{"DELETE", "/api/spaces/dev/restrictions", false},
		{"POST", "/api/spaces/dev/restrict", false},
		{"GET", "/api/spaces/other/restrictions", false},
		{"GET", "/api/spaces/dev", false},
		{"GET", "/api/files/buckets", false},
		{"GET", "/api/users/whoami", false},
		{"POST", "/api/spaces/dev/restart", false},
		{"GET", "/tunnel/spaces/web/8080", false},
	}
	for _, c := range cases {
		if got := spaceCliAllows(space, c.method, c.path); got != c.want {
			t.Errorf("%s %s: got %v, want %v", c.method, c.path, got, c.want)
		}
	}
}
