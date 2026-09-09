package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExports pins the declared export modules ([tool.knot] export): each
// path is read (peer-first, disk-second — same as assets), linted once at
// load with a broken module failing the plugin loudly, and stored for user
// tool environments to materialize — every module as
// plugin.<name>.<stem>, the first also aliased as plugin.<name>.
func TestExports(t *testing.T) {
	// Two modules: both stored in declaration order.
	dir := t.TempDir()
	writePlugin(t, dir, "good", `
[tool.knot]
version = "1.0.0"
export = ["client.py", "format.py"]`)
	if err := os.WriteFile(filepath.Join(dir, "good", "client.py"), []byte("import plugin.good.format as fmt\nclass Api:\n    def ping(self):\n        return fmt.tidy(\"pong\")\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "good", "format.py"), []byte("def tidy(word):\n    return word.strip()\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	registry, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := registry.ByName("good")
	if p == nil {
		t.Fatalf("good failed: %+v", registry.Failed())
	}
	if len(p.Exports) != 2 || p.Exports[0].Path != "client.py" || p.Exports[1].Path != "format.py" {
		t.Fatalf("exports = %+v", p.Exports)
	}
	if !strings.Contains(p.Exports[0].Source, "class Api:") || !strings.Contains(p.Exports[1].Source, "def tidy") {
		t.Errorf("export sources not stored: %+v", p.Exports)
	}

	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{"broken", "[tool.knot]\nversion = \"1.0.0\"\nexport = [\"client.py\"]", "export"},
		{"wrongtype", "[tool.knot]\nversion = \"1.0.0\"\nexport = \"client.py\"", "list"},
		{"nonpy", "[tool.knot]\nversion = \"1.0.0\"\nexport = [\"client.txt\"]", ".py"},
		{"dupstem", "[tool.knot]\nversion = \"1.0.0\"\nexport = [\"a/client.py\", \"b/client.py\"]", "duplicate"},
		{"badstem", "[tool.knot]\nversion = \"1.0.0\"\nexport = [\"my-client.py\"]", "module name"},
	} {
		name, body, want := tc.name, tc.body, tc.want
		dir = t.TempDir()
		writePlugin(t, dir, name, body)
		switch name {
		case "broken":
			os.WriteFile(filepath.Join(dir, name, "client.py"), []byte("def oops(:\n"), 0o644)
		case "nonpy":
			os.WriteFile(filepath.Join(dir, name, "client.txt"), []byte("x\n"), 0o644)
		case "dupstem":
			for _, sub := range []string{"a", "b"} {
				os.MkdirAll(filepath.Join(dir, name, sub), 0o755)
				os.WriteFile(filepath.Join(dir, name, sub, "client.py"), []byte("x = 1\n"), 0o644)
			}
		case "badstem":
			os.WriteFile(filepath.Join(dir, name, "my-client.py"), []byte("x = 1\n"), 0o644)
		}
		registry, _ = Load(dir)
		var reason string
		for _, f := range registry.Failed() {
			if f.Name == name {
				reason = f.Reason
			}
		}
		if reason == "" {
			t.Errorf("%s: should fail, loaded %+v", name, registry.All())
			continue
		}
		if !strings.Contains(reason, want) {
			t.Errorf("%s: failure %q should mention %q", name, reason, want)
		}
	}

	// Declared but missing on disk (and no peer): an error, like any other
	// declared asset.
	dir = t.TempDir()
	writePlugin(t, dir, "missing", `
[tool.knot]
version = "1.0.0"
export = ["client.py"]`)
	registry, _ = Load(dir)
	found := false
	for _, f := range registry.Failed() {
		if f.Name == "missing" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing export module should fail the plugin, got %+v", registry.All())
	}
}
