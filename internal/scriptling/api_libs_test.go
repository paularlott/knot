package scriptling

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/libloader"
	"github.com/paularlott/scriptling/object"
	"github.com/paularlott/scriptling/stdlib"
)

// apiCall is one request a knot.* library made to the fake API.
type apiCall struct {
	Method      string
	URI         string // path and query, as sent
	ContentType string
	Body        string
}

// fakeAPI answers knot.* library requests from canned JSON keyed by
// "METHOD /path" (no query) and records every request in order.
type fakeAPI struct {
	mu        sync.Mutex
	calls     []apiCall
	responses map[string]string
	status    map[string]int
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.calls = append(f.calls, apiCall{Method: r.Method, URI: r.URL.RequestURI(), ContentType: r.Header.Get("Content-Type"), Body: string(body)})
	key := r.Method + " " + r.URL.EscapedPath()
	resp, ok := f.responses[key]
	status := f.status[key]
	f.mu.Unlock()
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"no fake for ` + key + `"}`))
		return
	}
	if status == 0 {
		status = http.StatusOK
	}
	if strings.HasPrefix(resp, "csv:") {
		w.Header().Set("Content-Type", "text/csv")
		w.WriteHeader(status)
		w.Write([]byte(strings.TrimPrefix(resp, "csv:")))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write([]byte(resp))
}

// find returns the recorded calls whose "METHOD uri" starts with prefix.
func (f *fakeAPI) find(prefix string) []apiCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []apiCall
	for _, c := range f.calls {
		if strings.HasPrefix(c.Method+" "+c.URI, prefix) {
			out = append(out, c)
		}
	}
	return out
}

// libEnv runs scripts with the embedded knot.* Python libraries over the Go
// knot.apiclient transport, talking to the fake API over HTTP.
func libEnv(t *testing.T, responses map[string]string, status map[string]int) (*fakeAPI, func(script string) (interface{}, error)) {
	t.Helper()
	fake := &fakeAPI{responses: responses, status: status}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	client, err := rest.NewClient(srv.URL, "token", true)
	if err != nil {
		t.Fatal(err)
	}

	return fake, func(script string) (interface{}, error) {
		env := scriptling.New()
		stdlib.RegisterAll(env)
		env.RegisterLibrary(GetApiClientLibrary(client, "u1"))
		env.SetLibraryLoader(libloader.NewFuncLoader(func(name string) (string, bool, error) {
			if !strings.HasPrefix(name, "knot.") {
				return "", false, nil
			}
			data, err := EmbeddedLibs.ReadFile("lib/knot/" + strings.TrimPrefix(name, "knot.") + ".py")
			if err != nil {
				return "", false, nil
			}
			return string(data), true, nil
		}, "test-embedded"))
		res, err := env.EvalWithContext(context.Background(), script)
		if err != nil {
			return nil, err
		}
		if e, ok := res.(*object.Error); ok {
			return nil, errorString(e.Message)
		}
		v, errObj := env.GetVar("result")
		if errObj != nil {
			return nil, errorString("script set no result")
		}
		return v, nil
	}
}

func mustRun(t *testing.T, run func(string) (interface{}, error), script string) map[string]interface{} {
	t.Helper()
	v, err := run(script)
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, script)
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("result is %T (%v), want a dict", v, v)
	}
	return m
}

func jsonBody(t *testing.T, c apiCall) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(c.Body), &m); err != nil {
		t.Fatalf("%s %s body %q: %v", c.Method, c.URI, c.Body, err)
	}
	return m
}

const (
	sinkID   = "11111111-1111-1111-1111-111111111111"
	serverID = "22222222-2222-2222-2222-222222222222"
	spaceID  = "33333333-3333-3333-3333-333333333333"
)

func TestKnotEventSinkLib(t *testing.T) {
	sink := `{"event_sink_id":"` + sinkID + `","user_id":"u1","name":"hook","description":"d","events":["space.*"],"sink_type":"webhook","webhook":{"url":"https://h.example/x","secret":"s3cret","headers":{"X-A":"1"},"body_template":"","skip_tls_verify":false},"active":true}`
	fake, run := libEnv(t, map[string]string{
		"GET /api/event-sinks":              `{"count":1,"event_sinks":[` + sink + `]}`,
		"GET /api/event-sinks/" + sinkID:    sink,
		"POST /api/event-sinks":             `{"status":true,"event_sink_id":"` + sinkID + `"}`,
		"PUT /api/event-sinks/" + sinkID:    `{"status":true}`,
		"DELETE /api/event-sinks/" + sinkID: `{"status":true}`,
	}, map[string]int{"POST /api/event-sinks": 201})

	r := mustRun(t, run, `
import knot.event_sink as es
result = {}
result["id"] = es.create("hook2", ["custom.myapp.*"], url="https://h.example/y", is_global=True)
result["list"] = es.list()
result["get"] = es.get("hook")
es.update("hook", active=False, events=["*"])
es.delete(result["id"])
`)
	if r["id"] != sinkID {
		t.Fatalf("create id = %v", r["id"])
	}
	got := r["get"].(map[string]interface{})
	if got["is_global"] != false || got["sink_type"] != "webhook" || got["webhook"].(map[string]interface{})["url"] != "https://h.example/x" {
		t.Fatalf("get = %v", got)
	}

	create := jsonBody(t, fake.find("POST /api/event-sinks")[0])
	if create["user_id"] != "" || create["sink_type"] != "webhook" || create["webhook"].(map[string]interface{})["url"] != "https://h.example/y" {
		t.Fatalf("create body = %v", create)
	}
	upd := jsonBody(t, fake.find("PUT /api/event-sinks/" + sinkID)[0])
	wh := upd["webhook"].(map[string]interface{})
	if upd["active"] != false || upd["name"] != "hook" || wh["url"] != "https://h.example/x" || wh["secret"] != "" {
		t.Fatalf("update body = %v (want url kept, secret empty to keep it)", upd)
	}
	if evs := upd["events"].([]interface{}); len(evs) != 1 || evs[0] != "*" {
		t.Fatalf("update events = %v", upd["events"])
	}
	if len(fake.find("DELETE /api/event-sinks/"+sinkID)) != 1 {
		t.Fatal("delete not sent")
	}
	if _, err := run("import knot.event_sink as es\nes.get('nope')"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unknown sink: %v", err)
	}
}

func TestKnotMCPServerLib(t *testing.T) {
	details := `{"mcp_server_id":"` + serverID + `","user_id":"u1","namespace":"gh","url":"https://mcp.example","command":"","args":[],"env":[],"auth_type":"bearer","token":"tok","enabled":true,"tool_visibility":"native","disabled_tools":["x"],"remote_search":false}`
	fake, run := libEnv(t, map[string]string{
		"GET /api/mcp-servers":                               `{"count":1,"servers":[` + details + `]}`,
		"GET /api/mcp-servers/" + serverID:                   details,
		"POST /api/mcp-servers":                              `{"status":true,"mcp_server_id":"` + serverID + `"}`,
		"PUT /api/mcp-servers/" + serverID:                   ``,
		"DELETE /api/mcp-servers/" + serverID:                ``,
		"GET /api/mcp-servers/" + serverID + "/tools":        `{"tools":[{"name":"search","description":"Search","enabled":true,"is_app":true}]}`,
		"POST /api/mcp-servers/" + serverID + "/toggle-tool": ``,
		"GET /api/mcp-servers/" + serverID + "/protocol":     `{"protocol_version":"2025-06-18"}`,
	}, map[string]int{"POST /api/mcp-servers": 201})

	r := mustRun(t, run, `
import knot.mcp_server as ms
result = {}
result["id"] = ms.create("gh", url="https://mcp.example", auth_type="bearer", token="tok")
result["get"] = ms.get("gh")
ms.update("gh", enabled=False)
result["tools"] = ms.tools("gh")
ms.set_tool_enabled("gh", "search", False)
result["proto"] = ms.protocol_version(result["id"])
ms.delete("gh")
`)
	got := r["get"].(map[string]interface{})
	if _, leaked := got["token"]; leaked || got["has_token"] != true || got["auth_type"] != "bearer" {
		t.Fatalf("get = %v (want token redacted, has_token set)", got)
	}
	upd := jsonBody(t, fake.find("PUT /api/mcp-servers/" + serverID)[0])
	if upd["enabled"] != false || upd["token"] != "tok" || upd["namespace"] != "gh" {
		t.Fatalf("update body = %v (want enabled off, token kept)", upd)
	}
	tools := r["tools"].([]interface{})
	if len(tools) != 1 || tools[0].(map[string]interface{})["is_app"] != true {
		t.Fatalf("tools = %v", tools)
	}
	toggle := jsonBody(t, fake.find("POST /api/mcp-servers/" + serverID + "/toggle-tool")[0])
	if toggle["tool_name"] != "search" || toggle["enabled"] != false {
		t.Fatalf("toggle body = %v", toggle)
	}
	if r["proto"] != "2025-06-18" {
		t.Fatalf("protocol = %v", r["proto"])
	}

	_, run2 := libEnv(t, map[string]string{
		"GET /api/mcp-servers/" + serverID + "/tools": `{"tools":[],"error":"connection refused"}`,
	}, nil)
	if _, err := run2("import knot.mcp_server as ms\nms.tools('" + serverID + "')"); err == nil || !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("unreachable server: %v", err)
	}
}

func TestKnotTunnelAndServerLib(t *testing.T) {
	fake, run := libEnv(t, map[string]string{
		"GET /api/tunnels":               `[{"name":"alice--web","address":"https://alice--web.t.example"}]`,
		"GET /api/tunnels/server-info":   `{"domain":".t.example","tunnel_servers":["wss://t1.example"]}`,
		"GET /api/users/whoami":          `{"user_id":"u1","username":"alice"}`,
		"DELETE /api/tunnels/alice--web": `{"error":"tunnel deleted"}`,
		"GET /api/ping":                  `{"status":true,"version":"1.2.3","zone":"z1"}`,
		"GET /api/cluster-info":          `[{"id":"n1","address":"10.0.0.1:12000","state":"alive","metadata":{"zone":"z1","hostname":"h1","allocated_spaces":"3","running_spaces":"2","runtimes":"docker,podman"}}]`,
		"GET /api/search":                `{"query":"web app","spaces":[{"id":"s1","name":"web app"}]}`,
		"GET /api/plugins":               `{"count":1,"plugins":[{"name":"jira","version":"1.0"}],"failed":[],"warnings":[]}`,
	}, nil)

	r := mustRun(t, run, `
import knot.tunnel as tunnel
import knot.server as server
result = {}
result["tunnels"] = tunnel.list()
result["info"] = tunnel.server_info()
tunnel.delete("web")
result["ping"] = server.ping()
result["cluster"] = server.cluster_info()
result["search"] = server.search("web app")
result["plugins"] = server.plugins()
`)
	if ts := r["tunnels"].([]interface{}); len(ts) != 1 || ts[0].(map[string]interface{})["name"] != "alice--web" {
		t.Fatalf("tunnels = %v", ts)
	}
	if len(fake.find("DELETE /api/tunnels/alice--web")) != 1 {
		t.Fatal("bare tunnel name not prefixed with the username")
	}
	if r["ping"].(map[string]interface{})["version"] != "1.2.3" {
		t.Fatalf("ping = %v", r["ping"])
	}
	node := r["cluster"].([]interface{})[0].(map[string]interface{})
	if node["running_spaces"] != int64(2) || len(node["runtimes"].([]interface{})) != 2 || node["hostname"] != "h1" {
		t.Fatalf("cluster node = %v", node)
	}
	if c := fake.find("GET /api/search"); len(c) != 1 || c[0].URI != "/api/search?q=web%20app" {
		t.Fatalf("search request = %v", c)
	}
	search := r["search"].(map[string]interface{})
	if len(search["spaces"].([]interface{})) != 1 || len(search["templates"].([]interface{})) != 0 {
		t.Fatalf("search = %v", search)
	}
	if r["plugins"].(map[string]interface{})["count"] != int64(1) {
		t.Fatalf("plugins = %v", r["plugins"])
	}
}

func TestKnotSpaceMethodsAndEvents(t *testing.T) {
	fake, run := libEnv(t, map[string]string{
		"GET /api/spaces/web":                         `{"space_id":"` + spaceID + `","name":"web"}`,
		"POST /api/spaces/" + spaceID + "/emit-event": ``,
		"POST /api/spaces/alice/stop-for-user":        ``,
		"GET /api/spaces":                             `{"count":0,"spaces":[]}`,
		"GET /api/methods":                            `{"count":1,"methods":[{"name":"greet","local_name":"greet","scope":"private","owner":"alice","provider_count":2}]}`,
		"POST /api/methods/call":                      `{"jsonrpc":"2.0","id":1,"result":{"hello":"world"}}`,
		"PUT /api/tokens/tok1":                        ``,
		"GET /api/stacks/dev/exists":                  `{"exists":true}`,
		"POST /api/spaces/stacks/dev/start":           ``,
	}, nil)

	r := mustRun(t, run, `
import knot.space as space
import knot.token as token
import knot.stack as stack
result = {}
space.emit_event("web", "myapp.deployed", {"v": 1})
space.stop_for_user("alice")
space.list(user_id="u9")
result["methods"] = space.list_methods()
result["call"] = space.call_method("greet", {"name": "bob"}, space="web")
token.update("tok1", scopes=[])
result["exists"] = stack.exists("dev", user_id="u9")
stack.start("dev")
`)
	emit := jsonBody(t, fake.find("POST /api/spaces/" + spaceID + "/emit-event")[0])
	if emit["type"] != "myapp.deployed" || emit["payload"].(map[string]interface{})["v"] != float64(1) {
		t.Fatalf("emit body = %v", emit)
	}
	if len(fake.find("POST /api/spaces/alice/stop-for-user")) != 1 {
		t.Fatal("stop-for-user not sent")
	}
	if c := fake.find("GET /api/spaces?"); len(c) != 1 || !strings.Contains(c[0].URI, "user_id=u9") {
		t.Fatalf("space list = %v", c)
	}
	if m := r["methods"].([]interface{}); len(m) != 1 || m[0].(map[string]interface{})["provider_count"] != int64(2) {
		t.Fatalf("methods = %v", m)
	}
	if r["call"].(map[string]interface{})["hello"] != "world" {
		t.Fatalf("call = %v", r["call"])
	}
	call := jsonBody(t, fake.find("POST /api/methods/call")[0])
	if call["method"] != "greet" || call["space_id"] != spaceID || call["jsonrpc"] != "2.0" {
		t.Fatalf("call body = %v", call)
	}
	tok := jsonBody(t, fake.find("PUT /api/tokens/tok1")[0])
	if s, ok := tok["scopes"].([]interface{}); !ok || len(s) != 0 {
		t.Fatalf("token update body = %v (want scopes: [])", tok)
	}
	if _, has := tok["name"]; has {
		t.Fatalf("token update sent a name it was not given: %v", tok)
	}
	if r["exists"] != true || len(fake.find("GET /api/stacks/dev/exists?user_id=u9")) != 1 {
		t.Fatalf("stack exists = %v %v", r["exists"], fake.find("GET /api/stacks"))
	}
	if c := fake.find("POST /api/spaces/stacks/dev/start"); len(c) != 1 || c[0].URI != "/api/spaces/stacks/dev/start" {
		t.Fatalf("stack start = %v", c)
	}

	// A JSON-RPC error raises with its message.
	_, run2 := libEnv(t, map[string]string{
		"POST /api/methods/call": `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"method not found"}}`,
	}, map[string]int{"POST /api/methods/call": 404})
	if _, err := run2("import knot.space as space\nspace.call_method('nope')"); err == nil || !strings.Contains(err.Error(), "method not found") {
		t.Fatalf("method error: %v", err)
	}
	_, run3 := libEnv(t, map[string]string{
		"POST /api/methods/call": `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"boom"}}`,
	}, nil)
	if _, err := run3("import knot.space as space\nspace.call_method('x')"); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("method error in a 200: %v", err)
	}
}

func TestKnotAuditAndTemplateLib(t *testing.T) {
	fake, run := libEnv(t, map[string]string{
		"GET /api/audit-logs/export":                           `[{"audit_log_id":7,"actor":"alice","event":"Login"}]`,
		"GET /api/templates/base/export":                       `"name: base\nplatform: docker\n"`,
		"POST /api/templates/import":                           `{"status":true,"template_id":"t1"}`,
		"POST /api/spec/parse":                                 `{"wizardable":true,"spec":{"image":"ubuntu"},"fully_representable":true}`,
		"GET /api/base-images":                                 `{"version":1,"manifest_version":"2026110101","images":[{"name":"ubuntu"}],"registry_auth":false}`,
		"POST /api/base-images/refresh":                        `{"updated":true,"active_version":"2026110102","update_url":"https://x"}`,
		"GET /api/capabilities":                                `{"capabilities":[{"name":"CAP_NET_ADMIN","description":"net","common":true}]}`,
		"GET /api/plugins/field-handlers":                      `{"handlers":[{"id":"plugin.jira.projects","label":"Projects","plugin":"jira"}]}`,
		"GET /api/plugins/field-handlers/plugin.jira.projects": `{"options":[{"key":"P1","text":"Project 1"}]}`,
	}, nil)

	r := mustRun(t, run, `
import knot.audit as audit
import knot.template as template
result = {}
result["export"] = audit.export(actor="alice bob", from_time="2026-01-01T00:00:00Z")
result["yaml"] = template.export("base")
result["imported"] = template.import_yaml(result["yaml"])
result["spec"] = template.parse_spec("docker", "image: ubuntu")
result["images"] = template.base_images()
result["refresh"] = template.refresh_base_images()
result["caps"] = template.capabilities()
result["handlers"] = template.field_handlers()
result["options"] = template.field_options("plugin.jira.projects", query="P")
`)
	if c := fake.find("GET /api/audit-logs/export"); len(c) != 1 || c[0].URI != "/api/audit-logs/export?format=json&actor=alice%20bob&from=2026-01-01T00%3A00%3A00Z" {
		t.Fatalf("audit export request = %v", c)
	}
	if e := r["export"].([]interface{}); len(e) != 1 || e[0].(map[string]interface{})["actor"] != "alice" {
		t.Fatalf("audit export = %v", r["export"])
	}
	if !strings.HasPrefix(r["yaml"].(string), "name: base") || r["imported"] != "t1" {
		t.Fatalf("template export/import = %v / %v", r["yaml"], r["imported"])
	}
	var sent string
	if err := json.Unmarshal([]byte(fake.find("POST /api/templates/import")[0].Body), &sent); err != nil || !strings.HasPrefix(sent, "name: base") {
		t.Fatalf("import body %q: %v (want a JSON string of the YAML)", fake.find("POST /api/templates/import")[0].Body, err)
	}
	if r["spec"].(map[string]interface{})["wizardable"] != true {
		t.Fatalf("parse_spec = %v", r["spec"])
	}
	if r["images"].(map[string]interface{})["manifest_version"] != "2026110101" || r["refresh"].(map[string]interface{})["updated"] != true {
		t.Fatalf("base images = %v / %v", r["images"], r["refresh"])
	}
	if len(r["caps"].([]interface{})) != 1 || len(r["handlers"].([]interface{})) != 1 {
		t.Fatalf("caps / handlers = %v / %v", r["caps"], r["handlers"])
	}
	if o := r["options"].([]interface{}); len(o) != 1 || o[0] != "P1" {
		t.Fatalf("field options = %v", o)
	}
	if c := fake.find("GET /api/plugins/field-handlers/plugin.jira.projects?query=P"); len(c) != 1 {
		t.Fatalf("field options request = %v", fake.find("GET /api/plugins/field-handlers/"))
	}

	_, run2 := libEnv(t, map[string]string{
		"GET /api/audit-logs/export": "csv:time,zone,actor,actor_type,event,details\n",
	}, nil)
	v, err := run2("import knot.audit as audit\nresult = audit.export(format='csv')")
	if err != nil || !strings.HasPrefix(v.(string), "time,zone,actor") {
		t.Fatalf("csv export = %v %v", v, err)
	}
}

func TestKnotFilesUploadsLib(t *testing.T) {
	const up = "/api/files/uploads/docs/big/file.bin"
	fake, run := libEnv(t, map[string]string{
		"POST " + up:                   `{"upload_id":"up1"}`,
		"PUT " + up:                    `{"number":1,"etag":"e1","size":3}`,
		"GET " + up:                    `{"parts":[{"number":1,"etag":"e1","size":3}]}`,
		"DELETE " + up:                 `{"status":true}`,
		"GET /api/files/share-targets": `{"users":[{"user_id":"u1","username":"alice","can_own":true,"self":true}],"groups":[{"group_id":"g1","name":"devs"}]}`,
		"POST /api/files/fsck":         `{"buckets":2,"files":9,"found":{"orphan":1},"repaired":{},"issues":[{"kind":"orphan","bucket":"b"}]}`,
	}, nil)

	r := mustRun(t, run, `
import knot.files as files
result = {}
uid = files.start_upload("docs", "big/file.bin", "application/zip")
result["part"] = files.upload_part("docs", "big/file.bin", uid, 1, b"abc")
result["parts"] = files.list_upload_parts("docs", "big/file.bin", uid)
files.abort_upload("docs", "big/file.bin", uid)
result["targets"] = files.share_targets()
result["fsck"] = files.fsck(repair=True)
`)
	start := fake.find("POST " + up)
	if len(start) != 1 || start[0].ContentType != "application/zip" {
		t.Fatalf("start upload = %v (want the file's content type sent)", start)
	}
	put := fake.find("PUT " + up)
	if len(put) != 1 || put[0].URI != up+"?upload_id=up1&part=1" || put[0].Body != "abc" {
		t.Fatalf("upload part = %v", put)
	}
	if r["part"].(map[string]interface{})["etag"] != "e1" || len(r["parts"].([]interface{})) != 1 {
		t.Fatalf("part / parts = %v / %v", r["part"], r["parts"])
	}
	if c := fake.find("DELETE " + up + "?upload_id=up1"); len(c) != 1 {
		t.Fatal("abort not sent")
	}
	tg := r["targets"].(map[string]interface{})
	if tg["users"].([]interface{})[0].(map[string]interface{})["id"] != "u1" || tg["groups"].([]interface{})[0].(map[string]interface{})["name"] != "devs" {
		t.Fatalf("share targets = %v", tg)
	}
	if fs := jsonBody(t, fake.find("POST /api/files/fsck")[0]); fs["repair"] != true || fs["deep"] != false {
		t.Fatalf("fsck body = %v", fs)
	}
	if r["fsck"].(map[string]interface{})["files"] != int64(9) {
		t.Fatalf("fsck = %v", r["fsck"])
	}

	// complete_upload with no parts given completes with the parts held.
	fake2, run2 := libEnv(t, map[string]string{
		"GET " + up:  `{"parts":[{"number":1,"etag":"e1","size":3},{"number":2,"etag":"e2","size":1}]}`,
		"POST " + up: `{"key":"big/file.bin","size":4,"etag":"z"}`,
	}, nil)
	v, err := run2(`
import knot.files as files
result = files.complete_upload("docs", "big/file.bin", "up1")
`)
	if err != nil || v.(map[string]interface{})["size"] != int64(4) {
		t.Fatalf("complete = %v %v", v, err)
	}
	done := fake2.find("POST " + up + "?upload_id=up1&complete=1")
	if len(done) != 1 {
		t.Fatalf("complete request = %v", fake2.calls)
	}
	parts := jsonBody(t, done[0])["parts"].([]interface{})
	if len(parts) != 2 || parts[1].(map[string]interface{})["etag"] != "e2" {
		t.Fatalf("complete parts = %v", parts)
	}
}
