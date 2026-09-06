package web

import (
	"testing"

	"github.com/paularlott/knot/internal/config"
)

// TestPluginTemplatesParse guards against template syntax errors in the
// plugin surface: the admin page and the shared nav partial that gained the
// external-link and logo rendering.
func TestPluginTemplatesParse(t *testing.T) {
	config.SetServerConfig(&config.ServerConfig{})
	for _, name := range []string{"page-plugins.tmpl", "page-roles.tmpl"} {
		if _, err := newTemplate(name); err != nil {
			t.Errorf("newTemplate(%s): %v", name, err)
		}
	}
}
