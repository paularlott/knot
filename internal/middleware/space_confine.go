package middleware

import (
	"net/http"
	"strings"

	"github.com/paularlott/knot/internal/database/model"
)

// A restricted space's own credential is confined, so its restrictions
// can't be stepped around: it can't make API tokens, run commands or
// scripts in other spaces, call their methods, change the owner's account
// or reach the MCP endpoint, each of which would act with the owner's full
// access. What it may still do is listed below; everything else is refused.

// confinedReadPaths may be read (GET) by a restricted space.
var confinedReadPaths = []string{
	"/api/ping",
	"/api/ok",
	"/api/server-info",
	"/api/capabilities",
	"/api/icons",
	"/api/users/whoami",
	"/api/spaces",
	"/api/templates",
	"/api/forward-target",
	"/api/tunnels/server-info",
}

// confinedReadPrefixes cover the paths below them, for reading only.
var confinedReadPrefixes = []string{
	"/api/scripts", // named scripts and their libraries, run in the space
	"/api/skill",
}

// confinedPrefixes may be used with any method. File storage is narrowed
// further by the space's bucket restrictions.
var confinedPrefixes = []string{
	"/api/files",
	"/api/tunnels",
	"/tunnel/",
}

// confinedOtherSpaceActions may be taken on other spaces and stacks: they
// change whether a space runs, never what is in it.
var confinedOtherSpaceActions = map[string]bool{"start": true, "stop": true, "restart": true}

// spaceConfinementAllows reports whether a restricted space may make the
// request with its own credential.
func spaceConfinementAllows(space *model.Space, method, path string) bool {
	if method == http.MethodGet || method == http.MethodHead {
		for _, p := range confinedReadPaths {
			if path == p {
				return true
			}
		}
		for _, p := range confinedReadPrefixes {
			if underPath(path, p) {
				return true
			}
		}
	}
	for _, p := range confinedPrefixes {
		if underPath(path, p) {
			return true
		}
	}

	// The space's own streams — commands, copies, ports, tunnels, jobs,
	// logs and its web proxies — but not another space's.
	for _, prefix := range []string{"/space-io/", "/proxy/spaces/", "/logs/"} {
		if rest, ok := strings.CutPrefix(path, prefix); ok {
			parts := strings.Split(rest, "/")
			if parts[0] == space.Id || parts[0] == space.Name {
				return true
			}
			// Another space's web ports, as a tunnel would reach them.
			return prefix == "/proxy/spaces/" && len(parts) >= 2 && parts[1] == "port"
		}
	}

	rest, ok := strings.CutPrefix(path, "/api/spaces/")
	if !ok {
		return false
	}
	parts := strings.Split(rest, "/")

	// Stacks: start, stop or restart only.
	if parts[0] == "stacks" {
		return method == http.MethodPost && len(parts) == 3 && confinedOtherSpaceActions[parts[2]]
	}

	// The space itself: anything, except lifting its own restrictions.
	if parts[0] == space.Id || parts[0] == space.Name {
		return !(method == http.MethodDelete && len(parts) == 2 && parts[1] == "restrictions")
	}

	// Another space: read its details, or start, stop or restart it.
	switch {
	case len(parts) == 1:
		return method == http.MethodGet || method == http.MethodHead
	case len(parts) == 2:
		return method == http.MethodPost && confinedOtherSpaceActions[parts[1]]
	}
	return false
}

// underPath reports whether path is prefix or below it, across a path
// boundary only: "/api/files" must not cover "/api/filesystem".
func underPath(path, prefix string) bool {
	if path == prefix {
		return true
	}
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	return prefix[len(prefix)-1] == '/' || path[len(prefix)] == '/'
}

// spaceCliAllows reports whether processes in a space whose command line is
// restricted (knot agent restrict --no-cli) may make the request with the
// token the agent hands them: only reading the space's own restrictions.
// Restarting and shutting the space down go through the agent instead.
func spaceCliAllows(space *model.Space, method, path string) bool {
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}
	rest, ok := strings.CutPrefix(path, "/api/spaces/")
	if !ok {
		return false
	}
	parts := strings.Split(rest, "/")
	return len(parts) == 2 && (parts[0] == space.Id || parts[0] == space.Name) && parts[1] == "restrictions"
}
