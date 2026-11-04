package scriptling

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/object"
)

// A script's call waits as long as its timeout says, past the REST client's
// own short timeout, and no longer; one with no timeout keeps the client's.
func TestApiClientTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	client, err := rest.NewClient(srv.URL, "token", true)
	if err != nil {
		t.Fatal(err)
	}
	client.SetTimeout(100 * time.Millisecond)

	run := func(script string) (string, error) {
		env := scriptling.New()
		env.RegisterLibrary(GetApiClientLibrary(client, "u1"))
		res, err := env.EvalWithContext(context.Background(), "import knot.apiclient as api\n"+script)
		if err != nil {
			return "", err
		}
		if e, ok := res.(*object.Error); ok {
			return "", errorString(e.Message)
		}
		return res.Inspect(), nil
	}

	cases := []struct {
		name, script string
		ok           bool
	}{
		{"get past the client's timeout", `api.get("/slow", timeout=5)["ok"]`, true},
		{"get with params and timeout", `api.get("/slow", {"a": "b"}, 5)["ok"]`, true},
		{"post past the client's timeout", `api.post("/slow", {}, timeout=5)["ok"]`, true},
		{"get with no timeout keeps the client's", `api.get("/slow")`, false},
		{"get with a null timeout keeps the client's", `api.get("/slow", None, None)`, false},
		{"put bounded by its timeout", `api.put("/slow", {}, timeout=0.2)`, false},
		{"delete past the client's timeout", `api.delete("/slow", timeout=5)["ok"]`, true},
		{"get_bytes bounded by its timeout", `api.get_bytes("/slow", timeout=0.1)`, false},
		{"put_bytes past the client's timeout", `api.put_bytes("/slow", "x", timeout=5)["ok"]`, true},
		{"a timeout that is not a number", `api.get("/slow", timeout="soon")`, false},
		{"a timeout of zero", `api.get("/slow", timeout=0)`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := run(c.script)
			if c.ok && (err != nil || !strings.Contains(out, "True")) {
				t.Fatalf("got %q %v", out, err)
			}
			if !c.ok && err == nil {
				t.Fatalf("succeeded with %q", out)
			}
		})
	}
}

type errorString string

func (e errorString) Error() string { return string(e) }
