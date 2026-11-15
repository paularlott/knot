package service

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/rest"
)

// sinkAPI is a fake knot API holding one webhook event sink, as the real
// handlers serve it: GET masks nothing but the secret (the AI script client
// redacts the rest), PUT records the body it was sent.
type sinkAPI struct {
	mu   sync.Mutex
	puts []map[string]interface{}
	hits int
}

const sinkID = "0b0d7d5e-1c2a-4f6e-9a77-2a3f6c1b9d10"

func newSinkAPI(t *testing.T, sink map[string]interface{}) (*sinkAPI, rest.RESTClient) {
	t.Helper()
	api := &sinkAPI{}
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		api.mu.Lock()
		api.hits++
		api.mu.Unlock()
		write(w, map[string]interface{}{"ok": true, "path": r.URL.Path, "query": r.URL.Query()})
	})
	mux.HandleFunc("GET /api/event-sinks/{id}", func(w http.ResponseWriter, r *http.Request) {
		write(w, sink)
	})
	mux.HandleFunc("PUT /api/event-sinks/{id}", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		api.mu.Lock()
		api.puts = append(api.puts, body)
		api.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /api/templates/{id}", func(w http.ResponseWriter, r *http.Request) {
		write(w, map[string]interface{}{"template_id": "t1", "name": "go", "env": map[string]interface{}{"token": "tok-123", "REGION": "eu"}})
	})
	mux.HandleFunc("PUT /api/templates/{id}", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		api.mu.Lock()
		api.puts = append(api.puts, body)
		api.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
	rest.SetAPIMux(mux)
	t.Cleanup(func() { rest.SetAPIMux(http.NewServeMux()) })
	return api, rest.NewMuxClient(&model.User{Id: "u1"})
}

func slackSink() map[string]interface{} {
	return map[string]interface{}{
		"event_sink_id": sinkID, "name": "hook", "sink_type": "webhook", "active": true,
		"events": []interface{}{"space.*"},
		"webhook": map[string]interface{}{
			"url":     "https://hooks.slack.com/services/T000/B000/XXXXSECRETXXXX?token=q1",
			"secret":  "whse****",
			"headers": map[string]interface{}{"Authorization": "Bearer xyz", "X-Api-Key": "k"},
		},
	}
}

// Item 7: event_sink.update(id, active=False) works on a webhook with
// headers and a key in its URL, and keeps them: the redacted values the
// script read are restored from the stored sink.
func TestRunAIScriptEventSinkUpdateKeepsRedactedValues(t *testing.T) {
	api, client := newSinkAPI(t, slackSink())
	run := runTestScript(t, client, AIScriptModeWrite, `result = event_sink.update("`+sinkID+`", active=False)`)
	if !run.OK {
		t.Fatalf("update refused: %s", run.Error)
	}
	if len(api.puts) != 1 {
		t.Fatalf("PUTs = %d", len(api.puts))
	}
	body := api.puts[0]
	hook := body["webhook"].(map[string]interface{})
	headers := hook["headers"].(map[string]interface{})
	if body["active"] != false || headers["Authorization"] != "Bearer xyz" || headers["X-Api-Key"] != "k" {
		t.Errorf("PUT body = %v", body)
	}
	if hook["url"] != "https://hooks.slack.com/services/T000/B000/XXXXSECRETXXXX?token=q1" {
		t.Errorf("url written back as %v", hook["url"])
	}
	if s := hook["secret"]; s != "" && s != "whse****" {
		t.Errorf("secret written back as %v", s)
	}
}

// Item 7: restoring is refused when the URL changes (the stored headers
// would go to the new address), or when a placeholder lands anywhere other
// than where it was read.
func TestRunAIScriptRedactedWriteBackRefusals(t *testing.T) {
	api, client := newSinkAPI(t, slackSink())
	for name, code := range map[string]string{
		"new url, kept headers": `event_sink.update("` + sinkID + `", url="https://evil.example.com/x")`,
		"placeholder moved": `
s = apiclient.get("/api/event-sinks/` + sinkID + `")
s["description"] = s["webhook"]["headers"]["Authorization"]
apiclient.put("/api/event-sinks/` + sinkID + `", s)`,
		"header swapped": `
s = apiclient.get("/api/event-sinks/` + sinkID + `")
h = s["webhook"]["headers"]
h["X-Other"] = h["Authorization"]
apiclient.put("/api/event-sinks/` + sinkID + `", s)`,
		"post": `apiclient.post("/api/event-sinks", {"name": "x", "webhook": {"headers": {"A": "[redacted]"}}})`,
	} {
		run := runTestScript(t, client, AIScriptModeWrite, code)
		if run.OK {
			t.Errorf("%s: allowed", name)
		}
	}
	if len(api.puts) != 0 {
		t.Errorf("a refused write reached the API: %v", api.puts)
	}
	// A new URL with new headers in full is an ordinary update.
	run := runTestScript(t, client, AIScriptModeWrite, `event_sink.update("`+sinkID+`", url="https://new.example.com/x", headers={"Authorization": "Bearer new"})`)
	if !run.OK || len(api.puts) != 1 {
		t.Fatalf("full update: ok=%v error=%q", run.OK, run.Error)
	}
}

// Item 7: generic read-modify-write of an object with redacted fields, such
// as a template whose env holds a variable named token.
func TestRunAIScriptGenericWriteBack(t *testing.T) {
	api, client := newSinkAPI(t, slackSink())
	run := runTestScript(t, client, AIScriptModeWrite, `
t = apiclient.get("/api/templates/t1")
t["name"] = "go2"
apiclient.put("/api/templates/t1", t)
result = t["env"]["token"]`)
	if !run.OK || run.Result != redactedValue {
		t.Fatalf("ok=%v result=%v error=%q", run.OK, run.Result, run.Error)
	}
	env := api.puts[0]["env"].(map[string]interface{})
	if env["token"] != "tok-123" || api.puts[0]["name"] != "go2" {
		t.Errorf("PUT body = %v", api.puts[0])
	}
}

// Item 8: a webhook URL's path after the first segment is redacted (Slack,
// Discord); the scheme and host stay.
func TestRedactWebhookURLPath(t *testing.T) {
	for in, want := range map[string]string{
		"https://hooks.slack.com/services/T000/B000/XXXX":                "https://hooks.slack.com/services/%5Bredacted%5D",
		"https://discord.com/api/webhooks/123/tok-abc":                   "https://discord.com/api/%5Bredacted%5D",
		"https://hooks.example.com/in":                                   "https://hooks.example.com/in",
		"https://hooks.example.com/":                                     "https://hooks.example.com/",
		"https://hooks.example.com/services/%5Bredacted%5D":              "https://hooks.example.com/services/%5Bredacted%5D",
		"https://u:p@hooks.example.com/a/b?key=k":                        "https://%5Bredacted%5D@hooks.example.com/a/%5Bredacted%5D?key=%5Bredacted%5D",
		"https://outlook.office.com/webhook/abc@def/IncomingWebhook/x/y": "https://outlook.office.com/webhook/%5Bredacted%5D",
	} {
		if got := redactURL(in, true); got != want {
			t.Errorf("redactURL(%q) = %q, want %q", in, got, want)
		}
		// Redacting is stable: a value written back matches.
		if got := redactURL(redactURL(in, true), true); got != redactURL(in, true) {
			t.Errorf("redactURL is not stable for %q: %q", in, got)
		}
	}
	// Other URLs keep their path.
	if got := redactURL("https://example.com/a/b/c", false); got != "https://example.com/a/b/c" {
		t.Errorf("plain url = %q", got)
	}
	var v interface{}
	_ = json.Unmarshal([]byte(`{"webhook":{"url":"https://discord.com/api/webhooks/123/tok-abc"}}`), &v)
	redactSecrets(&v)
	if got, _ := json.Marshal(v); strings.Contains(string(got), "tok-abc") || strings.Contains(string(got), "123") {
		t.Errorf("redacted sink leaks: %s", got)
	}
}

// Item 2: the body checks match keys whatever their case, as encoding/json
// does, and refuse keys that differ only in case.
func TestAIScriptBodyChecksIgnoreKeyCase(t *testing.T) {
	p := &AIScriptPolicy{Mode: AIScriptModeWrite}
	for _, r := range []struct {
		path string
		body map[string]interface{}
	}{
		{"/api/spaces/s1/execute-script", map[string]interface{}{"Content": "print(1)"}},
		{"/api/spaces/s1/execute-script", map[string]interface{}{"CONTENT": "print(1)", "script_name": "x"}},
		{"/api/event-sinks", map[string]interface{}{"name": "x", "Sink_Type": "script"}},
		{"/api/event-sinks", map[string]interface{}{"name": "x", "sink_type": "webhook", "Script_ID": "s1"}},
		{"/api/spaces/s1/execute-script", map[string]interface{}{"content": "", "Content": "print(1)"}},
		{"/api/templates/t1", map[string]interface{}{"spec": map[string]interface{}{"Name": "a", "name": "b"}}},
		{"/api/event-sinks/e1", map[string]interface{}{"webhook": map[string]interface{}{"url": "https://h.example.com/"}, "Webhook": nil}},
	} {
		method := "POST"
		if strings.HasPrefix(r.path, "/api/templates") || r.path == "/api/event-sinks/e1" {
			method = "PUT"
		}
		if err := p.CheckRequest(method, r.path, r.body); err == nil {
			t.Errorf("%s %v allowed", r.path, r.body)
		}
	}
	// Environment variables are case sensitive and may differ only in case.
	if err := p.CheckRequest("PUT", "/api/templates/t1", map[string]interface{}{"env": map[string]interface{}{"Path": "a", "PATH": "b"}}); err != nil {
		t.Errorf("env with case-differing names refused: %v", err)
	}
	// The redacted secret of a sink sent back under another case is still
	// sent empty.
	body := map[string]interface{}{"name": "x", "Webhook": map[string]interface{}{"url": "https://h.example.com/", "Secret": redactedValue}}
	if err := p.CheckRequest("PUT", "/api/event-sinks/e1", body); err != nil {
		t.Fatalf("sink with redacted Secret refused: %v", err)
	}
	if got := body["Webhook"].(map[string]interface{})["Secret"]; got != "" {
		t.Errorf("Secret sent as %q", got)
	}
}

// Item 3: the encoding rules apply to the path, not the query string, so
// knot.files.list_files (delimiter=%2F) and exists() with "/" in keys work.
func TestAIScriptPathRulesSpareTheQuery(t *testing.T) {
	p := &AIScriptPolicy{Mode: AIScriptModeRead}
	for _, path := range []string{
		"/api/files/list/b?limit=1000&delimiter=%2F",
		"/api/files/list/b?limit=1000&prefix=docs%2Freports%2F&after=docs%2Fa.md",
		"/api/files/list/b?limit=1&prefix=docs%2Fsecret-plan.md",
		"/api/spaces?q=a%2Fb%25c%3Bd%5Ce%23f",
		"/api/files/stat/b/docs/a%20b.md",
	} {
		if err := p.Check("GET", path); err != nil {
			t.Errorf("GET %q refused: %v", path, err)
		}
	}
	for _, path := range []string{
		"/api/x%2Ftokens?a=1",
		"/api/x%25/y?a=1",
		"/api/spaces;x?a=1",
		"/api/spaces?a=1#frag",
		"/api/spaces?a=#",
		"/api/spaces?a=\x00",
	} {
		if err := p.Check("GET", path); err == nil {
			t.Errorf("GET %q allowed", path)
		}
	}

	api, client := newSinkAPI(t, slackSink())
	run := runTestScript(t, client, AIScriptModeRead, `
r = apiclient.get("/api/files/list/b", {"q": "a/b", "delimiter": "/"})
result = r["query"]`)
	if !run.OK {
		t.Fatalf("get with a slash in params refused: %s", run.Error)
	}
	if got, _ := json.Marshal(run.Result); !strings.Contains(string(got), `"a/b"`) {
		t.Errorf("query reached the handler as %s", got)
	}
	_ = api
}

// Item 11: only the secret routes are refused, not any path naming a secret.
func TestAIScriptSecretRoutes(t *testing.T) {
	p := &AIScriptPolicy{Mode: AIScriptModeWrite}
	for _, path := range []string{
		"/api/spaces/secret-x",
		"/api/files/stat/b/docs/secret-plan.md",
		"/api/files/objects/b/secrets/notes.txt",
		"/api/templatevars/secret",
	} {
		if err := p.Check("GET", path); err != nil {
			t.Errorf("GET %s refused: %v", path, err)
		}
	}
	for _, path := range []string{
		"/api/secrets",
		"/api/secrets/x",
		"/api/users/whoami/secrets",
		"/api/users/u1/secrets/x",
		"/api/users/whoami/onepassword-token",
		"/api/users/u1/onepassword-token",
		"/api/onepassword/items",
		"/api/vault/x",
	} {
		if err := p.Check("GET", path); err == nil {
			t.Errorf("GET %s allowed", path)
		}
	}
}

// Item 1: a refused call costs nothing to build, and a body that shares
// references cannot expand into gigabytes of Go values.
func TestRunAIScriptSharedReferenceBodies(t *testing.T) {
	api, client := newSinkAPI(t, slackSink())
	code := "a = [0] * 1000\nb = [a] * 1000\nc = [b] * 20\napiclient.post(\"/api/spaces\", c)\n"
	measure := func(mode string) (*AIScriptRun, uint64) {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		run := runTestScript(t, client, mode, code)
		runtime.ReadMemStats(&after)
		return run, after.TotalAlloc - before.TotalAlloc
	}
	run, alloc := measure(AIScriptModeRead)
	if run.OK || !strings.Contains(run.Error, "read mode") {
		t.Errorf("read mode: ok=%v error=%q", run.OK, run.Error)
	}
	if alloc > 200<<20 {
		t.Errorf("a refused POST allocated %d MB", alloc>>20)
	}
	run, alloc = measure(AIScriptModeWrite)
	if run.OK || !strings.Contains(run.Error, "larger than") {
		t.Errorf("write mode: ok=%v error=%q", run.OK, run.Error)
	}
	if alloc > 200<<20 {
		t.Errorf("an oversized POST allocated %d MB", alloc>>20)
	}
	// Headers and query parameters are bounded too.
	run = runTestScript(t, client, AIScriptModeRead, "a = [\"x\" * 1000] * 1000\nb = [a] * 100\napiclient.get(\"/api/spaces\", {\"q\": b})\n")
	if run.OK || !strings.Contains(run.Error, "larger than") {
		t.Errorf("params: ok=%v error=%q", run.OK, run.Error)
	}
	run = runTestScript(t, client, AIScriptModeWrite, "a = [\"x\" * 1000] * 1000\napiclient.delete(\"/api/spaces/s1\", {\"X-A\": a})\n")
	if run.OK || !strings.Contains(run.Error, "larger than") {
		t.Errorf("headers: ok=%v error=%q", run.OK, run.Error)
	}
	if api.hits != 0 {
		t.Errorf("%d refused calls reached the API", api.hits)
	}
}

// Item 9: one user may run AIScriptMaxConcurrentPerUser scripts at once,
// and the server AIScriptMaxConcurrent.
func TestAIScriptConcurrencyCaps(t *testing.T) {
	oldUser, oldAll := AIScriptMaxConcurrentPerUser, AIScriptMaxConcurrent
	AIScriptMaxConcurrentPerUser, AIScriptMaxConcurrent = 2, 3
	defer func() { AIScriptMaxConcurrentPerUser, AIScriptMaxConcurrent = oldUser, oldAll }()

	r1, err := acquireAIScriptSlot("a")
	if err != nil {
		t.Fatal(err)
	}
	r2, err := acquireAIScriptSlot("a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireAIScriptSlot("a"); !errors.Is(err, ErrAIScriptBusy) || !strings.Contains(err.Error(), "try again") {
		t.Errorf("third run for one user: %v", err)
	}
	r3, err := acquireAIScriptSlot("b")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireAIScriptSlot("c"); !errors.Is(err, ErrAIScriptBusy) || !strings.Contains(err.Error(), "Try again") {
		t.Errorf("run over the server cap: %v", err)
	}
	// RunAIScript refuses before it runs anything.
	user := &model.User{Id: "c", Username: "c", Roles: []string{model.RoleAdminUUID, aiScriptsRole(t)}}
	if _, err := RunAIScript(t.Context(), user, AIScriptRequest{Code: "print(1)", Purpose: "p"}); !errors.Is(err, ErrAIScriptBusy) {
		t.Errorf("RunAIScript over the cap: %v", err)
	}
	r1()
	r1() // releasing twice gives back one place
	r4, err := acquireAIScriptSlot("c")
	if err != nil {
		t.Fatalf("after a release: %v", err)
	}
	if _, err := acquireAIScriptSlot("d"); err == nil {
		t.Error("a double release gave back two places")
	}
	r2()
	r3()
	r4()
	aiScriptSlots.Lock()
	defer aiScriptSlots.Unlock()
	if aiScriptSlots.total != 0 || len(aiScriptSlots.byUser) != 0 {
		t.Errorf("slots left: %d %v", aiScriptSlots.total, aiScriptSlots.byUser)
	}
}
