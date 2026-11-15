package mcp

import (
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

// A remote server answers only its own namespaced names. A server with no
// namespace answers nothing, so an un-namespaced name, one of knot's own
// tools such as run_script, never maps to a remote server (where a view
// mounted from that server could call it).
func TestBareNameForNeverTakesKnotTools(t *testing.T) {
	bare := &model.MCPServer{Namespace: ""}
	for _, name := range []string{"run_script", "delete_space", "gh__x", ""} {
		if got, ok := bareNameFor(bare, name); ok {
			t.Errorf("server without a namespace took %q as %q", name, got)
		}
	}
	gh := &model.MCPServer{Namespace: "gh"}
	if got, ok := bareNameFor(gh, "gh__issues"); !ok || got != "issues" {
		t.Errorf("gh__issues = %q %v", got, ok)
	}
	for _, name := range []string{"run_script", "other__issues", "gh__", "ghx__issues"} {
		if got, ok := bareNameFor(gh, name); ok {
			t.Errorf("gh took %q as %q", name, got)
		}
	}
}
