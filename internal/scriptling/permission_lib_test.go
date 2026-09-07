package scriptling

import (
	"embed"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

//go:embed lib/knot/permission.py
var permissionLibFS embed.FS

// pyConstantNames maps permission keys to the constant names the embedded
// knot.permission library uses where they differ from the mechanical
// UPPER_SNAKE of the key.
var pyConstantNames = map[string]string{
	"use_vs_code_tunnel": "USE_VSCODE_TUNNEL",
}

// TestPermissionLibConstantsInSync guards the embedded knot.permission
// library against drift: every built-in permission appears as a constant
// with the exact id, so scripts using knot.permission.MANAGE_SPACES agree
// with the server's own checks. A new permission without a constant, or a
// renumbered id, fails here.
func TestPermissionLibConstantsInSync(t *testing.T) {
	source, err := permissionLibFS.ReadFile("lib/knot/permission.py")
	if err != nil {
		t.Fatal(err)
	}
	py := map[string]string{}
	for _, m := range regexp.MustCompile(`(?m)^([A-Z][A-Z0-9_]*) = (\d+)$`).FindAllStringSubmatch(string(source), -1) {
		py[m[1]] = m[2]
	}

	for _, pn := range model.PermissionNames {
		key := model.PermissionKey(uint16(pn.Id))
		if key == "" {
			t.Errorf("permission id %d (%s) has no stable key", pn.Id, pn.Name)
			continue
		}
		name := pyConstantNames[key]
		if name == "" {
			name = strings.ToUpper(key)
		}
		got, ok := py[name]
		if !ok {
			t.Errorf("knot.permission: constant %s (%s) missing from the embedded library", name, key)
			continue
		}
		if want := fmt.Sprintf("%d", pn.Id); got != want {
			t.Errorf("knot.permission.%s = %s, want %s (id of %s)", name, got, want, key)
		}
	}
}
