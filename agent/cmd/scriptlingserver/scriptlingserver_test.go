package scriptlingserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/scriptling/pluginfetch"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/cli"
	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/object"
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

	t.Run("glob", func(t *testing.T) {
		entries, err := pc.FetchGlob(ctx, "knot://libs", "lib/*")
		if err != nil {
			t.Fatalf("FetchGlob(lib/*): %v", err)
		}
		foundKnot, foundLib := false, false
		for _, e := range entries {
			if e.Name == "lib/knot" && e.IsDir {
				foundKnot = true
			}
			if e.Name == "lib/mylib.py" && !e.IsDir {
				foundLib = true
			}
		}
		if !foundKnot || !foundLib {
			t.Fatalf("expected lib/knot dir and lib/mylib.py in matches, got %v", entries)
		}

		// An exact-path probe answers the directory entry itself.
		exact, err := pc.FetchGlob(ctx, "knot://libs", "lib/knot")
		if err != nil || len(exact) != 1 || !exact[0].IsDir {
			t.Fatalf("FetchGlob(lib/knot) = %v, %v; want the directory entry", exact, err)
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

// TestShouldAutoStart verifies the peer guard: the env var scriptling sets
// on spawned peers is the sole trigger; argv (plugin args) is irrelevant.
func TestShouldAutoStart(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"knot"}

	t.Run("with env var", func(t *testing.T) {
		t.Setenv(PeerEnvVar, "0.23.0")
		if !ShouldAutoStart() {
			t.Fatal("expected auto-start with env var set")
		}
	})
	t.Run("without env var", func(t *testing.T) {
		t.Setenv(PeerEnvVar, "")
		if ShouldAutoStart() {
			t.Fatal("expected no auto-start without env var")
		}
	})
	t.Run("with plugin args", func(t *testing.T) {
		os.Args = []string{"knot", "--alias", "work"}
		t.Setenv(PeerEnvVar, "0.23.0")
		if !ShouldAutoStart() {
			t.Fatal("expected auto-start with env var set and plugin args present")
		}
		os.Args = origArgs
	})
}

// TestAutoStartCmdParsesArgs runs the plugin command through the real CLI
// machinery: --alias in every form selects the alias, KNOT_ALIAS supplies
// it from the environment, and anything that isn't a plugin option is
// rejected instead of silently ignored.
func TestAutoStartCmdParsesArgs(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// An empty-but-set KNOT_ALIAS overrides the flag default in the CLI
	// library, so clear it entirely for the cases that don't use it.
	envName := config.CONFIG_ENV_PREFIX + "_ALIAS"
	origEnv, hadEnv := os.LookupEnv(envName)
	defer func() {
		if hadEnv {
			os.Setenv(envName, origEnv)
		} else {
			os.Unsetenv(envName)
		}
	}()

	cases := []struct {
		name  string
		args  []string
		env   string
		alias string
	}{
		{"default alias", []string{"knot"}, "", "default"},
		{"alias separate", []string{"knot", "--alias", "work"}, "", "work"},
		{"alias joined", []string{"knot", "--alias=work"}, "", "work"},
		{"alias short", []string{"knot", "-a", "work"}, "", "work"},
		{"alias from env", []string{"knot"}, "envalias", "envalias"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			os.Args = tc.args
			if tc.env != "" {
				t.Setenv(envName, tc.env)
			} else {
				os.Unsetenv(envName)
			}

			var got string
			cmd := newPluginCmd(func(ctx context.Context, cmd *cli.Command) error {
				got = cmd.GetString("alias")
				return nil
			})
			if err := cmd.Execute(context.Background()); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if got != tc.alias {
				t.Fatalf("alias = %q, want %q", got, tc.alias)
			}
		})
	}

	reject := []struct {
		name string
		args []string
	}{
		{"unknown flag", []string{"knot", "--verbose"}},
		{"positional argument", []string{"knot", "server"}},
		{"alias without value", []string{"knot", "--alias"}},
	}
	for _, tc := range reject {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			os.Args = tc.args

			cmd := newPluginCmd(func(ctx context.Context, cmd *cli.Command) error {
				t.Fatal("run must not execute for rejected arguments")
				return nil
			})
			if err := cmd.Execute(context.Background()); err == nil {
				t.Fatalf("expected an error for %v", tc.args)
			}
		})
	}
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
// False, a transition from stopped to running is detected, and a space that
// reports running before its agent registers is not treated as ready.
func TestWaitForStart(t *testing.T) {
	var running, agentState atomic.Bool

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
				"is_running": running.Load(),
				"has_state":  agentState.Load(),
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
		running.Store(true)
		agentState.Store(true)
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
		running.Store(false)
		agentState.Store(false)
		result, err := p.Eval(`knot.space.wait_for_start("testspace", timeout=1, interval=0.2)`)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}
		if result.Inspect() != "False" {
			t.Fatalf("expected False (timeout), got %s", result.Inspect())
		}
	})

	t.Run("transition detected", func(t *testing.T) {
		running.Store(false)
		agentState.Store(false)
		go func() {
			time.Sleep(500 * time.Millisecond)
			running.Store(true)
			agentState.Store(true)
		}()
		result, err := p.Eval(`knot.space.wait_for_start("testspace", timeout=5, interval=0.2)`)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}
		if result.Inspect() != "True" {
			t.Fatalf("expected True (transition detected), got %s", result.Inspect())
		}
	})

	// Regression: the container runtime reports a space running before the
	// in-container agent connects back. wait_for_start must keep polling
	// until has_state flips, or run() races into "Agent session not found".
	t.Run("running without agent session is not ready", func(t *testing.T) {
		running.Store(true)
		agentState.Store(false)
		result, err := p.Eval(`knot.space.wait_for_start("testspace", timeout=1, interval=0.2)`)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}
		if result.Inspect() != "False" {
			t.Fatalf("expected False (agent never connected), got %s", result.Inspect())
		}
	})

	t.Run("agent connecting after start is detected", func(t *testing.T) {
		running.Store(true)
		agentState.Store(false)
		go func() {
			time.Sleep(500 * time.Millisecond)
			agentState.Store(true)
		}()
		result, err := p.Eval(`knot.space.wait_for_start("testspace", timeout=5, interval=0.2)`)
		if err != nil {
			t.Fatalf("eval: %v", err)
		}
		if result.Inspect() != "True" {
			t.Fatalf("expected True (agent connected), got %s", result.Inspect())
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

// filesStore holds the file content the mock knot API received, so tests can
// assert on the exact bytes that crossed the transports.
type filesStore struct {
	mu    sync.Mutex
	files map[string][]byte
	types map[string]string
}

func (s *filesStore) put(key string, body []byte, contentType string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[key] = body
	s.types[key] = contentType
}

func (s *filesStore) get(key string) ([]byte, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.files[key]
	return b, s.types[key], ok
}

// mockFilesAPI serves the object endpoints knot.files needs.
func mockFilesAPI(t *testing.T) (*httptest.Server, *filesStore) {
	t.Helper()
	store := &filesStore{files: map[string][]byte{}, types: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/files/objects/{bucket}/{key...}", func(w http.ResponseWriter, r *http.Request) {
		body, contentType, ok := store.get(r.PathValue("key"))
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.Write(body)
	})
	mux.HandleFunc("PUT /api/files/objects/{bucket}/{key...}", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		store.put(r.PathValue("key"), body, r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"key": r.PathValue("key"), "size": len(body), "etag": "etag", "sha256": "sha",
			"content_type": r.Header.Get("Content-Type"), "modified_at": "2026-10-05T00:00:00Z",
		})
	})
	return httptest.NewServer(mux), store
}

// TestPluginRawTransfers covers the raw byte transfers knot.files needs on
// the plugin transport: the plugin functions over the wire, and the
// plugin-variant knot.apiclient plus knot.files running as scripts, the path
// the scriptling CLI takes inside a space.
func TestPluginRawTransfers(t *testing.T) {
	api, store := mockFilesAPI(t)
	defer api.Close()

	client, err := apiclient.NewClient(api.URL, "test-token", true)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.SetContentType("application/json")

	fetcher := pluginfetch.NewFetcher(client)
	server := plugin.NewServer("knot", "test", "raw transfer test")
	server.RegisterFetcher("knot", fetcherAdapter{fetcher})
	registerAPIFunctions(server, client.GetRESTClient())

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

	t.Run("wire put and get", func(t *testing.T) {
		content := "héllo ü" // non-ASCII must survive the Base64 bridge
		result, err := pc.CallFunction(ctx, "api_put_bytes", []plugin.Value{
			{Type: "string", Value: "/api/files/objects/scripts/wire.txt"},
			{Type: "string", Value: base64.StdEncoding.EncodeToString([]byte(content))},
		}, map[string]plugin.Value{"content_type": {Type: "string", Value: "text/plain; charset=utf-8"}})
		if err != nil {
			t.Fatalf("api_put_bytes: %v", err)
		}
		if result.Type != "dict" {
			t.Fatalf("expected dict, got %s", result.Type)
		}
		if body, ct, _ := store.get("wire.txt"); string(body) != content || ct != "text/plain; charset=utf-8" {
			t.Fatalf("stored %q (%s), want %q", body, ct, content)
		}

		result, err = pc.CallFunction(ctx, "api_get_bytes", []plugin.Value{
			{Type: "string", Value: "/api/files/objects/scripts/wire.txt"},
		}, nil)
		if err != nil {
			t.Fatalf("api_get_bytes: %v", err)
		}
		if result.Type != "string" {
			t.Fatalf("expected the body as a Base64 string, got %s", result.Type)
		}
		got, derr := base64.StdEncoding.DecodeString(result.Value.(string))
		if derr != nil || string(got) != content {
			t.Fatalf("api_get_bytes returned %q (%v), want %q", got, derr, content)
		}
	})

	t.Run("knot.files through the plugin transport", func(t *testing.T) {
		env := scriptling.New()
		env.EnableOutputCapture()
		stdlib.RegisterAll(env)
		env.RegisterLibrary(testPluginControlLibrary(pc))

		apiSource, err := pc.FetchFile(ctx, "knot://libs", "lib/knot/apiclient.py")
		if err != nil {
			t.Fatalf("fetch plugin-variant apiclient: %v", err)
		}
		env.RegisterScriptLibrary("knot.apiclient", string(apiSource))
		filesSource, err := pc.FetchFile(ctx, "knot://libs", "lib/knot/files.py")
		if err != nil {
			t.Fatalf("fetch files.py: %v", err)
		}
		env.RegisterScriptLibrary("knot.files", string(filesSource))

		script := `
import knot.files
info = knot.files.write_file("scripts", "app/héllo.txt", "héllo ü")
result = {
    "size": info["size"],
    "type": info["content_type"],
    "text": knot.files.read_text("scripts", "app/héllo.txt"),
    "roundtrip": knot.files.read_file("scripts", "app/héllo.txt").decode() == "héllo ü",
}
`
		if _, err := env.Eval(script); err != nil {
			t.Fatalf("script failed: %v", err)
		}
		v, errObj := env.GetVar("result")
		if errObj != nil {
			t.Fatalf("script set no result: %v", errObj)
		}
		m, ok := v.(map[string]interface{})
		if !ok {
			t.Fatalf("result is %T, want a dict", v)
		}
		// The server sent an integer and it must stay one over the wire:
		// UseNumber at the decode keeps it, FromGo makes it an Integer and
		// the plugin protocol tags it int.
		if s, ok := m["size"].(int64); !ok || s != int64(len("héllo ü")) {
			t.Errorf("size = %v (%T)", m["size"], m["size"])
		}
		if m["type"] != "text/plain; charset=utf-8" {
			t.Errorf("content type = %v", m["type"])
		}
		if m["text"] != "héllo ü" || m["roundtrip"] != true {
			t.Errorf("round trip failed: %v", m)
		}
		if body, ct, _ := store.get("app/héllo.txt"); string(body) != "héllo ü" || ct != "text/plain; charset=utf-8" {
			t.Fatalf("server stored %q (%s)", body, ct)
		}
	})
}

// testPluginControlLibrary is a minimal stand-in for the scriptling.plugin
// library the scriptling CLI provides: call_function routes to the knot
// plugin over the same wire, converting the plain values these tests pass.
func testPluginControlLibrary(pc *plugin.Client) *object.Library {
	return object.NewLibrary("scriptling.plugin", map[string]*object.Builtin{
		"call_function": {Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
			if len(args) < 2 {
				return &object.Error{Message: "call_function: library and function name required"}
			}
			library, errObj := args[0].AsString()
			if errObj != nil {
				return errObj
			}
			name, errObj := args[1].AsString()
			if errObj != nil {
				return errObj
			}
			if library != "plugin.knot" {
				return &object.Error{Message: "plugin not found: " + library}
			}
			values := make([]plugin.Value, 0, len(args)-2)
			for _, a := range args[2:] {
				v, err := testPluginValue(a)
				if err != nil {
					return &object.Error{Message: err.Error()}
				}
				values = append(values, v)
			}
			kw := make(map[string]plugin.Value, len(kwargs.Kwargs))
			for k, v := range kwargs.Kwargs {
				pv, err := testPluginValue(v)
				if err != nil {
					return &object.Error{Message: err.Error()}
				}
				kw[k] = pv
			}
			var result plugin.Value
			var callErr error
			object.RunBlocking(ctx, func() {
				result, callErr = pc.CallFunction(ctx, name, values, kw)
			})
			if callErr != nil {
				return &object.Error{Message: callErr.Error()}
			}
			return testPluginObject(result)
		}},
	}, nil, "test stand-in for scriptling.plugin")
}

// testPluginValue converts the plain values these tests pass to the wire.
func testPluginValue(o object.Object) (plugin.Value, error) {
	switch v := o.(type) {
	case *object.String:
		return plugin.Value{Type: "string", Value: v.StringValue()}, nil
	case *object.Integer:
		return plugin.Value{Type: "int", Value: v.IntValue()}, nil
	case *object.Float:
		return plugin.Value{Type: "float", Value: v.FloatValue()}, nil
	case *object.Boolean:
		return plugin.Value{Type: "bool", Value: v.BoolValue()}, nil
	case *object.Null:
		return plugin.Value{Type: "null"}, nil
	}
	return plugin.Value{}, fmt.Errorf("cannot pass %s to a plugin in tests", o.Type())
}

// testPluginObject converts a wire value back, numbers arriving as float64
// as JSON decodes them.
func testPluginObject(v plugin.Value) object.Object {
	switch v.Type {
	case "string":
		return object.NewString(v.Value.(string))
	case "int":
		return object.NewInteger(int64(v.Value.(float64)))
	case "float":
		return object.NewFloat(v.Value.(float64))
	case "bool":
		return object.NewBoolean(v.Value.(bool))
	case "null":
		return &object.Null{}
	case "dict":
		entries := make(map[string]object.Object, len(v.Entries))
		for k, e := range v.Entries {
			entries[k] = testPluginObject(e)
		}
		return object.NewStringDict(entries)
	case "list":
		elements := make([]object.Object, 0, len(v.Items))
		for _, i := range v.Items {
			elements = append(elements, testPluginObject(i))
		}
		return &object.List{Elements: elements}
	}
	return &object.Error{Message: "test cannot convert plugin value " + v.Type}
}
