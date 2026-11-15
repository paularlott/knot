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

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/methods"
	"github.com/paularlott/knot/internal/syncproject"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/object"
)

const skillID = "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"

// TestKnotSkillLib pins knot.skill against the API's real shapes: ids come
// back as "skill_id", create makes a user skill unless asked for a global
// one, update preserves fields it is not given, names resolve through
// GET /api/skill/{name}, and search returns a list of {skill, score}.
func TestKnotSkillLib(t *testing.T) {
	content := "---\nname: deploy\ndescription: How to deploy\n---\nSteps"
	details := `{"skill_id":"` + skillID + `","user_id":"u1","name":"deploy","description":"How to deploy","content":` + jsonString(content) + `,"groups":null,"zones":["z1"],"active":true,"is_managed":false}`
	fake, run := libEnv(t, map[string]string{
		"GET /api/skill":               `{"count":1,"skills":[{"skill_id":"` + skillID + `","user_id":"u1","name":"deploy","description":"How to deploy","groups":[],"zones":[],"active":true,"is_managed":false}]}`,
		"GET /api/skill/deploy":        details,
		"GET /api/skill/search":        `[{"skill":` + jsonString(content) + `,"score":0.9}]`,
		"POST /api/skill":              `{"status":true,"skill_id":"new-id"}`,
		"PUT /api/skill/" + skillID:    ``,
		"DELETE /api/skill/" + skillID: ``,
		"GET /api/users/whoami":        `{"user_id":"u1"}`,
	}, map[string]int{"POST /api/skill": 201})

	r := mustRun(t, run, `
import knot.skill as skill
result = {}
result["list"] = skill.list()
result["own"] = skill.list(owner="current")
result["get"] = skill.get("deploy")
result["user_id"] = skill.create("---\nname: a\ndescription: b\n---\n")
result["global_id"] = skill.create("---\nname: a\ndescription: b\n---\n", is_global=True)
skill.update("deploy", zones=["z2"])
skill.delete("deploy")
result["search"] = skill.search("deploy")
`)
	if l := r["list"].([]interface{}); len(l) != 1 || l[0].(map[string]interface{})["id"] != skillID {
		t.Fatalf("list = %v", l)
	}
	if c := fake.find("GET /api/skill?"); len(c) != 2 || !strings.Contains(c[1].URI, "user_id=u1") {
		t.Fatalf("list requests = %v", c)
	}
	if g := r["get"].(map[string]interface{}); g["id"] != skillID || g["content"] != content || g["active"] != true {
		t.Fatalf("get = %v", g)
	}
	if r["user_id"] != "new-id" || r["global_id"] != "new-id" {
		t.Fatalf("create returned %v / %v", r["user_id"], r["global_id"])
	}
	creates := fake.find("POST /api/skill")
	if len(creates) != 2 || jsonBody(t, creates[0])["user_id"] != "current" || jsonBody(t, creates[1])["user_id"] != "" {
		t.Fatalf("create bodies = %v", creates)
	}
	puts := fake.find("PUT /api/skill/" + skillID)
	if len(puts) != 1 {
		t.Fatalf("update did not PUT to the skill's UUID: %v", fake.calls)
	}
	body := jsonBody(t, puts[0])
	if body["active"] != true || body["content"] != content || body["zones"].([]interface{})[0] != "z2" {
		t.Fatalf("update body = %v", body)
	}
	if len(fake.find("DELETE /api/skill/"+skillID)) != 1 {
		t.Fatalf("delete did not hit the skill's UUID: %v", fake.calls)
	}
	if len(fake.find("GET /api/skill/name/")) != 0 {
		t.Fatal("library used the non-existent /api/skill/name route")
	}
	s := r["search"].([]interface{})
	if len(s) != 1 {
		t.Fatalf("search = %v", s)
	}
	m := s[0].(map[string]interface{})
	if m["id"] != skillID || m["name"] != "deploy" || m["description"] != "How to deploy" || m["score"] != 0.9 || m["content"] != content {
		t.Fatalf("search result = %v", m)
	}
}

// TestKnotSpacePortsAndJobsResolveNames pins that the /space-io endpoints,
// which need a UUID, are reached with the space's ID when given a name.
func TestKnotSpacePortsAndJobsResolveNames(t *testing.T) {
	fake, run := libEnv(t, map[string]string{
		"GET /api/spaces/web":                          `{"space_id":"` + spaceID + `","name":"web"}`,
		"GET /space-io/" + spaceID + "/port/list":      `{"forwards":[{"local_port":8080,"space":"db","remote_port":5432,"persistent":true,"mode":"direct"}]}`,
		"POST /space-io/" + spaceID + "/port/forward":  `{}`,
		"POST /space-io/" + spaceID + "/port/stop":     `{}`,
		"POST /space-io/" + spaceID + "/port/throttle": `{}`,
		"POST /space-io/" + spaceID + "/port/apply":    `{"applied":[],"stopped":[]}`,
		"POST /space-io/" + spaceID + "/jobs/run":      `{"success":true}`,
	}, nil)

	r := mustRun(t, run, `
import knot.space as space
import knot.jobs as jobs
result = {}
result["list"] = space.port_list("web")
space.port_forward("web", 8080, "db", 5432)
space.port_stop("web", 8080)
space.port_throttle("web", 8080, latency_ms=10)
space.port_apply("web", [])
result["run"] = jobs.run("web", "backup")
`)
	fwd := r["list"].([]interface{})[0].(map[string]interface{})
	if fwd["local_port"] != int64(8080) || fwd["persistent"] != true || fwd["mode"] != "direct" {
		t.Fatalf("port_list = %v", fwd)
	}
	for _, p := range []string{"GET /space-io/" + spaceID + "/port/list", "POST /space-io/" + spaceID + "/port/forward", "POST /space-io/" + spaceID + "/port/stop", "POST /space-io/" + spaceID + "/port/throttle", "POST /space-io/" + spaceID + "/port/apply", "POST /space-io/" + spaceID + "/jobs/run"} {
		if len(fake.find(p)) != 1 {
			t.Errorf("no request %s; calls %v", p, fake.calls)
		}
	}
	if len(fake.find("POST /space-io/web")) != 0 || len(fake.find("GET /space-io/web")) != 0 {
		t.Fatal("a space name reached /space-io")
	}
}

// TestKnotUserGroupUpdatePreserveQuotas pins that update() keeps the file
// storage quota and bucket limit, and accepts them as arguments.
func TestKnotUserGroupUpdatePreserveQuotas(t *testing.T) {
	fake, run := libEnv(t, map[string]string{
		"GET /api/users/alice": `{"user_id":"u2","username":"alice","email":"a@example.com","roles":[],"groups":[],"active":true,"max_spaces":3,"compute_units":4,"storage_units":5,"max_tunnels":6,"file_storage_mb":700,"max_buckets":8,"preferred_shell":"bash","timezone":"UTC"}`,
		"PUT /api/users/alice": ``,
		"GET /api/groups/g1":   `{"group_id":"g1","name":"devs","max_spaces":1,"compute_units":2,"storage_units":3,"max_tunnels":4,"file_storage_mb":500,"max_buckets":6}`,
		"PUT /api/groups/g1":   ``,
	}, nil)
	r := mustRun(t, run, `
import knot.user as user
import knot.group as group
user.update("alice", email="b@example.com")
user.update("alice", max_buckets=9, file_storage_mb=1000)
group.update("g1", name="devs2")
group.update("g1", max_buckets=7, file_storage_mb=900, max_tunnels=1)
result = {"user": user.get("alice"), "group": group.get("g1")}
`)
	puts := fake.find("PUT /api/users/alice")
	if b := jsonBody(t, puts[0]); b["file_storage_mb"] != 700.0 || b["max_buckets"] != 8.0 || b["max_tunnels"] != 6.0 || b["email"] != "b@example.com" {
		t.Fatalf("user update body = %v", b)
	}
	if b := jsonBody(t, puts[1]); b["file_storage_mb"] != 1000.0 || b["max_buckets"] != 9.0 {
		t.Fatalf("user update with quotas = %v", b)
	}
	gputs := fake.find("PUT /api/groups/g1")
	if b := jsonBody(t, gputs[0]); b["file_storage_mb"] != 500.0 || b["max_buckets"] != 6.0 || b["max_tunnels"] != 4.0 || b["name"] != "devs2" {
		t.Fatalf("group update body = %v", b)
	}
	if b := jsonBody(t, gputs[1]); b["file_storage_mb"] != 900.0 || b["max_buckets"] != 7.0 || b["max_tunnels"] != 1.0 {
		t.Fatalf("group update with quotas = %v", b)
	}
	if u := r["user"].(map[string]interface{}); u["file_storage_mb"] != int64(700) || u["max_buckets"] != int64(8) {
		t.Fatalf("user.get = %v", u)
	}
	if g := r["group"].(map[string]interface{}); g["file_storage_mb"] != int64(500) || g["max_buckets"] != int64(6) {
		t.Fatalf("group.get = %v", g)
	}
}

// TestKnotVarsUpdateKeepsProtectedValue pins that update() without value=
// leaves "value" out of the request (the server then keeps the stored
// value; reads blank a protected one), and sends it when given.
func TestKnotVarsUpdateKeepsProtectedValue(t *testing.T) {
	fake, run := libEnv(t, map[string]string{
		"GET /api/templatevars/secret": `{"templatevar_id":"v1","name":"secret","value":"","zones":null,"local":false,"protected":true,"restricted":false}`,
		"PUT /api/templatevars/secret": ``,
	}, nil)
	mustRun(t, run, `
import knot.vars as vars
vars.update("secret", restricted=True)
vars.set_value("secret", "new")
result = {}
`)
	puts := fake.find("PUT /api/templatevars/secret")
	first := jsonBody(t, puts[0])
	if _, sent := first["value"]; sent {
		t.Fatalf("update without value sent one: %v", first)
	}
	if first["protected"] != true || first["restricted"] != true {
		t.Fatalf("update body = %v", first)
	}
	if second := jsonBody(t, puts[1]); second["value"] != "new" {
		t.Fatalf("set_value body = %v", second)
	}
}

// TestKnotPoolMemberTCPRPS pins the member tcp_rps read from the API's
// "tcprps" key.
func TestKnotPoolMemberTCPRPS(t *testing.T) {
	_, run := libEnv(t, map[string]string{
		"GET /api/pools/web": `{"pool_id":"p1","name":"web","utilization":{"tcp_rps":3.5},"members":[{"space_id":"s1","name":"web-1","tcprps":2.5}]}`,
	}, nil)
	r := mustRun(t, run, `
import knot.pool as pool
result = pool.get("web")
`)
	m := r["members"].([]interface{})[0].(map[string]interface{})
	if m["tcp_rps"] != 2.5 {
		t.Fatalf("member = %v", m)
	}
	if r["utilization"].(map[string]interface{})["tcp_rps"] != 3.5 {
		t.Fatalf("utilization = %v", r["utilization"])
	}
}

// TestApiClientKeywordArgs pins that the Go transport honours body= on
// post/put and headers= on delete, as the Python transport does.
func TestApiClientKeywordArgs(t *testing.T) {
	type seen struct{ method, body, ifMatch string }
	var mu sync.Mutex
	var calls []seen
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		calls = append(calls, seen{r.Method, string(b), r.Header.Get("If-Match")})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	client, err := rest.NewClient(srv.URL, "token", true)
	if err != nil {
		t.Fatal(err)
	}
	env := scriptling.New()
	env.RegisterLibrary(GetApiClientLibrary(client, "u1"))
	res, err := env.EvalWithContext(context.Background(), `
import knot.apiclient as api
api.post("/x", body={"a": 1})
api.put("/x", body={"b": 2})
api.delete("/x", headers={"If-Match": "etag1"})
api.delete("/x", {"If-Match": "etag2"})
`)
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := res.(*object.Error); ok {
		t.Fatal(e.Message)
	}
	if len(calls) != 4 {
		t.Fatalf("calls = %v", calls)
	}
	if calls[0].method != "POST" || !strings.Contains(calls[0].body, `"a":1`) {
		t.Errorf("post body= dropped: %v", calls[0])
	}
	if calls[1].method != "PUT" || !strings.Contains(calls[1].body, `"b":2`) {
		t.Errorf("put body= dropped: %v", calls[1])
	}
	if calls[2].ifMatch != "etag1" || calls[3].ifMatch != "etag2" {
		t.Errorf("delete headers dropped: %v / %v", calls[2], calls[3])
	}
}

// TestEventAccessorDefaults pins that get_*(name, default=) honours the
// keyword form as well as the positional one.
func TestEventAccessorDefaults(t *testing.T) {
	env := scriptling.New()
	env.RegisterLibrary(GetEventLibrary())
	if err := env.SetObjectVar(EventParamsVarName, object.NewStringDict(map[string]object.Object{"present": object.NewString("yes")})); err != nil {
		t.Fatal(err)
	}
	_, err := env.EvalWithContext(context.Background(), `
import knot.event as e
result = [
    e.get_string("missing", default="d"),
    e.get_string("missing", "p"),
    e.get_int("missing", default=7),
    e.get_bool("missing", default=True),
    e.get_list("missing", default=[1]),
    e.get_dict("missing", default={"k": 1}),
    e.get_string("present", default="d"),
]
`)
	if err != nil {
		t.Fatal(err)
	}
	v, errObj := env.GetVar("result")
	if errObj != nil {
		t.Fatal(errObj)
	}
	got, _ := json.Marshal(v)
	if want := `["d","p",7,true,[1],{"k":1},"yes"]`; string(got) != want {
		t.Fatalf("accessors = %s, want %s", got, want)
	}
}

// TestMCPToolSearchPositionalMaxResults pins tool_search(q, 5) and
// call_tool(name=, arguments=).
func TestMCPToolSearchPositionalMaxResults(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]interface{}
		json.NewDecoder(r.Body).Decode(&m)
		mu.Lock()
		bodies = append(bodies, m)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set(build.VersionHeader, build.Version)
		w.Write([]byte(`{"content":[{"type":"text","text":"[]"}]}`))
	}))
	t.Cleanup(srv.Close)
	client, err := apiclient.NewClient(srv.URL, "token", true)
	if err != nil {
		t.Fatal(err)
	}
	env := scriptling.New()
	env.RegisterLibrary(GetMCPToolsLibrary(client, nil))
	res, err := env.EvalWithContext(context.Background(), `
import knot.mcp as mcp
mcp.tool_search("files", 5)
mcp.tool_search(query="files", max_results=3)
mcp.call_tool(name="echo", arguments={"x": 1})
`)
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := res.(*object.Error); ok {
		t.Fatal(e.Message)
	}
	if len(bodies) != 3 {
		t.Fatalf("bodies = %v", bodies)
	}
	maxOf := func(b map[string]interface{}) interface{} {
		return b["arguments"].(map[string]interface{})["max_results"]
	}
	if maxOf(bodies[0]) != 5.0 || maxOf(bodies[1]) != 3.0 {
		t.Fatalf("max_results = %v / %v", maxOf(bodies[0]), maxOf(bodies[1]))
	}
	if bodies[2]["name"] != "echo" || bodies[2]["arguments"].(map[string]interface{})["x"] != 1.0 {
		t.Fatalf("call_tool body = %v", bodies[2])
	}
}

// TestServerUnregisterByKeyword pins that unregister(name="x") removes
// only that method, like unregister("x").
func TestServerUnregisterByKeyword(t *testing.T) {
	var regs []methods.Registration
	unregAll := 0
	SetMethodsRegistrar(func(reg *methods.Registration) error {
		regs = append(regs, *reg)
		return nil
	})
	SetMethodsUnregisterAll(func() error { unregAll++; return nil })
	t.Cleanup(func() { SetMethodsRegistrar(nil); SetMethodsUnregisterAll(nil) })

	env := scriptling.New()
	env.RegisterLibrary(GetMethodsLibrary())
	env.RegisterLibrary(GetMethodsSchemaLibrary())
	res, err := env.EvalWithContext(context.Background(), `
from knot.methods import Server
srv = Server("python3", args=["app.py"])
srv.method("a")
srv.method("b")
srv.method("c")
srv.register()
srv.unregister(name="a")
srv.unregister("b")
`)
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := res.(*object.Error); ok {
		t.Fatal(e.Message)
	}
	if unregAll != 0 {
		t.Fatalf("unregister by name removed every method")
	}
	last := regs[len(regs)-1]
	if len(last.Methods) != 1 || last.Methods[0].Name != "c" {
		t.Fatalf("remaining methods = %v", last.Methods)
	}
}

type fakeSyncAgent struct {
	started []string
}

func (f *fakeSyncAgent) Start(content, name string) (string, error) {
	f.started = append(f.started, name)
	return name, nil
}
func (f *fakeSyncAgent) Stop(name string) error { return nil }
func (f *fakeSyncAgent) List() ([]syncproject.ProjectStatus, error) {
	var out []syncproject.ProjectStatus
	for _, n := range f.started {
		out = append(out, syncproject.ProjectStatus{Name: n})
	}
	return out, nil
}

// TestSyncStartPositionalArgs pins start(project, name, wait, timeout) and
// stop/status(name=) in positional and keyword forms.
func TestSyncStartPositionalArgs(t *testing.T) {
	agent := &fakeSyncAgent{}
	SetSyncProjectAgent(agent)
	t.Cleanup(func() { SetSyncProjectAgent(nil) })

	env := scriptling.New()
	env.RegisterLibrary(GetSyncLibrary(true))
	res, err := env.EvalWithContext(context.Background(), `
import knot.files.sync as sync
project = {"sync": [{"local": "/tmp/x", "remote": "b:f/"}]}
sync.start(project, "positional", False, 30)
sync.start(project=project, name="keyword", wait=False, timeout=30)
sync.status(name="keyword")
sync.stop(name="keyword")
`)
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := res.(*object.Error); ok {
		t.Fatal(e.Message)
	}
	if strings.Join(agent.started, ",") != "positional,keyword" {
		t.Fatalf("started = %v", agent.started)
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
