package web

import (
	"testing"

	"github.com/paularlott/knot/internal/config"
)

// TestNewTemplateEmbeddedCache pins the embedded-template memoization:
// repeated renders reuse the parsed template instead of re-parsing every
// partial and layout. The server.template_path override branch runs before
// the cache, so a dev disk override always wins over a cached entry.
func TestNewTemplateEmbeddedCache(t *testing.T) {
	config.SetServerConfig(&config.ServerConfig{})

	first, err := newTemplate("page-plugin.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	second, err := newTemplate("page-plugin.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Error("newTemplate re-parsed the embedded template; want the cached instance")
	}
}
