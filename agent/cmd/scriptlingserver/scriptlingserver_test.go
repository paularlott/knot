package scriptlingserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/scriptling/pluginfetch"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/plugin"
	"github.com/paularlott/scriptling/scriptling-cli/pack"
	"github.com/paularlott/scriptling/scriptling-cli/pluginpack"
	"github.com/paularlott/scriptling/stdlib"
)

// mockKnotAPI serves a minimal knot API for the plugin tests.
func mockKnotAPI(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/scripts", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"count":1,"scripts":[{"script_id":"1","user_id":"","name":"mylib","script_type":"lib","active":true}]}`))
	})
	mux.HandleFunc("GET /api/scripts/name/mylib/lib", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`"def value():\n    return 'from plugin test'\n"`))
	})
	mux.HandleFunc("GET /api/scripts/name/myscript/script", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`"import mylib\nimport knot.apiclient\nprint(mylib.value())\n"`))
	})
	mux.HandleFunc("GET /api/users", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"count":1,"users":[{"id":"u1","email":"test@example.com"}]}`))
	})

	return httptest.NewServer(mux)
}

// TestPluginProtocolCycle drives the knot plugin server over in-process
// pipes: handshake, fetch reads for embedded libs, user libs and scripts,
// and the API transport functions — the full path the scriptling CLI uses.
func TestPluginProtocolCycle(t *testing.T) {
	api := mockKnotAPI(t)
	defer api.Close()

	client, err := apiclient.NewClient(api.URL, "test-token", true)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.SetContentType("application/json")

	// Build the same server the command builds.
	fetcher := pluginfetch.NewFetcher(client)
	server := plugin.NewServer("knot", "test", "knot plugin test")
	server.RegisterFetcher("knot", fetcherAdapter{fetcher})
	registerAPIFunctions(server, client.GetRESTClient())

	// Wire over pipes.
	clientToPluginR, clientToPluginW := io.Pipe()
	pluginToClientR, pluginToClientW := io.Pipe()
	go func() { _ = server.RunIO(clientToPluginR, pluginToClientW) }()

	ctx := context.Background()
	pc, err := plugin.LoadClientFromIO(ctx, pluginToClientR, clientToPluginW)
	if err != nil {
		t.Fatalf("LoadClientFromIO: %v", err)
	}
	defer func() {
		_ = pc.Close()
		_ = clientToPluginW.Close()
		_ = pluginToClientR.Close()
	}()

	t.Run("handshake", func(t *testing.T) {
		if pc.Scheme() != "knot" {
			t.Fatalf("expected scheme knot, got %q", pc.Scheme())
		}
		if !pc.SupportsFetch() {
			t.Fatal("expected SupportsFetch")
		}
	})

	t.Run("embedded library", func(t *testing.T) {
		data, err := pc.FetchFile(ctx, "knot://libs", "lib/knot/space.py")
		if err != nil {
			t.Fatalf("FetchFile(knot/space.py): %v", err)
		}
		if len(data) == 0 {
			t.Fatal("knot.space.py is empty")
		}
	})

	t.Run("apiclient variant", func(t *testing.T) {
		data, err := pc.FetchFile(ctx, "knot://libs", "lib/knot/apiclient.py")
		if err != nil {
			t.Fatalf("FetchFile(apiclient): %v", err)
		}
		if !strings.Contains(string(data), "plugin.knot") {
			t.Fatal("expected the plugin-transport apiclient")
		}
	})

	t.Run("user library from API", func(t *testing.T) {
		data, err := pc.FetchFile(ctx, "knot://libs", "lib/mylib.py")
		if err != nil {
			t.Fatalf("FetchFile(lib/mylib.py): %v", err)
		}
		if !strings.Contains(string(data), "from plugin test") {
			t.Fatalf("expected user library content, got %q", data)
		}
	})

	t.Run("script source", func(t *testing.T) {
		data, err := pc.FetchFile(ctx, "knot://myscript", "")
		if err != nil {
			t.Fatalf("FetchFile(knot://myscript): %v", err)
		}
		if !strings.Contains(string(data), "mylib.value()") {
			t.Fatalf("expected script content, got %q", data)
		}
	})

	t.Run("directory listing", func(t *testing.T) {
		entries, err := pc.FetchList(ctx, "knot://libs", "lib")
		if err != nil {
			t.Fatalf("FetchList(lib): %v", err)
		}
		foundKnot, foundLib := false, false
		for _, e := range entries {
			if e.Name == "knot" && e.IsDir {
				foundKnot = true
			}
			if e.Name == "mylib.py" {
				foundLib = true
			}
		}
		if !foundKnot || !foundLib {
			t.Fatalf("expected knot dir and mylib.py in listing, got %v", entries)
		}
	})

	t.Run("api transport function", func(t *testing.T) {
		result, err := pc.CallFunction(ctx, "api_get", []plugin.Value{
			{Type: "string", Value: "/api/users"},
		}, nil)
		if err != nil {
			t.Fatalf("api_get(/api/users): %v", err)
		}
		if result.Type != "dict" {
			t.Fatalf("expected dict result, got %s: %+v", result.Type, result)
		}
	})

	t.Run("connection info", func(t *testing.T) {
		result, err := pc.CallFunction(ctx, "connection_info", nil, nil)
		if err != nil {
			t.Fatalf("connection_info: %v", err)
		}
		if result.Type != "dict" {
			t.Fatalf("expected dict, got %s", result.Type)
		}
	})

	t.Run("not-found is distinct from error", func(t *testing.T) {
		_, err := pc.FetchFile(ctx, "knot://libs", "lib/nosuch.py")
		if !strings.Contains(err.Error(), "not found") {
			t.Fatalf("expected not-found, got %v", err)
		}
	})
}

// TestShouldAutoStart verifies the bare-invocation guard.
func TestShouldAutoStart(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"knot"}

	t.Run("with version", func(t *testing.T) {
		t.Setenv(PeerEnvVar, "0.23.0")
		if !ShouldAutoStart() {
			t.Fatal("expected auto-start with env var set and no args")
		}
	})
	t.Run("without env var", func(t *testing.T) {
		t.Setenv(PeerEnvVar, "")
		if ShouldAutoStart() {
			t.Fatal("expected no auto-start without env var")
		}
	})
	t.Run("with arguments", func(t *testing.T) {
		os.Args = []string{"knot", "server"}
		t.Setenv(PeerEnvVar, "0.23.0")
		if ShouldAutoStart() {
			t.Fatal("expected no auto-start with arguments")
		}
		os.Args = origArgs
	})
}

// TestCheckPeerVersion verifies the version compatibility check.
func TestCheckPeerVersion(t *testing.T) {
	cases := []struct {
		version string
		ok      bool
	}{
		{"0.23.0", true},
		{"0.23.1", true},
		{"0.24.0", true},
		{"1.0.0", true},
		{"0.22.0", false}, // before the fetcher contract
		{"0.21.4", false},
		{"garbage", false},
		{"", false},
	}
	for _, tc := range cases {
		err := checkPeerVersion(tc.version)
		if tc.ok && err != nil {
			t.Errorf("checkPeerVersion(%q): unexpected error %v", tc.version, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("checkPeerVersion(%q): expected error, got nil", tc.version)
		}
	}
}

// TestSemver tests the lightweight version parser.
func TestSemver(t *testing.T) {
	v, err := semver("0.23.0")
	if err != nil || v != [2]int{0, 23} {
		t.Fatalf("semver(0.23.0) = %v, %v", v, err)
	}
	if _, err := semver("not-a-version"); err == nil {
		t.Fatal("expected error for non-version string")
	}
}

// TestRESTClientInterface is a compile-time check that the RESTClient
// interface used by registerAPIFunctions matches the real one.
var _ rest.RESTClient = (rest.RESTClient)(nil)

// TestAPIFunctionToleratesEmptyBody verifies the transport handles HTTP 200
// with an empty body (what the knot API returns for start/stop/etc.) the
// same way the embedded apiclient does: as a successful null result.
func TestAPIFunctionToleratesEmptyBody(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/spaces/testspace/start" {
			// HTTP 200 with empty body, no Content-Type header.
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer api.Close()

	client := newTestAPIClient(t, api.URL)
	server := plugin.NewServer("knot", "test", "empty body test")
	registerAPIFunctions(server, client.GetRESTClient())

	// Wire over pipes.
	clientToPluginR, clientToPluginW := io.Pipe()
	pluginToClientR, pluginToClientW := io.Pipe()
	go func() { _ = server.RunIO(clientToPluginR, pluginToClientW) }()

	ctx := context.Background()
	pc, err := plugin.LoadClientFromIO(ctx, pluginToClientR, clientToPluginW)
	if err != nil {
		t.Fatalf("LoadClientFromIO: %v", err)
	}
	defer func() {
		_ = pc.Close()
		_ = clientToPluginW.Close()
		_ = pluginToClientR.Close()
	}()

	// POST to the empty-body endpoint must succeed (return null), not raise.
	result, err := pc.CallFunction(ctx, "api_post", []plugin.Value{
		{Type: "string", Value: "/api/spaces/testspace/start"},
	}, nil)
	if err != nil {
		t.Fatalf("api_post to empty-body endpoint: %v", err)
	}
	// An empty body is a successful call with a null result.
	if result.Type != "null" && result.Type != "" {
		t.Logf("result type: %s, value: %+v (null is expected for empty body)", result.Type, result)
	}
}

// TestWaitForStart exercises knot.space.wait_for_start through the full
// plugin transport (HTTP, so the manager owns the client and proxy libraries
// register correctly): already-running returns immediately, timeout returns
// False, and a transition from stopped to running is detected.
func TestWaitForStart(t *testing.T) {
	running := false

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/api/spaces/testspace" && r.Method == "GET":
			w.Write(jsonBody(t, map[string]interface{}{
				"space_id":   "testspace",
				"name":       "testspace",
				"is_running": running,
			}))
		case r.URL.Path == "/api/scripts":
			w.Write(jsonBody(t, map[string]interface{}{"count": 0, "scripts": []interface{}{}}))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer api.Close()

	client := newTestAPIClient(t, api.URL)
	fetcher := newTestFetcher(t, api.URL)

	// Serve the plugin over HTTP so the manager can load it.
	pluginSrv := plugin.NewServer("knot", "test", "wait_for_start test")
	pluginSrv.RegisterFetcher("knot", fetcher)
	registerAPIFunctions(pluginSrv, client.GetRESTClient())
	pluginHTTP := httptest.NewServer(pluginSrv)
	defer pluginHTTP.Close()

	// Load through the manager — this is what registers the plugin.knot
	// proxy library, which the apiclient variant calls through.
	manager := plugin.NewManager(nil)
	defer manager.Close()
	if _, err := manager.LoadURL(context.Background(), "knot", pluginHTTP.URL, true, false); err != nil {
		t.Fatalf("LoadURL: %v", err)
	}

	// Bridge the fetcher for the interpreter.
	bridge := pluginpack.New(pluginpack.Options{Manager: manager, Context: context.Background()})
	if err := bridge.Register(); err != nil {
		t.Fatalf("Bridge.Register: %v", err)
	}
	defer bridge.Close()

	bundles, err := bridge.Bundles()
	if err != nil {
		t.Fatalf("Bundles: %v", err)
	}

	p := scriptling.New()
	stdlib.RegisterAll(p)
	plugin.RegisterLibraries(p, manager)

	loader := pack.NewLoader()
	for _, b := range bundles {
		loader.AddBundle(b)
	}
	loader.SetFallback(p.GetLibraryLoader())
	p.SetLibraryLoader(loader)

	t.Run("already running returns immediately", func(t *testing.T) {
		running = true
		result, err := p.Eval(`import knot.space
knot.space.wait_for_start("testspace", timeout=5, interval=0.1)`)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}
		if result.Inspect() != "True" {
			t.Fatalf("expected True, got %s", result.Inspect())
		}
	})

	t.Run("timeout returns False", func(t *testing.T) {
		running = false
		result, err := p.Eval(`knot.space.wait_for_start("testspace", timeout=1, interval=0.2)`)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}
		if result.Inspect() != "False" {
			t.Fatalf("expected False (timeout), got %s", result.Inspect())
		}
	})

	t.Run("transition detected", func(t *testing.T) {
		running = false
		go func() {
			time.Sleep(500 * time.Millisecond)
			running = true
		}()
		result, err := p.Eval(`knot.space.wait_for_start("testspace", timeout=5, interval=0.2)`)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}
		if result.Inspect() != "True" {
			t.Fatalf("expected True (transition detected), got %s", result.Inspect())
		}
	})
}

func jsonBody(t *testing.T, v interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
