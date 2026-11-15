package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/toolsurface"
	"github.com/paularlott/knot/internal/util/rest"
)

// securityAPI is a fake knot API for the sandbox tests: it records what
// reached it.
type securityAPI struct {
	mux      *http.ServeMux
	hits     atomic.Int32
	surfaces []toolsurface.Surface
	bodies   []string
}

func newSecurityAPI(t *testing.T) (*securityAPI, rest.RESTClient) {
	t.Helper()
	api := &securityAPI{mux: http.NewServeMux()}
	write := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	api.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		api.hits.Add(1)
		api.surfaces = append(api.surfaces, toolsurface.From(r.Context()))
		body, _ := io.ReadAll(r.Body)
		api.bodies = append(api.bodies, string(body))
		write(w, map[string]interface{}{"ok": true})
	})
	api.mux.HandleFunc("GET /api/event-sinks/{id}", func(w http.ResponseWriter, r *http.Request) {
		api.hits.Add(1)
		write(w, map[string]interface{}{
			"event_sink_id": "e1", "name": "hook", "sink_type": "webhook",
			"webhook": map[string]interface{}{
				"url":     "https://user:pass@hooks.example.com/in?key=abc123&team=x",
				"secret":  "whsec",
				"headers": map[string]interface{}{"Authorization": "Bearer xyz", "X-Api-Key": "k"},
			},
			"icon_url": "https://ftp:pw@example.com/i.png",
		})
	})
	api.mux.HandleFunc("GET /api/big", func(w http.ResponseWriter, r *http.Request) {
		api.hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":"` + strings.Repeat("x", 4<<20) + `"}`))
	})
	api.mux.HandleFunc("GET /api/files/objects/{bucket}/{key...}", func(w http.ResponseWriter, r *http.Request) {
		api.hits.Add(1)
		w.Header().Set("Content-Type", "application/octet-stream")
		chunk := make([]byte, 64<<10)
		for i := 0; i < 64; i++ { // 4 MB, in chunks as a file is streamed
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	})
	api.mux.HandleFunc("POST /api/spaces/{id}/run-command", func(w http.ResponseWriter, r *http.Request) {
		api.hits.Add(1)
		// Like the real handler: wait on the agent, but no longer than the
		// request.
		select {
		case <-r.Context().Done():
			w.WriteHeader(http.StatusGatewayTimeout)
		case <-time.After(30 * time.Second):
			write(w, map[string]interface{}{"success": true})
		}
	})
	rest.SetAPIMux(api.mux)
	t.Cleanup(func() { rest.SetAPIMux(http.NewServeMux()) })
	return api, rest.NewMuxClient(&model.User{Id: "u1"})
}

func TestAIScriptPolicyDeniesStoredAndForeignCode(t *testing.T) {
	p := &AIScriptPolicy{Mode: AIScriptModeWrite}
	for _, d := range []struct{ method, path string }{
		{"POST", "/api/scripts"},
		{"PUT", "/api/scripts/s1"},
		{"DELETE", "/api/scripts/s1"},
		{"PUT", "/api/spaces/s1/jobs"},
		{"GET", "/api/plugins/field-handlers/h1"},
		{"GET", "/api/plugins/field-handlers/h1?space=x"},
	} {
		err := p.Check(d.method, d.path)
		if err == nil || !strings.Contains(err.Error(), "in either mode") {
			t.Errorf("%s %s: got %v, want a deny list refusal", d.method, d.path, err)
		}
	}
	for _, a := range []struct{ method, path string }{
		{"GET", "/api/scripts"},
		{"GET", "/api/scripts/s1"},
		{"GET", "/api/spaces/s1/jobs"},
		{"GET", "/api/plugins/field-handlers"},
	} {
		if err := p.Check(a.method, a.path); err != nil {
			t.Errorf("%s %s refused: %v", a.method, a.path, err)
		}
	}
	// Read mode refuses the field handlers too: they run plugin code.
	if err := (&AIScriptPolicy{Mode: AIScriptModeRead}).Check("GET", "/api/plugins/field-handlers/h1"); err == nil {
		t.Error("read mode reached a plugin field handler")
	}
}

func TestAIScriptPolicyBodyChecks(t *testing.T) {
	p := &AIScriptPolicy{Mode: AIScriptModeWrite}
	refused := []struct {
		method, path string
		body         interface{}
	}{
		{"POST", "/api/spaces/s1/execute-script", map[string]interface{}{"content": "print(1)"}},
		{"POST", "/api/spaces/s1/execute-script", struct {
			Content string `json:"content"`
		}{"import knot.token"}},
		{"POST", "/api/event-sinks", map[string]interface{}{"name": "x", "sink_type": "script", "script_id": "s1"}},
		{"POST", "/api/event-sinks", map[string]interface{}{"name": "x", "sink_type": " Script "}},
		{"PUT", "/api/event-sinks/e1", map[string]interface{}{"name": "x", "sink_type": "webhook", "script_id": "s1"}},
		{"PUT", "/api/event-sinks/e1", map[string]interface{}{"webhook": map[string]interface{}{"headers": map[string]interface{}{"Authorization": redactedValue}}}},
		{"PUT", "/api/templates/t1", map[string]interface{}{"description": "a " + redactedValue + " b"}},
		{"PUT", "/api/event-sinks/e1", map[string]interface{}{"webhook": map[string]interface{}{"url": "https://%5Bredacted%5D@h.example.com/"}}},
	}
	for _, r := range refused {
		if err := p.CheckRequest(r.method, r.path, r.body); err == nil {
			t.Errorf("%s %s %v allowed", r.method, r.path, r.body)
		}
	}
	allowed := []struct {
		method, path string
		body         interface{}
	}{
		{"POST", "/api/spaces/s1/execute-script", map[string]interface{}{"script_name": "backup", "arguments": []interface{}{"a"}}},
		{"POST", "/api/spaces/s1/execute-script", map[string]interface{}{"script_id": "s1", "content": ""}},
		{"POST", "/api/event-sinks", map[string]interface{}{"name": "x", "sink_type": "webhook", "script_id": ""}},
		{"PUT", "/api/event-sinks/e1", map[string]interface{}{"name": "x", "sink_type": "json-rpc"}},
		{"POST", "/api/spaces/s1/stop", nil},
	}
	for _, a := range allowed {
		if err := p.CheckRequest(a.method, a.path, a.body); err != nil {
			t.Errorf("%s %s %v refused: %v", a.method, a.path, a.body, err)
		}
	}

	// A redacted webhook secret is sent back empty, which the server takes
	// as "keep the stored one".
	body := map[string]interface{}{"name": "x", "sink_type": "webhook", "webhook": map[string]interface{}{"url": "https://h.example.com/", "secret": redactedValue}}
	if err := p.CheckRequest("PUT", "/api/event-sinks/e1", body); err != nil {
		t.Fatalf("sink update with a redacted secret refused: %v", err)
	}
	if got := body["webhook"].(map[string]interface{})["secret"]; got != "" {
		t.Errorf("redacted secret sent as %q, want empty", got)
	}
}

func TestAIScriptPolicyCountsMethodBatches(t *testing.T) {
	p := &AIScriptPolicy{Mode: AIScriptModeWrite, MaxCalls: 10}
	batch := make([]interface{}, 8)
	for i := range batch {
		batch[i] = map[string]interface{}{"jsonrpc": "2.0", "method": "m", "id": i}
	}
	if err := p.CheckRequest("POST", "/api/methods/call", batch); err != nil {
		t.Fatal(err)
	}
	if p.Calls() != 8 {
		t.Errorf("calls = %d, want 8", p.Calls())
	}
	err := p.CheckRequest("POST", "/api/methods/call", batch)
	if err == nil || !strings.Contains(err.Error(), "at most 10 API calls") {
		t.Errorf("second batch: %v", err)
	}
	// A single call counts once, and method calls need write mode.
	single := &AIScriptPolicy{Mode: AIScriptModeWrite}
	_ = single.CheckRequest("POST", "/api/methods/call", map[string]interface{}{"method": "m"})
	if single.Calls() != 1 {
		t.Errorf("single call counted %d", single.Calls())
	}
	if err := (&AIScriptPolicy{Mode: AIScriptModeRead}).CheckRequest("POST", "/api/methods/call", batch); err == nil {
		t.Error("read mode called methods")
	}
}

func TestAIScriptPolicyRefusesAmbiguousPaths(t *testing.T) {
	p := &AIScriptPolicy{Mode: AIScriptModeWrite}
	for _, path := range []string{
		"/api/spaces#/../tokens",
		"/api/tokens#x",
		"/api/spaces/s1#",
		"/api/spaces%23/x",
		"/api/spaces;x=1",
		"/api/spaces%3Bx",
		"/api/spaces\\tokens",
		"/api/spaces%5Ctokens",
		"/api/x%252Ftokens",
		"/api/x%2ftokens",
		"/api/spaces%00",
		"/api/spaces\x00",
	} {
		if err := p.Check("GET", path); err == nil {
			t.Errorf("GET %q allowed", path)
		}
	}
	if err := p.Check("GET", "/api/spaces?user_id=u1&q=a%20b"); err != nil {
		t.Errorf("a plain query string was refused: %v", err)
	}
}

func TestRedactSecretsHeadersAndURLs(t *testing.T) {
	var v interface{}
	_ = json.Unmarshal([]byte(`{"webhook":{"url":"https://u:p@hooks.example.com/x?key=abc&team=t","headers":{"Authorization":"Bearer x","X-Api-Key":"k"},"secret":"s"},
		"icon_url":"https://a:b@cdn.example.com/i.png?v=2","url":"https://example.com/plain?page=2","name":"n"}`), &v)
	redactSecrets(&v)
	m := v.(map[string]interface{})
	hook := m["webhook"].(map[string]interface{})
	headers := hook["headers"].(map[string]interface{})
	if len(headers) != 2 || headers["Authorization"] != redactedValue || headers["X-Api-Key"] != redactedValue {
		t.Errorf("headers = %v", headers)
	}
	u := hook["url"].(string)
	if strings.Contains(u, "u:p") || strings.Contains(u, "abc") || !strings.Contains(u, "hooks.example.com/x") || !strings.Contains(u, "key=") {
		t.Errorf("webhook url = %q", u)
	}
	if hook["secret"] != redactedValue {
		t.Errorf("secret = %v", hook["secret"])
	}
	if icon := m["icon_url"].(string); strings.Contains(icon, "a:b") || !strings.Contains(icon, "v=2") {
		t.Errorf("icon_url = %q: want the user info gone and the query kept", icon)
	}
	if m["url"] != "https://example.com/plain?page=2" || m["name"] != "n" {
		t.Errorf("plain fields changed: %v", m)
	}
}

func TestRunAIScriptRedactsEventSinks(t *testing.T) {
	_, client := newSecurityAPI(t)
	run := runTestScript(t, client, AIScriptModeRead, `result = apiclient.get("/api/event-sinks/e1")`)
	if !run.OK {
		t.Fatal(run.Error)
	}
	got, _ := json.Marshal(run.Result)
	for _, leak := range []string{"Bearer xyz", "abc123", "pass@", "whsec", "pw@"} {
		if strings.Contains(string(got), leak) {
			t.Errorf("result leaks %q: %s", leak, got)
		}
	}
	if !strings.Contains(string(got), "Authorization") {
		t.Errorf("header names should stay: %s", got)
	}
}

func TestRunAIScriptMemoryLimits(t *testing.T) {
	_, client := newSecurityAPI(t)
	// One huge value: the value cap.
	run := runTestScript(t, client, AIScriptModeRead, "x = \"a\" * 400_000_000\ny = x + \"b\"\n")
	if run.OK || !strings.Contains(strings.ToLower(run.Error), "limit") {
		t.Errorf("huge string: ok=%v error=%q", run.OK, run.Error)
	}
	// Many values under the cap: the memory limit.
	old := AIScriptMemoryLimit
	AIScriptMemoryLimit = 32 << 20
	defer func() { AIScriptMemoryLimit = old }()
	run = runTestScript(t, client, AIScriptModeRead, `
keep = []
for i in range(200):
    keep.append(("%d" % i) * 500_000)
`)
	if run.OK || !strings.Contains(run.Error, "memory") {
		t.Errorf("many strings: ok=%v error=%q", run.OK, run.Error)
	}
}

func TestRunAIScriptResponseCap(t *testing.T) {
	_, client := newSecurityAPI(t)
	old := AIScriptMaxResponseBytes
	AIScriptMaxResponseBytes = 1 << 20
	defer func() { AIScriptMaxResponseBytes = old }()

	run := runTestScript(t, client, AIScriptModeRead, `apiclient.get("/api/big")`)
	if run.OK || !strings.Contains(run.Error, "too large") {
		t.Errorf("large JSON: ok=%v error=%q", run.OK, run.Error)
	}
	run = runTestScript(t, client, AIScriptModeRead, `apiclient.get_bytes("/api/files/objects/b/big.bin")`)
	if run.OK || !strings.Contains(run.Error, "too large") {
		t.Errorf("large file: ok=%v error=%q", run.OK, run.Error)
	}
	AIScriptMaxResponseBytes = 8 << 20
	run = runTestScript(t, client, AIScriptModeRead, `result = len(apiclient.get_bytes("/api/files/objects/b/big.bin"))`)
	if !run.OK || run.Result != int64(4<<20) {
		t.Errorf("file under the cap: ok=%v result=%v error=%q", run.OK, run.Result, run.Error)
	}
}

func TestRunAIScriptErrorIsCapped(t *testing.T) {
	_, client := newSecurityAPI(t)
	run := runTestScript(t, client, AIScriptModeRead, `raise Exception("e" * 5_000_000)`)
	if run.OK || len(run.Error) > AIScriptMaxOutput+100 || !run.Truncated || !strings.Contains(run.Error, "truncated") {
		t.Errorf("error is %d bytes (truncated=%v), want it capped at %d", len(run.Error), run.Truncated, AIScriptMaxOutput)
	}
}

func TestRunAIScriptResultSizeCheckedCheaply(t *testing.T) {
	_, client := newSecurityAPI(t)
	// A list that holds itself must not hang or blow the stack.
	run := runTestScript(t, client, AIScriptModeRead, "x = []\nx.append(x)\nresult = x\n")
	if run.OK || !strings.Contains(run.Error, "byte limit") {
		t.Errorf("self-referencing result: ok=%v error=%q", run.OK, run.Error)
	}
	run = runTestScript(t, client, AIScriptModeRead, `result = {"rows": ["y" * 1000] * 2000}`)
	if run.OK || !run.Truncated || !strings.Contains(run.Error, "byte limit") {
		t.Errorf("large result: ok=%v error=%q", run.OK, run.Error)
	}
	run = runTestScript(t, client, AIScriptModeRead, `result = {"rows": ["y" * 10] * 20}`)
	if !run.OK {
		t.Errorf("small result refused: %q", run.Error)
	}
}

// The run's context reaches the in-process API: its time limit ends a
// handler waiting on a space, and no tool-call surface goes with it.
func TestRunAIScriptContextReachesAPI(t *testing.T) {
	api, client := newSecurityAPI(t)
	old := AIScriptTimeout
	AIScriptTimeout = 300 * time.Millisecond
	defer func() { AIScriptTimeout = old }()

	user := &model.User{Id: "u1", Username: "kai", Roles: []string{model.RoleAdminUUID, aiScriptsRole(t)}}
	ctx := toolsurface.With(context.Background(), toolsurface.WebChat)
	started := time.Now()
	run := runAIScript(ctx, user, client, AIScriptRequest{Code: `
apiclient.get("/api/spaces")
apiclient.post("/api/spaces/s1/run-command", {"command": "sleep 60"})
`, Purpose: "test", Mode: AIScriptModeWrite})
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("the run took %s: the time limit did not reach the handler", elapsed)
	}
	if run.OK || !strings.Contains(run.Error, "time limit") {
		t.Errorf("got ok=%v error=%q", run.OK, run.Error)
	}
	for i, s := range api.surfaces {
		if s != "" {
			t.Errorf("API call %d saw surface %q", i, s)
		}
	}
	if len(api.surfaces) == 0 {
		t.Error("no API call reached the fake")
	}
}

func TestRunAIScriptRefusesInlineSpaceCode(t *testing.T) {
	api, client := newSecurityAPI(t)
	run := runTestScript(t, client, AIScriptModeWrite, `apiclient.post("/api/spaces/s1/execute-script", {"content": "print('hi')"})`)
	if run.OK || !strings.Contains(run.Error, "their own code") {
		t.Errorf("inline code: ok=%v error=%q", run.OK, run.Error)
	}
	if api.hits.Load() != 0 {
		t.Error("the refused call reached the API")
	}
	run = runTestScript(t, client, AIScriptModeWrite, `apiclient.post("/api/spaces/s1/execute-script", {"script_name": "backup"})`)
	if !run.OK {
		t.Errorf("named script refused: %q", run.Error)
	}
}
