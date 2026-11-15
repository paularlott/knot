package service

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/scriptdocs"
	"github.com/paularlott/knot/internal/util/rest"
)

func TestAIScriptPolicyReadModeRefusesWrites(t *testing.T) {
	p := &AIScriptPolicy{Mode: AIScriptModeRead}
	for _, method := range []string{"GET", "HEAD", "get"} {
		if err := p.Check(method, "/api/spaces"); err != nil {
			t.Errorf("read mode %s refused: %v", method, err)
		}
	}
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		err := p.Check(method, "/api/spaces/s1/stop")
		if err == nil || !strings.Contains(err.Error(), "read mode") {
			t.Errorf("read mode %s: got %v, want a read mode refusal", method, err)
		}
	}
	// An unset mode is read mode.
	if err := (&AIScriptPolicy{}).Check("POST", "/api/spaces"); err == nil {
		t.Error("empty mode allowed a POST")
	}
}

func TestAIScriptPolicyDenyList(t *testing.T) {
	denied := []struct{ method, path string }{
		{"GET", "/api/tokens"},
		{"POST", "/api/tokens"},
		{"DELETE", "/api/tokens/t1"},
		{"POST", "/api/auth"},
		{"POST", "/api/auth/totp/verify"},
		{"GET", "/api/sessions"},
		{"DELETE", "/api/sessions/s1"},
		{"GET", "/api/users/u1/passkeys"},
		{"DELETE", "/api/users/u1/passkeys/p1"},
		{"POST", "/api/users/whoami/passkeys/begin"},
		{"PUT", "/api/users/u1/linked-users/u2"},
		{"DELETE", "/api/users/u1/linked-users/u2"},
		{"PUT", "/api/users/whoami/ssh-private-key"},
		{"PUT", "/api/users/whoami/ssh-public-key"},
		{"GET", "/api/users/whoami/onepassword-token"},
		{"DELETE", "/api/users/u1/auth-provider/p1"},
		{"GET", "/api/backup/info"},
		{"GET", "/api/backup/users"},
		{"POST", "/api/restore/users"},
		{"PUT", "/api/restore/content/abc"},
		{"POST", "/api/roles"},
		{"PUT", "/api/roles/r1"},
		{"DELETE", "/api/roles/r1"},
		{"POST", "/api/groups"},
		{"PUT", "/api/groups/g1"},
		{"DELETE", "/api/groups/g1"},
		{"POST", "/api/users"},
		{"PUT", "/api/users/u1"},
		{"DELETE", "/api/users/u1"},
		{"POST", "/api/chat/tools/call"},
		{"GET", "/api/chat/tools"},
		{"GET", "/api/events"},
		{"GET", "/api/spaces/s1/execute-script-stream"},
		{"GET", "/api/mcp-servers/m1/tools"},
		{"GET", "/api/mcp-servers"},
		{"POST", "/api/mcp-servers"},
		{"GET", "/api/users/whoami/secrets"},
		{"GET", "/api/secrets/x"},
	}
	p := &AIScriptPolicy{Mode: AIScriptModeWrite}
	for _, d := range denied {
		err := p.Check(d.method, d.path)
		if err == nil || !strings.Contains(err.Error(), "in either mode") {
			t.Errorf("write mode %s %s: got %v, want a deny list refusal", d.method, d.path, err)
		}
	}

	allowed := []struct{ method, path string }{
		{"GET", "/api/roles"},
		{"GET", "/api/roles/r1"},
		{"GET", "/api/groups"},
		{"GET", "/api/users"},
		{"GET", "/api/users/whoami"},
		{"GET", "/api/users/u1/quota"},
		{"PUT", "/api/users/preferences/nav"},
		{"POST", "/api/spaces/s1/stop"},
		{"POST", "/api/spaces"},
		{"DELETE", "/api/spaces/s1"},
		{"PUT", "/api/files/objects/b/k"},
		{"GET", "/api/spaces?user_id=u1&all_zones=true"},
		{"GET", "/api/tunnels"},
		{"POST", "/api/event-sinks"},
	}
	for _, a := range allowed {
		if err := p.Check(a.method, a.path); err != nil {
			t.Errorf("write mode %s %s refused: %v", a.method, a.path, err)
		}
	}
}

func TestAIScriptPolicyRefusesOddPaths(t *testing.T) {
	p := &AIScriptPolicy{Mode: AIScriptModeWrite}
	for _, path := range []string{
		"/api/spaces/../tokens",
		"/api/./tokens",
		"http://example.com/api/spaces",
		"//example.com/api/spaces",
		"/api/x%2Ftokens",
		"/v1/chat/completions",
		"/mcp",
		"/switch-user/u2",
		"api/spaces",
		"",
	} {
		if err := p.Check("GET", path); err == nil {
			t.Errorf("GET %q allowed", path)
		}
	}
}

func TestAIScriptPolicyCallCap(t *testing.T) {
	p := &AIScriptPolicy{Mode: AIScriptModeRead, MaxCalls: 3}
	for i := 0; i < 3; i++ {
		if err := p.Check("GET", "/api/spaces"); err != nil {
			t.Fatalf("call %d refused: %v", i+1, err)
		}
	}
	err := p.Check("GET", "/api/spaces")
	if err == nil || !strings.Contains(err.Error(), "at most 3 API calls") {
		t.Fatalf("call 4: got %v, want the call cap", err)
	}
	if p.Calls() != 3 {
		t.Errorf("Calls() = %d, want 3", p.Calls())
	}
	// Refused calls (mode, deny list) do not use the budget.
	q := &AIScriptPolicy{Mode: AIScriptModeRead, MaxCalls: 1}
	_ = q.Check("POST", "/api/spaces")
	_ = q.Check("GET", "/api/tokens")
	if err := q.Check("GET", "/api/spaces"); err != nil {
		t.Errorf("first allowed call refused: %v", err)
	}
}

func TestRedactSecrets(t *testing.T) {
	var v interface{}
	_ = json.Unmarshal([]byte(`{"username":"a","service_password":"pw","ssh_private_key":"k","totp_secret":"",
		"nested":[{"token":"t","name":"x"}]}`), &v)
	redactSecrets(&v)
	m := v.(map[string]interface{})
	if m["service_password"] != redactedValue || m["ssh_private_key"] != redactedValue {
		t.Errorf("credentials not redacted: %v", m)
	}
	if m["totp_secret"] != "" {
		t.Errorf("an empty value should stay empty: %v", m["totp_secret"])
	}
	if m["username"] != "a" {
		t.Errorf("username changed: %v", m["username"])
	}
	item := m["nested"].([]interface{})[0].(map[string]interface{})
	if item["token"] != redactedValue || item["name"] != "x" {
		t.Errorf("nested: %v", item)
	}
}

func TestNormalizeAIScriptRequest(t *testing.T) {
	req, err := NormalizeAIScriptRequest(AIScriptRequest{Code: "print(1)", Purpose: " p "})
	if err != nil || req.Mode != AIScriptModeRead || req.Purpose != "p" {
		t.Errorf("got %+v, %v", req, err)
	}
	for _, mode := range []string{AIScriptModeRead, AIScriptModeWrite} {
		if req, err := NormalizeAIScriptRequest(AIScriptRequest{Code: "x", Purpose: "p", Mode: mode}); err != nil || req.Mode != mode {
			t.Errorf("mode %q: got %+v, %v", mode, req, err)
		}
	}
	for _, bad := range []AIScriptRequest{
		{Code: " ", Purpose: "p"},
		{Code: "x", Purpose: " "},
		{Code: "x", Purpose: "p", Mode: "admin"},
		// The web assistant's confirmation matches "write" exactly, so a
		// variant must not be taken as write mode (nor as read mode).
		{Code: "x", Purpose: "p", Mode: "WRITE"},
		{Code: "x", Purpose: "p", Mode: " write"},
		{Code: "x", Purpose: "p", Mode: "write "},
		{Code: "x", Purpose: "p", Mode: "Write"},
		{Code: "x", Purpose: "p", Mode: "READ"},
		{Code: "x", Purpose: "p", Mode: " "},
	} {
		if _, err := NormalizeAIScriptRequest(bad); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}

func TestAIScriptPermission(t *testing.T) {
	model.SetRoleCache(nil)
	if err := CheckAIScriptPermission(nil); err != ErrAIScriptPermission {
		t.Errorf("nil user: %v", err)
	}
	plain := &model.User{Id: "u1"}
	if err := CheckAIScriptPermission(plain); err != ErrAIScriptPermission {
		t.Errorf("user without the permission: %v", err)
	}
	if _, err := RunAIScript(context.Background(), plain, AIScriptRequest{Code: "print(1)", Purpose: "p"}); err != ErrAIScriptPermission {
		t.Errorf("RunAIScript without the permission: %v", err)
	}
	admin := &model.User{Id: "u2", Roles: []string{model.RoleAdminUUID}}
	if err := CheckAIScriptPermission(admin); err != ErrAIScriptPermission {
		t.Errorf("admin without an AI scripts role: %v", err)
	}
	granted := &model.User{Id: "u3", Roles: []string{model.RoleAdminUUID, aiScriptsRole(t)}}
	if err := CheckAIScriptPermission(granted); err != nil {
		t.Errorf("user with the permission refused: %v", err)
	}
}

// fakeKnotAPI serves the few endpoints the run tests use through the mux
// client, and counts the stops.
func fakeKnotAPI(t *testing.T) (*int32, rest.RESTClient) {
	t.Helper()
	var stops int32
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("GET /api/users/whoami", func(w http.ResponseWriter, r *http.Request) {
		write(w, map[string]interface{}{"user_id": "u1", "username": "kai", "service_password": "hunter2"})
	})
	mux.HandleFunc("GET /api/spaces", func(w http.ResponseWriter, r *http.Request) {
		write(w, map[string]interface{}{"spaces": []map[string]interface{}{
			{"space_id": "s1", "name": "test-a", "template_name": "go", "is_deployed": true},
			{"space_id": "s2", "name": "test-b", "template_name": "go", "is_deployed": false},
			{"space_id": "s3", "name": "prod", "template_name": "node", "is_deployed": true},
		}})
	})
	mux.HandleFunc("POST /api/spaces/{id}/stop", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&stops, 1)
		write(w, map[string]interface{}{"status": true})
	})
	mux.HandleFunc("GET /api/templates", func(w http.ResponseWriter, r *http.Request) {
		write(w, map[string]interface{}{"templates": []map[string]interface{}{
			{"template_id": "t1", "name": "go", "active": true, "usage": 2},
			{"template_id": "t2", "name": "rust", "active": true, "usage": 0},
		}})
	})
	mux.HandleFunc("GET /api/spaces/{id}/usage/current", func(w http.ResponseWriter, r *http.Request) {
		write(w, map[string]interface{}{"memory_bytes": 1024})
	})
	mux.HandleFunc("GET /api/tokens", func(w http.ResponseWriter, r *http.Request) {
		t.Error("the deny list let GET /api/tokens through")
	})
	rest.SetAPIMux(mux)
	t.Cleanup(func() { rest.SetAPIMux(http.NewServeMux()) })
	return &stops, rest.NewMuxClient(&model.User{Id: "u1"})
}

func runTestScript(t *testing.T, client rest.RESTClient, mode, code string) *AIScriptRun {
	t.Helper()
	user := &model.User{Id: "u1", Username: "kai", Roles: []string{model.RoleAdminUUID, aiScriptsRole(t)}}
	return runAIScript(context.Background(), user, client, AIScriptRequest{Code: code, Purpose: "test", Mode: mode})
}

func TestRunAIScriptReadsAndReturnsResult(t *testing.T) {
	_, client := fakeKnotAPI(t)
	run := runTestScript(t, client, AIScriptModeRead, `
counts = defaultdict(int)
for s in space.list():
    counts[s["template_name"]] += 1
print("templates:", len(counts))
result = dict(counts)
`)
	if !run.OK {
		t.Fatalf("run failed: %s", run.Error)
	}
	got, _ := json.Marshal(run.Result)
	if string(got) != `{"go":2,"node":1}` {
		t.Errorf("result = %s", got)
	}
	if !strings.Contains(run.Output, "templates: 2") {
		t.Errorf("output = %q", run.Output)
	}
	if run.Calls != 2 { // whoami + spaces
		t.Errorf("calls = %d, want 2", run.Calls)
	}
}

func TestRunAIScriptRedactsSecrets(t *testing.T) {
	_, client := fakeKnotAPI(t)
	run := runTestScript(t, client, AIScriptModeRead, `result = apiclient.get("/api/users/whoami")["service_password"]`)
	if !run.OK || run.Result != redactedValue {
		t.Errorf("got %+v", run)
	}
}

func TestRunAIScriptReadModeCannotWrite(t *testing.T) {
	stops, client := fakeKnotAPI(t)
	run := runTestScript(t, client, AIScriptModeRead, `space.stop("test-a")`)
	if run.OK || !strings.Contains(run.Error, "read mode") {
		t.Errorf("got ok=%v error=%q, want a read mode refusal", run.OK, run.Error)
	}
	if atomic.LoadInt32(stops) != 0 {
		t.Error("a read mode script stopped a space")
	}
}

func TestRunAIScriptWriteMode(t *testing.T) {
	stops, client := fakeKnotAPI(t)
	run := runTestScript(t, client, AIScriptModeWrite, `
stopped = []
for s in space.list():
    if s["name"].startswith("test-") and s["is_running"]:
        space.stop(s["name"])
        stopped.append(s["name"])
result = stopped
`)
	if !run.OK {
		t.Fatalf("run failed: %s", run.Error)
	}
	if atomic.LoadInt32(stops) != 1 {
		t.Errorf("stops = %d, want 1", *stops)
	}
	// The deny list holds in write mode.
	run = runTestScript(t, client, AIScriptModeWrite, `apiclient.get("/api/tokens")`)
	if run.OK || !strings.Contains(run.Error, "API tokens") {
		t.Errorf("tokens: got ok=%v error=%q", run.OK, run.Error)
	}
}

func TestRunAIScriptCallCap(t *testing.T) {
	_, client := fakeKnotAPI(t)
	old := AIScriptMaxCalls
	AIScriptMaxCalls = 5
	defer func() { AIScriptMaxCalls = old }()
	run := runTestScript(t, client, AIScriptModeRead, `
for i in range(10):
    apiclient.get("/api/users/whoami")
`)
	if run.OK || !strings.Contains(run.Error, "at most 5 API calls") {
		t.Errorf("got ok=%v error=%q", run.OK, run.Error)
	}
	if run.Calls != 5 {
		t.Errorf("calls = %d, want 5", run.Calls)
	}
}

func TestRunAIScriptLimits(t *testing.T) {
	_, client := fakeKnotAPI(t)

	run := runTestScript(t, client, AIScriptModeRead, `
for i in range(20000):
    print("0123456789")
`)
	if !run.OK || !run.Truncated || len(run.Output) > AIScriptMaxOutput+100 || !strings.Contains(run.Output, "truncated") {
		t.Errorf("output cap: ok=%v truncated=%v len=%d", run.OK, run.Truncated, len(run.Output))
	}

	run = runTestScript(t, client, AIScriptModeRead, `result = ["x" * 100] * 1000`)
	if run.OK || run.Result != nil || !strings.Contains(run.Error, "byte limit") {
		t.Errorf("result cap: ok=%v error=%q", run.OK, run.Error)
	}

	old := AIScriptTimeout
	AIScriptTimeout = 200 * time.Millisecond
	defer func() { AIScriptTimeout = old }()
	run = runTestScript(t, client, AIScriptModeRead, `
while True:
    pass
`)
	if run.OK || !strings.Contains(run.Error, "time limit") {
		t.Errorf("timeout: ok=%v error=%q", run.OK, run.Error)
	}
}

func TestRunAIScriptEnvironmentIsNarrow(t *testing.T) {
	_, client := fakeKnotAPI(t)
	for _, module := range []string{"requests", "os", "subprocess", "logging", "secrets", "scriptling.ai", "scriptling.mcp", "knot.ai", "knot.mcp", "knot.token", "knot.secret", "knot.event", "knot.plugin", "platform"} {
		run := runTestScript(t, client, AIScriptModeRead, "import "+module+"\n")
		if run.OK {
			t.Errorf("import %s succeeded", module)
		}
	}
	run := runTestScript(t, client, AIScriptModeRead, `
import itertools
import knot.space
result = [len(docs()) > 0, "knot.space" in docs("space"), identity.user().id]
`)
	if !run.OK {
		t.Fatalf("run failed: %s", run.Error)
	}
	if got, _ := json.Marshal(run.Result); string(got) != `[true,true,"u1"]` {
		t.Errorf("result = %s", got)
	}
}

func TestRunAIScriptErrorLinesAreTheScripts(t *testing.T) {
	_, client := fakeKnotAPI(t)
	run := runTestScript(t, client, AIScriptModeRead, "x = 1\ny = 2\nraise Exception(\"boom\")\n")
	if run.OK || !strings.Contains(run.Error, "boom") {
		t.Fatalf("got ok=%v error=%q", run.OK, run.Error)
	}
	run = runTestScript(t, client, AIScriptModeRead, "x = 1\ny = 2\nz = undefined_name + 1\n")
	if run.OK || !strings.Contains(run.Error, "(line 3)") {
		t.Errorf("error line not renumbered to the script's: %q", run.Error)
	}
	run = runTestScript(t, client, AIScriptModeRead, "x = 1\n")
	if !run.OK || run.Note == "" {
		t.Errorf("a silent script should get a note: %+v", run)
	}
}

// Every library an AI script may import has a page in the scripting docs,
// and every page names a library a script can import.
func TestAIScriptLibrariesMatchDocs(t *testing.T) {
	var offered []string
	offered = append(offered, aiScriptGoLibraries...)
	offered = append(offered, aiScriptLibraries...)
	sort.Strings(offered)
	documented := scriptdocs.Names()
	if strings.Join(offered, ",") != strings.Join(documented, ",") {
		t.Errorf("AI script libraries %v != documented libraries %v", offered, documented)
	}
	for name := range scriptdocs.NotForScripts {
		for _, o := range offered {
			if o == name {
				t.Errorf("%s is offered to AI scripts but listed in scriptdocs.NotForScripts", name)
			}
		}
	}
}

// The guide's worked examples run as written: each python block of
// writing-scripts.md against the fake API, in write mode when it changes
// something.
func TestAIScriptGuideExamplesRun(t *testing.T) {
	stops, client := fakeKnotAPI(t)
	guide := scriptdocs.Guide()
	blocks := strings.Split(guide, "```python\n")[1:]
	if len(blocks) < 4 {
		t.Fatalf("guide has %d examples, want at least 4", len(blocks))
	}
	for i, block := range blocks {
		code := block[:strings.Index(block, "```")]
		mode := AIScriptModeRead
		if strings.Contains(code, "space.stop") {
			mode = AIScriptModeWrite
		}
		run := runTestScript(t, client, mode, code)
		if !run.OK {
			t.Errorf("example %d failed: %s\n%s", i+1, run.Error, code)
		}
	}
	if atomic.LoadInt32(stops) != 1 {
		t.Errorf("the write example stopped %d spaces, want 1", *stops)
	}

	run := runTestScript(t, client, AIScriptModeRead, `help(space.list)`)
	if !run.OK || !strings.Contains(run.Output, "all_zones") {
		t.Errorf("help(space.list): ok=%v output=%q error=%q", run.OK, run.Output, run.Error)
	}
}

// aiScriptsRole caches a role carrying Run AI Scripts, which no built-in
// role holds, and returns its id.
func aiScriptsRole(t *testing.T) string {
	t.Helper()
	role := model.NewRole("AI Scripts", []uint16{model.PermissionRunAIScripts}, "")
	model.SaveRoleToCache(role)
	t.Cleanup(func() { model.DeleteRoleFromCache(role.Id) })
	return role.Id
}
