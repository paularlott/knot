package scriptling

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/extlibs/netsecurity"
	"github.com/paularlott/scriptling/extlibs/requests"
	"github.com/paularlott/scriptling/libloader"
	"github.com/paularlott/scriptling/object"
	"github.com/paularlott/scriptling/stdlib"
)

// TestKnotSpaceFileErrorsRaise pins that read_file and write_file raise when
// the agent answers {"success": false} (the server sends that with HTTP 200),
// rather than reading a missing file as "" or reporting a failed write as
// written.
func TestKnotSpaceFileErrorsRaise(t *testing.T) {
	_, run := libEnv(t, map[string]string{
		"POST /api/spaces/ok/files/read":   `{"success":true,"content":"hello"}`,
		"POST /api/spaces/ok/files/write":  `{"success":true}`,
		"POST /api/spaces/bad/files/read":  `{"success":false,"error":"open /nope: no such file or directory"}`,
		"POST /api/spaces/bad/files/write": `{"success":false,"error":"permission denied"}`,
	}, nil)

	r := mustRun(t, run, `
import knot.space as space
result = {}
result["read"] = space.read_file("ok", "/a.txt")
result["write"] = space.write_file("ok", "/a.txt", "x")
try:
    space.read_file("bad", "/nope")
    result["read_err"] = ""
except Exception as e:
    result["read_err"] = str(e)
try:
    space.write_file("bad", "/ro", "x")
    result["write_err"] = ""
except Exception as e:
    result["write_err"] = str(e)
`)
	if r["read"] != "hello" || r["write"] != true {
		t.Fatalf("successful read/write = %v / %v", r["read"], r["write"])
	}
	if !strings.Contains(r["read_err"].(string), "no such file") {
		t.Fatalf("read_file of a missing file did not raise: %q", r["read_err"])
	}
	if !strings.Contains(r["write_err"].(string), "permission denied") {
		t.Fatalf("failed write_file did not raise: %q", r["write_err"])
	}
}

// TestKnotSpaceRunReportsFailure pins space.run: the output string on
// success, an exception carrying the error and exit code on failure, and a
// dict either way with check=False.
func TestKnotSpaceRunReportsFailure(t *testing.T) {
	_, run := libEnv(t, map[string]string{
		"POST /api/spaces/ok/run-command":   `{"success":true,"output":"hi\n","exit_code":0}`,
		"POST /api/spaces/bad/run-command":  `{"success":false,"output":"boom\n","exit_code":3,"error":"exit status 3"}`,
		"POST /api/spaces/slow/run-command": `{"success":false,"output":"","exit_code":124,"error":"Command timed out"}`,
	}, nil)

	r := mustRun(t, run, `
import knot.space as space
result = {}
result["ok"] = space.run("ok", "echo hi")
try:
    space.run("bad", "false")
    result["bad_err"] = ""
except Exception as e:
    result["bad_err"] = str(e)
try:
    space.run("slow", "sleep 99", timeout=1)
    result["slow_err"] = ""
except Exception as e:
    result["slow_err"] = str(e)
result["ok_dict"] = space.run("ok", "echo hi", check=False)
result["bad_dict"] = space.run("bad", "false", check=False)
`)
	if r["ok"] != "hi\n" {
		t.Fatalf("run output = %q", r["ok"])
	}
	if e := r["bad_err"].(string); !strings.Contains(e, "exit code 3") || !strings.Contains(e, "exit status 3") || !strings.Contains(e, "boom") {
		t.Fatalf("failed run raised %q", e)
	}
	if e := r["slow_err"].(string); !strings.Contains(e, "exit code 124") || !strings.Contains(e, "timed out") {
		t.Fatalf("timed out run raised %q", e)
	}
	ok := r["ok_dict"].(map[string]interface{})
	if ok["output"] != "hi\n" || ok["success"] != true || ok["exit_code"] != int64(0) || ok["error"] != "" {
		t.Fatalf("check=False success = %v", ok)
	}
	bad := r["bad_dict"].(map[string]interface{})
	if bad["output"] != "boom\n" || bad["success"] != false || bad["exit_code"] != int64(3) || bad["error"] != "exit status 3" {
		t.Fatalf("check=False failure = %v", bad)
	}
}

// TestKnotTemplateRuntimeAvailable pins that get() and list() report the
// server's runtime_available instead of assuming true.
func TestKnotTemplateRuntimeAvailable(t *testing.T) {
	_, run := libEnv(t, map[string]string{
		"GET /api/templates":     `{"count":1,"templates":[{"template_id":"t1","name":"vm","active":true,"runtime_available":false}]}`,
		"GET /api/templates/vm":  `{"template_id":"t1","name":"vm","active":true,"runtime_available":false}`,
		"GET /api/templates/old": `{"template_id":"t2","name":"old","active":true}`,
	}, nil)

	r := mustRun(t, run, `
import knot.template as template
result = {}
result["list"] = template.list()[0]["runtime_available"]
result["get"] = template.get("vm")["runtime_available"]
result["old"] = template.get("old")["runtime_available"]
`)
	if r["list"] != false || r["get"] != false {
		t.Fatalf("runtime_available list=%v get=%v, want false", r["list"], r["get"])
	}
	if r["old"] != true {
		t.Fatalf("a server without the field should read as available, got %v", r["old"])
	}
}

// TestKnotSpaceUnshareEncodesUser pins that unshare URL-encodes user_id.
func TestKnotSpaceUnshareEncodesUser(t *testing.T) {
	fake, run := libEnv(t, map[string]string{
		"DELETE /api/spaces/web/share": ``,
	}, nil)

	mustRun(t, run, `
import knot.space as space
result = {"ok": space.unshare("web", "a b&c=d")}
`)
	calls := fake.find("DELETE /api/spaces/web/share")
	if len(calls) != 1 || !strings.HasSuffix(calls[0].URI, "?user_id=a%20b%26c%3Dd") {
		t.Fatalf("unshare requests = %v", calls)
	}
}

// TestPythonApiClientGetEmptyBody pins the standalone (Python) knot.apiclient:
// get() returns None for an empty 2xx body, like post/put/delete.
func TestPythonApiClientGetEmptyBody(t *testing.T) {
	fake := &fakeAPI{responses: map[string]string{
		"GET /api/empty": ``,
		"GET /api/full":  `{"a":1}`,
	}}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)

	env := scriptling.New()
	stdlib.RegisterAll(env)
	requests.Register(env, &netsecurity.Config{AllowAll: true})
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

	res, err := env.EvalWithContext(context.Background(), `
import knot.apiclient as api
api.configure("`+srv.URL+`", "token")
result = {"empty": api.get("/api/empty"), "full": api.get("/api/full")}
`)
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := res.(*object.Error); ok {
		t.Fatal(e.Message)
	}
	v, errObj := env.GetVar("result")
	if errObj != nil {
		t.Fatal(errObj)
	}
	m := v.(map[string]interface{})
	if m["empty"] != nil {
		t.Fatalf("get() of an empty body = %v, want None", m["empty"])
	}
	if full, ok := m["full"].(map[string]interface{}); !ok || full["a"] != int64(1) {
		t.Fatalf("get() of a JSON body = %v", m["full"])
	}
}
