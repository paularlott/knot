package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/paularlott/knot/internal/plugins"
)

// serveAsset routes a request through the plugin asset handler with the
// {plugin_name} and {asset_path...} path values the real router supplies.
func serveAsset(plugin, asset string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /plugins/{plugin_name}/assets/{asset_path...}", HandlePluginAsset)
	r := httptest.NewRequest("GET", "/plugins/"+plugin+"/assets/"+asset, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

// TestPluginAssetServingBothKinds pins that declared assets serve from the
// plugin folder for BOTH plugin kinds — a pure-script plugin (demo-scriptling)
// and a peer plugin whose manifest came from the peer handshake (demo-go,
// no main.py). Both keep their logos/icons in the folder on disk, so one
// disk-backed handler serves them; a peer needs no fetch path. Undeclared
// paths stay 404, so the handler is not a general file server.
func TestPluginAssetServingBothKinds(t *testing.T) {
	examples := filepath.Join("..", "examples", "plugins")
	if _, err := os.Stat(examples); err != nil {
		t.Skip("examples/plugins not present")
	}
	registry, err := plugins.Load(examples)
	if err != nil {
		t.Fatal(err)
	}
	plugins.SetRegistry(registry)
	t.Cleanup(func() {
		registry.Close()
		plugins.SetRegistry(nil)
	})

	for _, name := range []string{"demo-go", "demo-scriptling"} {
		p := registry.ByName(name)
		if p == nil {
			// demo-go only loads once its peer is built; skip rather than
			// fail so the suite passes on a clean checkout.
			t.Logf("%s not loaded (peer not built?); skipping", name)
			continue
		}
		if p.LogoLight == "" {
			t.Errorf("%s declares no logo to serve", name)
			continue
		}
		// The declared logo serves with an image content type. The asset
		// path value the router captures after /assets/ is the declared
		// path verbatim (which itself begins "assets/"), so the served URL
		// is /plugins/<name>/assets/<LogoLight>.
		w := serveAsset(name, p.LogoLight)
		if w.Code != http.StatusOK {
			t.Errorf("%s logo %q status = %d, body = %s", name, p.LogoLight, w.Code, w.Body.String())
		}
		if ct := w.Header().Get("Content-Type"); ct == "" {
			t.Errorf("%s logo served without a content type", name)
		}

		// An undeclared path in the same folder is refused.
		if w := serveAsset(name, "../main.py"); w.Code == http.StatusOK {
			t.Errorf("%s served an undeclared path (../main.py) with 200", name)
		}
		if w := serveAsset(name, "nope.svg"); w.Code == http.StatusOK {
			t.Errorf("%s served an undeclared asset with 200", name)
		}
	}
}
