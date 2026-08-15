package configwizard

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func postTOML(t *testing.T, handler http.HandlerFunc, toml string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/preview", strings.NewReader("toml="+toml))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func TestPreviewHandlerMergesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "knot.toml")
	existing := "[server]\nlisten = \"0.0.0.0:3000\"\n\n[server.base_image]\nmanifest = \"/m.json\"\n\n[my.custom]\nflag = true\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	generated := "[server]\nlisten = \"0.0.0.0:3000\"\nurl = \"https://knot.example.com\"\n"
	rec := postTOML(t, PreviewHandler(Options{TargetPath: path, AllowOverwrite: true}), generated)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"[server.base_image]", "[my.custom]", "url = \"https://knot.example.com\""} {
		if !strings.Contains(body, want) {
			t.Errorf("merged preview missing %s\n%s", want, body)
		}
	}

	// The file on disk must be untouched by a preview.
	after, err := os.ReadFile(path)
	if err != nil || string(after) != existing {
		t.Error("preview modified the config file")
	}
}

func TestPreviewHandlerNoExistingFile(t *testing.T) {
	dir := t.TempDir()
	generated := "[server]\nlisten = \"0.0.0.0:3000\"\n"
	rec := postTOML(t, PreviewHandler(Options{TargetPath: filepath.Join(dir, "knot.toml")}), generated)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Body.String() != generated {
		t.Errorf("preview without existing config should return the TOML unchanged, got:\n%s", rec.Body.String())
	}
}

func TestPreviewHandlerEmptyBody(t *testing.T) {
	rec := postTOML(t, PreviewHandler(Options{}), "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
