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

func TestSaveMergedHonoursDeletions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "knot.toml")
	required := "listen = \"0.0.0.0:3000\"\nlisten_agent = \"0.0.0.0:3010\"\nagent_endpoint = \"knot.example.com:3010\"\nurl = \"https://knot.example.com\"\ntimezone = \"Europe/London\"\nencrypt = \"testkey\"\n\n[server.badgerdb]\nenabled = true\npath = \"/tmp/knot-data\"\n"
	existing := "[server]\n" + required + "\n[server.base_image]\nmanifest = \"/m.json\"\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	deleted := "[server]\n" + required

	// merged=1: content is already the merged result — write verbatim.
	req := httptest.NewRequest("POST", "/save", strings.NewReader("toml="+deleted+"&merged=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	saveConfig(rec, req, path, true, Options{AllowOverwrite: true})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	after, _ := os.ReadFile(path)
	if strings.Contains(string(after), "base_image") {
		t.Errorf("merged=1 save should honour deletions, file still has base_image:\n%s", after)
	}

	// Without merged: the merge preserves sections absent from the content.
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest("POST", "/save", strings.NewReader("toml="+deleted))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	saveConfig(rec, req, path, true, Options{AllowOverwrite: true})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	after, _ = os.ReadFile(path)
	if !strings.Contains(string(after), "base_image") {
		t.Errorf("plain save should merge and preserve base_image:\n%s", after)
	}
}

// TestPreviewThenVerbatimSave mirrors the browser flow end to end: the
// review step fetches the merged preview, the user deletes a section in
// the editor, and the save carries merged=1 so the editor content is
// written verbatim.
func TestPreviewThenVerbatimSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "knot.toml")
	existing := "[server]\n" +
		"listen = \"0.0.0.0:3000\"\nlisten_agent = \"0.0.0.0:3010\"\nagent_endpoint = \"knot.example.com:3010\"\n" +
		"url = \"https://knot.example.com\"\ntimezone = \"Europe/London\"\nencrypt = \"testkey\"\n" +
		"\n[server.badgerdb]\nenabled = true\npath = \"/tmp/knot-data\"\n" +
		"\n[server.license]\nkey = \"PRO-KEY\"\nname = \"Test Co\"\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	generated := "[server]\n" +
		"listen = \"0.0.0.0:3000\"\nlisten_agent = \"0.0.0.0:3010\"\nagent_endpoint = \"knot.example.com:3010\"\n" +
		"url = \"https://knot.example.com\"\ntimezone = \"Europe/London\"\nencrypt = \"testkey\"\n" +
		"\n[server.badgerdb]\nenabled = true\npath = \"/tmp/knot-data\"\n"
	opts := Options{TargetPath: path, AllowOverwrite: true}

	// Browser: fetch merged preview.
	rec := postTOML(t, PreviewHandler(opts), generated)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview status = %d", rec.Code)
	}
	// User deletes the licence section in the editor (simulated by
	// regenerating from the preview minus that block).
	merged := rec.Body.String()
	if !strings.Contains(merged, "[server.license]") {
		t.Fatalf("preview should contain the existing license table:\n%s", merged)
	}
	edited := merged[:strings.Index(merged, "[server.license]")]

	// Browser: save with merged=1.
	req := httptest.NewRequest("POST", "/save", strings.NewReader("toml="+edited+"&merged=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srec := httptest.NewRecorder()
	saveConfig(srec, req, path, true, opts)
	if srec.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %q", srec.Code, srec.Body.String())
	}
	after, _ := os.ReadFile(path)
	if strings.Contains(string(after), "license") {
		t.Errorf("deleted license should stay deleted:\n%s", after)
	}
	if !strings.Contains(string(after), "[server.badgerdb]") {
		t.Errorf("managed tables must survive the verbatim save:\n%s", after)
	}
}
