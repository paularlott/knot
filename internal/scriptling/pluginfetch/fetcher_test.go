package pluginfetch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
)

func mockAPIServer(t *testing.T, handlers map[string]func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		if handler, ok := handlers[key]; ok {
			if r.Header.Get("Authorization") != "Bearer test-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			handler(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

func jsonBody(t *testing.T, v interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func newTestClient(t *testing.T, ts *httptest.Server) *apiclient.ApiClient {
	t.Helper()
	client, err := apiclient.NewClient(ts.URL, "test-token", true)
	if err != nil {
		t.Fatal(err)
	}
	client.SetContentType("application/json")
	return client
}

func TestFetcherServesEmbeddedLibs(t *testing.T) {
	ts := mockAPIServer(t, nil)
	defer ts.Close()
	f := NewFetcher(newTestClient(t, ts))

	modules := embeddedModuleNames()
	if len(modules) < 15 {
		t.Fatalf("expected 15+ embedded modules, got %d", len(modules))
	}
	for _, mod := range modules {
		data, err := f.Read(context.Background(), "knot://libs", "lib/knot/"+mod+".py")
		if err != nil {
			t.Errorf("Read(knot/%s.py): %v", mod, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("knot/%s.py is empty", mod)
		}
	}

	// The apiclient variant routes through the plugin.
	data, err := f.Read(context.Background(), "knot://libs", "lib/knot/apiclient.py")
	if err != nil {
		t.Fatalf("Read(apiclient variant): %v", err)
	}
	if !strings.Contains(string(data), "plugin.knot") {
		t.Fatal("expected the plugin-transport apiclient variant, got the HTTP one")
	}
}

func TestFetcherServesUserLibs(t *testing.T) {
	ts := mockAPIServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /api/scripts/name/mylib/lib": func(w http.ResponseWriter, r *http.Request) {
			w.Write(jsonBody(t, "def value():\n    return 'from server'\n"))
		},
		"GET /api/scripts": func(w http.ResponseWriter, r *http.Request) {
			w.Write(jsonBody(t, map[string]interface{}{
				"count": 1,
				"scripts": []map[string]interface{}{
					{"script_id": "1", "user_id": "", "name": "mylib", "script_type": "lib", "active": true},
				},
			}))
		},
	})
	defer ts.Close()
	f := NewFetcher(newTestClient(t, ts))

	data, err := f.Read(context.Background(), "knot://libs", "lib/mylib.py")
	if err != nil {
		t.Fatalf("Read(lib/mylib.py): %v", err)
	}
	if !strings.Contains(string(data), "from server") {
		t.Fatalf("expected server content, got %q", data)
	}

	entries, err := f.Tree(context.Background())
	if err != nil {
		t.Fatalf("Tree: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Name == "lib/mylib.py" && !e.IsDir {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected lib/mylib.py in the tree, got %v", entries)
	}
}

func TestFetcherServesScripts(t *testing.T) {
	ts := mockAPIServer(t, map[string]func(w http.ResponseWriter, r *http.Request){
		"GET /api/scripts/name/myscript/script": func(w http.ResponseWriter, r *http.Request) {
			w.Write(jsonBody(t, "import knot.space\nprint('hello')\n"))
		},
	})
	defer ts.Close()
	f := NewFetcher(newTestClient(t, ts))

	data, err := f.Read(context.Background(), "knot://myscript", "")
	if err != nil {
		t.Fatalf("Read(knot://myscript): %v", err)
	}
	if !strings.Contains(string(data), "knot.space") {
		t.Fatalf("expected script content, got %q", data)
	}
}

func TestFetcherNotFound(t *testing.T) {
	ts := mockAPIServer(t, nil)
	defer ts.Close()
	f := NewFetcher(newTestClient(t, ts))

	for _, tc := range []struct{ source, path string }{
		{"knot://nonexistent", ""},
		{"knot://libs", "lib/nosuchlib.py"},
		{"knot://libs", "lib/knot/nosuchmodule.py"},
		{"knot://libs", "notunderlib.txt"},
	} {
		if _, err := f.Read(context.Background(), tc.source, tc.path); err == nil {
			t.Errorf("Read(%s, %s): expected error", tc.source, tc.path)
		}
	}
}
