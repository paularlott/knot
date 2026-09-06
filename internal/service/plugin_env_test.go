package service

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/plugins"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/scriptling/conversion"
)

// dispatchPage evaluates a loaded plugin's entry in a fresh run-as-user
// environment and calls a page handler — the core of HandlePluginPage minus
// the HTTP/template glue.
func dispatchPage(t *testing.T, client *apiclient.ApiClient, user *model.User, plugin *plugins.Plugin, handler string) any {
	t.Helper()
	env, err := NewPluginScriptlingEnv(client, user, plugin)
	if err != nil {
		t.Fatalf("env: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := env.EvalWithContext(ctx, plugin.EntrySource); err != nil {
		t.Fatalf("entry eval: %v", err)
	}
	// Handlers see the request object (method/path) and params dict, as the
	// web dispatcher provides them.
	if err := env.SetObjectVar("request", conversion.FromGo(map[string]any{"method": "GET", "path": "/test"})); err != nil {
		t.Fatalf("request: %v", err)
	}
	if err := env.SetObjectVar("params", conversion.FromGo(map[string]any{})); err != nil {
		t.Fatalf("params: %v", err)
	}
	result, err := env.CallFunctionWithContext(ctx, handler)
	if err != nil {
		t.Fatalf("handler %q: %v", handler, err)
	}
	return conversion.ToGo(result)
}

// TestPluginPageDispatch verifies the dispatch pipeline on a fixture plugin:
// fresh env, jailed libraries, entry evaluation, handler call, and Go
// conversion of the returned dict. The empty API mux stands in for the
// loopback — handlers that make no knot.* calls never touch it.
func TestPluginPageDispatch(t *testing.T) {
	rest.SetAPIMux(http.NewServeMux())
	config.SetServerConfig(&config.ServerConfig{})

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "paged")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `# /// script
# requires-scriptling = ">=0.1"
#
# [tool.knot]
# version = "1.0"
# permissions = ["read"]
#
# [[tool.knot.pages]]
# path = "/report"
# handler = "report"
# label = "Report"
# menu_label = "Report"
# ///
def report():
    return {"answer": 42, "name": "paged"}
`
	if err := os.WriteFile(filepath.Join(pluginDir, "main.py"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	registry, err := plugins.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	plugin, page := registry.Page("paged", "/report")
	if plugin == nil || page == nil {
		t.Fatalf("page not registered: %+v", registry.Failed())
	}
	if len(registry.All()[0].Menus) != 1 || registry.All()[0].Menus[0].URL != "/plugins/paged/report" || registry.All()[0].Menus[0].Label != "Report" {
		t.Errorf("auto menu = %+v", registry.All()[0].Menus)
	}

	user := &model.User{Username: "tester", Roles: []string{model.RoleAdminUUID}}
	got := dispatchPage(t, apiclient.NewMuxClient(user), user, plugin, page.Handler)
	dict, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("result = %#v, want dict", got)
	}
	if dict["name"] != "paged" {
		t.Errorf("name = %v", dict["name"])
	}
	if fmt.Sprintf("%v", dict["answer"]) != "42" {
		t.Errorf("answer = %#v (%T), want 42", dict["answer"], dict["answer"])
	}
}

// TestPluginPageDispatchDashboard dispatches the demo dashboard page:
// with an empty API mux the loopback list fails and the handler degrades to
// zero aggregates rather than erroring — resilience by design.
func TestPluginPageDispatchDashboard(t *testing.T) {
	rest.SetAPIMux(http.NewServeMux())
	config.SetServerConfig(&config.ServerConfig{})

	registry, err := plugins.Load(filepath.Join("..", "..", "examples", "plugins"))
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	plugin, page := registry.Page("dashboard", "/home")
	if plugin == nil || page == nil {
		t.Fatalf("dashboard not loaded: %+v", registry.Failed())
	}
	if !page.Default {
		t.Error("dashboard page should claim the login default")
	}
	if got := registry.DefaultPageURL(); got != "/plugins/dashboard/home" {
		t.Errorf("DefaultPageURL = %q", got)
	}

	user := &model.User{Username: "tester", Roles: []string{model.RoleAdminUUID}}
	got := dispatchPage(t, apiclient.NewMuxClient(user), user, plugin, page.Handler)
	dict, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("result = %#v, want a block document", got)
	}
	rows, ok := dict["rows"].([]any)
	if !ok || len(rows) < 2 {
		t.Fatalf("rows = %#v", dict["rows"])
	}
}

// TestPluginPageDispatchDashboardCharts runs the demo dashboard page
// against a fake loopback serving two running spaces, then calls column
// handlers directly - the rows/columns contract's per-column dispatch.
func TestPluginPageDispatchDashboardCharts(t *testing.T) {
	now := time.Now().UTC()
	usage := func(cpu float64, memUsed, memLimit, diskUsed, diskLimit uint64) map[string]any {
		return map[string]any{
			"cpu_percent":        cpu,
			"memory_used_bytes":  memUsed,
			"memory_limit_bytes": memLimit,
			"disk_used_bytes":    diskUsed,
			"disk_limit_bytes":   diskLimit,
		}
	}
	space := func(id, name string, u map[string]any) map[string]any {
		return map[string]any{
			"id": id, "name": name, "template_name": "basic",
			"is_deployed": true, "is_pending": false, "is_deleting": false,
			"node_hostname": "node1", "resource_usage": u,
		}
	}
	gib := func(g float64) uint64 { return uint64(g * 1073741824) }

	var uris []string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users/whoami", func(w http.ResponseWriter, r *http.Request) {
		rest.WriteResponse(http.StatusOK, w, r, map[string]any{"user_id": "u-1", "id": "u-1"})
	})
	mux.HandleFunc("GET /api/spaces", func(w http.ResponseWriter, r *http.Request) {
		uris = append(uris, r.URL.RequestURI())
		rest.WriteResponse(http.StatusOK, w, r, map[string]any{
			"count": 2,
			"spaces": []map[string]any{
				space("s-1", "alpha", usage(12.5, gib(3), gib(4), gib(10), gib(20))),
				space("s-2", "beta", usage(4.5, gib(1.5), gib(4), gib(5), gib(20))),
			},
		})
	})
	mux.HandleFunc("GET /api/spaces/{space_id}/usage/history", func(w http.ResponseWriter, r *http.Request) {
		uris = append(uris, r.URL.RequestURI())
		points := []map[string]any{}
		for i := 2; i >= 0; i-- {
			points = append(points, map[string]any{
				"bucket_start":   now.Add(-time.Duration(i) * time.Minute),
				"resource_usage": usage(float64(10+i), gib(2+float64(i)), gib(4), gib(9), gib(20)),
			})
		}
		rest.WriteResponse(http.StatusOK, w, r, map[string]any{"space_id": r.PathValue("space_id"), "points": points})
	})
	rest.SetAPIMux(mux)
	config.SetServerConfig(&config.ServerConfig{})

	registry, err := plugins.Load(filepath.Join("..", "..", "examples", "plugins"))
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	plugin, page := registry.Page("dashboard", "/home")
	if plugin == nil || page == nil {
		t.Fatalf("dashboard not loaded: %+v", registry.Failed())
	}

	user := &model.User{Username: "tester", Roles: []string{model.RoleAdminUUID}}
	got := dispatchPage(t, apiclient.NewMuxClient(user), user, plugin, page.Handler)
	dict, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("result = %#v, want a layout document", got)
	}
	rows, ok := dict["rows"].([]any)
	if !ok || len(rows) < 2 {
		t.Fatalf("rows = %#v", dict["rows"])
	}

	// Column data handlers produce chart payloads with real history.
	series := dispatchPage(t, apiclient.NewMuxClient(user), user, plugin, "col_series")
	chart, ok := series.(map[string]any)
	if !ok || len(chart["labels"].([]any)) < 2 {
		t.Fatalf("col_series = %#v", series)
	}
	spaces := dispatchPage(t, apiclient.NewMuxClient(user), user, plugin, "col_spaces")
	spacesTable, ok := spaces.(map[string]any)
	if !ok || len(spacesTable["rows"].([]any)) < 1 {
		t.Fatalf("col_spaces = %#v", spaces)
	}
	top := dispatchPage(t, apiclient.NewMuxClient(user), user, plugin, "col_top")
	topChart, ok := top.(map[string]any)
	if !ok || len(topChart["labels"].([]any)) < 1 {
		t.Fatalf("col_top = %#v", top)
	}

	sawUser, sawRange := false, false
	for _, uri := range uris {
		if strings.Contains(uri, "/api/spaces?") && strings.Contains(uri, "user_id=u-1") {
			sawUser = true
		}
		if strings.Contains(uri, "/usage/history?") && strings.Contains(uri, "range=1h") {
			sawRange = true
		}
	}
	if !sawUser {
		t.Errorf("user_id not forwarded on /api/spaces, requests = %v", uris)
	}
	if !sawRange {
		t.Errorf("range not forwarded on usage history, requests = %v", uris)
	}
}

// TestPluginPageDispatchPeer exercises the full binary-peer path when the
// demo-go example's peer has been built (make in examples/plugins/demo-go):
// the plugin env registers the peer's proxy library, and the handler's
// `import plugin.demolib` call reaches the Go process.
func TestPluginPageDispatchPeer(t *testing.T) {
	peerPath := filepath.Join("..", "..", "examples", "plugins", "demo-go", "bin", "demolib_"+runtime.GOOS+"_"+runtime.GOARCH)
	if _, err := os.Stat(peerPath); err != nil {
		t.Skip("demo-go peer not built (run make in examples/plugins/demo-go)")
	}

	rest.SetAPIMux(http.NewServeMux())
	config.SetServerConfig(&config.ServerConfig{})

	registry, err := plugins.Load(filepath.Join("..", "..", "examples", "plugins"))
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	plugin, page := registry.Page("demo-go", "/status")
	if plugin == nil || page == nil {
		t.Fatalf("demo-go not loaded with peer built: %+v", registry.Failed())
	}

	user := &model.User{Username: "tester", Roles: []string{model.RoleAdminUUID}}
	got := dispatchPage(t, apiclient.NewMuxClient(user), user, plugin, page.Handler)
	dict, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("result = %#v, want a block document", got)
	}
	// The layout references per-column handlers; the peer identity and
	// real round-trips surface through them.
	rows, ok := dict["rows"].([]any)
	if !ok || len(rows) < 1 {
		t.Fatalf("rows = %#v", dict["rows"])
	}
	peer := dispatchPage(t, apiclient.NewMuxClient(user), user, plugin, "col_peer")
	peerTable, ok := peer.(map[string]any)
	if !ok {
		t.Fatalf("col_peer = %#v", peer)
	}
	dump := fmt.Sprintf("%#v", peerTable)
	if !strings.Contains(dump, "demolib") || !strings.Contains(dump, runtime.GOOS) {
		t.Errorf("peer identity missing from col_peer: %s", dump)
	}
}
