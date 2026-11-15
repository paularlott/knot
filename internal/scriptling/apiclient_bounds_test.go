package scriptling

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/object"
)

// prechecked is a REST client that refuses every call by method and path,
// counting what reached it.
type prechecked struct {
	rest.RESTClient
	checked []string
}

func (p *prechecked) PrecheckRequest(method, path string) error {
	p.checked = append(p.checked, method+" "+path)
	return errors.New("refused by policy")
}

func runAPIScript(t *testing.T, client rest.RESTClient, script string) (object.Object, error) {
	t.Helper()
	env := scriptling.New()
	env.RegisterLibrary(GetApiClientLibrary(client, "u1"))
	res, err := env.EvalWithContext(context.Background(), "import knot.apiclient as api\n"+script)
	if err != nil {
		return nil, err
	}
	if e, ok := res.(*object.Error); ok {
		return nil, errorString(e.Message)
	}
	return res, nil
}

// A client that refuses a call is asked before the body, headers or query
// parameters are converted, so a refused call costs nothing to build.
func TestApiClientPrecheckBeforeConversion(t *testing.T) {
	inner, err := rest.NewClient("http://127.0.0.1:1", "t", true)
	if err != nil {
		t.Fatal(err)
	}
	client := &prechecked{RESTClient: inner}
	big := "a = [0] * 1000\nb = [a] * 1000\nc = [b] * 2000\n"
	for _, call := range []string{
		`api.post("/api/x", c)`,
		`api.put("/api/x", c)`,
		`api.post("/api/x", None, None, {"h": c})`,
		`api.delete("/api/x", {"h": c})`,
		`api.get("/api/x", {"q": c})`,
	} {
		if _, err := runAPIScript(t, client, big+call); err == nil || !strings.Contains(err.Error(), "refused by policy") {
			t.Errorf("%s: %v", call, err)
		}
	}
	if len(client.checked) != 5 {
		t.Errorf("prechecks = %v", client.checked)
	}
}

// A body, headers or parameters that share references are bounded by what
// they would encode to, whoever the client is.
func TestApiClientBoundsSharedValues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("%s %s reached the server", r.Method, r.URL.Path)
	}))
	defer srv.Close()
	client, err := rest.NewClient(srv.URL, "t", true)
	if err != nil {
		t.Fatal(err)
	}
	big := "a = [0] * 1000\nb = [a] * 1000\nc = [b] * 20\nd = [\"x\" * 1000] * 1000\n"
	for _, call := range []string{
		`api.post("/api/x", c)`,
		`api.put("/api/x", c)`,
		`api.post("/api/x", {}, None, {"h": d})`,
		`api.delete("/api/x", {"h": d})`,
		`api.get("/api/x", {"q": [d] * 2})`,
	} {
		if _, err := runAPIScript(t, client, big+call); err == nil || !strings.Contains(err.Error(), "larger than") {
			t.Errorf("%s: %v", call, err)
		}
	}
}

// get() and put_bytes() return None for an empty 2xx body, as post, put
// and delete do.
func TestApiClientEmptyBodies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	client, err := rest.NewClient(srv.URL, "t", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range []string{
		`api.get("/api/x") is None`,
		`api.put_bytes("/api/x", "data") is None`,
		`api.post("/api/x", {}) is None`,
		`api.delete("/api/x") is None`,
	} {
		res, err := runAPIScript(t, client, call)
		if err != nil || res.Inspect() != "True" {
			t.Errorf("%s: %v %v", call, res, err)
		}
	}
}
