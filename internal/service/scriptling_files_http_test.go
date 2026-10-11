package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/extlibs"
)

// TestKnotFilesOverHTTPTransport runs the embedded knot.apiclient and
// knot.files python libraries, not the Go transport, against a live HTTP
// server, configured the way a standalone script is: environment variables.
// A text body must arrive as UTF-8 through the whole python stack. The
// explicit encode in put_bytes also covers standalone CPython, whose
// http.client would otherwise send a str body as latin-1; the interpreter
// path pinned here cannot tell the two apart.
func TestKnotFilesOverHTTPTransport(t *testing.T) {
	files := map[string][]byte{}
	types := map[string]string{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/files/objects/{bucket}/{key...}", func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[r.PathValue("key")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if ct := types[r.PathValue("key")]; ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		w.Write(body)
	})
	mux.HandleFunc("PUT /api/files/objects/{bucket}/{key...}", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		key := r.PathValue("key")
		files[key] = body
		types[key] = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"key": key, "size": len(body), "etag": "etag", "sha256": "sha",
			"content_type": r.Header.Get("Content-Type"), "modified_at": "2026-10-05T00:00:00Z",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	t.Setenv("KNOT_URL", srv.URL)
	t.Setenv("KNOT_TOKEN", "test-token")
	t.Setenv("KNOT_INSECURE", "true")

	netPolicy, err := serverNetConfig()
	if err != nil {
		t.Fatal(err)
	}
	env := scriptling.New()
	env.EnableOutputCapture()
	registerBaseLibraries(env, nil, netPolicy)
	// The python transports read KNOT_URL and friends from the environment.
	extlibs.RegisterOSLibrary(env, nil)
	env.SetLibraryLoader(newKnotLibsLoader())

	script := `
import knot.files
info = knot.files.write_file("scripts", "app/héllo ü.txt", "héllo ü 日本語")
result = {
    "text": knot.files.read_text("scripts", "app/héllo ü.txt"),
    "roundtrip": knot.files.read_file("scripts", "app/héllo ü.txt").decode() == "héllo ü 日本語",
    "type": info["content_type"],
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
	if m["text"] != "héllo ü 日本語" || m["roundtrip"] != true {
		t.Errorf("round trip failed: %v", m)
	}
	if m["type"] != "text/plain; charset=utf-8" {
		t.Errorf("content type = %v", m["type"])
	}
	// The server must have received the exact UTF-8 bytes, not latin-1.
	if got := string(files["app/héllo ü.txt"]); got != "héllo ü 日本語" {
		t.Fatalf("server stored %q", got)
	}
	if ct := types["app/héllo ü.txt"]; ct != "text/plain; charset=utf-8" {
		t.Fatalf("content type sent was %q", ct)
	}
}
