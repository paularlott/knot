package plugins

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/paularlott/scriptling/plugin"
)

// writePlugin creates a folder plugin with the given metadata block content.
func writePlugin(t *testing.T, dir, name, metadataBody string) string {
	t.Helper()
	pluginDir := filepath.Join(dir, name)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Every metadata line must be a comment; prefix any that are not so the
	// case bodies can be written as plain TOML.
	var lines []string
	for _, line := range strings.Split(metadataBody, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		} else {
			lines = append(lines, "# "+line)
		}
	}
	source := "# /// script\n" + strings.Join(lines, "\n") + "\n# ///\n\nprint('hi')\n"
	if err := os.WriteFile(filepath.Join(pluginDir, "main.py"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	return pluginDir
}

func goodMetadata() string {
	return `# requires-scriptling = ">=0.1"
#
# [tool.knot]
# version = "1.0.0"
# description = "test plugin"
# permissions = ["read", "write"]
#
# [[tool.knot.menus]]
# label = "Dash"
# url = "https://example.com"
# permission = "read"`
}

func TestScanCandidates(t *testing.T) {
	dir := t.TempDir()

	// Folder plugin, single-file plugin, junk.
	writePlugin(t, dir, "folder-plugin", goodMetadata())
	if err := os.WriteFile(filepath.Join(dir, "single.py"), []byte("# /// script\n# [tool.knot]\n# version = \"1.0\"\n# ///\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Folder without main.py and an invalid-name folder are ignored.
	if err := os.MkdirAll(filepath.Join(dir, "noentry"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "Bad_Name"), 0o755); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	candidates, warnings := scanCandidates(dir, entries)
	if len(candidates) != 1 {
		t.Fatalf("candidates = %v, want 1 (folders only)", candidates)
	}
	if candidates[0].name != "folder-plugin" {
		t.Errorf("candidates[0] = %+v", candidates[0])
	}
	if !slices.Contains(warnings, "file single.py is not a plugin — plugins are folders with a main.py; ignored") {
		t.Errorf("warnings = %v, want the loose file named", warnings)
	}
	if len(warnings) < 2 {
		t.Errorf("warnings = %v, want the no-main.py and invalid-name folders warned", warnings)
	}

	// A file and folder claiming the same name: the folder wins.
	// A loose file next to a folder of the same name is simply ignored
	// (with its own warning) — folders are the only plugin shape.
	dir2 := t.TempDir()
	writePlugin(t, dir2, "dupe", goodMetadata())
	if err := os.WriteFile(filepath.Join(dir2, "dupe.py"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries2, _ := os.ReadDir(dir2)
	candidates2, warnings2 := scanCandidates(dir2, entries2)
	if len(candidates2) != 1 || candidates2[0].name != "dupe" {
		t.Fatalf("candidates2 = %+v", candidates2)
	}
	if len(warnings2) != 1 {
		t.Errorf("warnings2 = %v, want the loose file warned once", warnings2)
	}
}

func TestLoadValidatesMetadata(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"no tool.knot", "# requires-scriptling = \">=0.1\"\n", "[tool.knot]"},
		{"unknown key", "# [tool.knot]\nversion = \"1\"\nfrobnicate = true\n", "unknown key"},
		{"bad permission id", "# [tool.knot]\nversion = \"1\"\npermissions = [\"Not Valid\"]\n", "[a-z0-9_]+"},
		{"menu unknown key", "# [tool.knot]\nversion = \"1\"\n\n[[tool.knot.menus]]\nlabel = \"x\"\nurl = \"/x\"\nwat = 1\n", "unknown key"},
		{"menu missing label", "# [tool.knot]\nversion = \"1\"\n\n[[tool.knot.menus]]\nurl = \"/x\"\n", "label is required"},
		{"menu bad url", "# [tool.knot]\nversion = \"1\"\n\n[[tool.knot.menus]]\nlabel = \"x\"\nurl = \"ftp://x\"\n", "must start with"},
		{"menu undeclared permission", "# [tool.knot]\nversion = \"1\"\npermissions = [\"read\"]\n\n[[tool.knot.menus]]\nlabel = \"x\"\nurl = \"/x\"\npermission = \"other\"\n", "not declared"},
		{"logo escape", "# [tool.knot]\nversion = \"1\"\nlogo_light = \"../../etc/passwd\"\nlogo_dark = \"../../etc/passwd\"\n", "inside the plugin folder"},
		{"site_logo unknown key", "# [tool.knot]\nversion = \"1\"\nsite_logo = true\n", "unknown key"},
		{"use_logo unknown key", "# [tool.knot]\nversion = \"1\"\n\n[[tool.knot.menus]]\nlabel = \"x\"\nurl = \"/x\"\nuse_logo = true\n", "unknown key"},
		{"menu unknown icon name", "# [tool.knot]\nversion = \"1\"\n\n[[tool.knot.menus]]\nlabel = \"x\"\nurl = \"/x\"\nicon = \"scripts\"\n", "icon"},
		{"page reserved path", "# [tool.knot]\nversion = \"1\"\n\n[[tool.knot.pages]]\npath = \"/assets/logo.svg\"\nhandler = \"h\"\n", "reserved"},
		{"page missing handler", "# [tool.knot]\nversion = \"1\"\n\n[[tool.knot.pages]]\npath = \"/x\"\n", "handler is required"},
		{"page menu key gone", "# [tool.knot]\nversion = \"1\"\n\n[[tool.knot.pages]]\npath = \"/x\"\nhandler = \"h\"\nmenu = true\n", "unknown key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writePlugin(t, dir, "test-plugin", tc.body)
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			candidates, _ := scanCandidates(dir, entries)
			if len(candidates) != 1 {
				t.Fatalf("candidates = %+v", candidates)
			}
			_, _, err = loadPlugin(candidates[0], func() *plugin.Manager { return nil })
			if err == nil {
				t.Fatalf("loadPlugin accepted %q", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestLoadGoodPlugin(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "metrics", goodMetadata())

	registry, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Failed()) != 0 {
		t.Fatalf("failed = %+v", registry.Failed())
	}
	all := registry.All()
	if len(all) != 1 || all[0].Name != "metrics" {
		t.Fatalf("plugins = %+v", all)
	}
	p := all[0]
	if p.Version != "1.0.0" || len(p.Permissions) != 2 || len(p.Menus) != 1 {
		t.Errorf("plugin = %+v", p)
	}
	if p.Permissions[0].Id != "plugin.metrics.read" || p.Permissions[1].Id != "plugin.metrics.write" {
		t.Errorf("permissions = %+v", p.Permissions)
	}
	if p.Menus[0].Permission != "plugin.metrics.read" {
		t.Errorf("menu permission = %q", p.Menus[0].Permission)
	}
	registry.Close()
}

func TestLoadFailedPluginRecordedNotFatal(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "good", goodMetadata())
	writePlugin(t, dir, "bad", "# [tool.knot]\nversion = \"1\"\npermissions = [\"x!\"]\n")

	registry, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.All()) != 1 || registry.All()[0].Name != "good" {
		t.Fatalf("plugins = %+v", registry.All())
	}
	if len(registry.Failed()) != 1 || registry.Failed()[0].Name != "bad" {
		t.Fatalf("failed = %+v", registry.Failed())
	}
	registry.Close()
}

func TestLoadEmptyAndMissingPath(t *testing.T) {
	if r, err := Load(""); err != nil || r != nil {
		t.Errorf("Load(\"\") = %v, %v; want nil, nil", r, err)
	}
	if r, err := Load(filepath.Join(t.TempDir(), "missing")); err != nil || r != nil {
		t.Errorf("Load(missing) = %v, %v; want nil, nil", r, err)
	}
}

// TestLooseFileIsNotPlugin pins the folders-only rule: a loose .py in the
// plugins root is ignored with a warning — plugins are folders with a
// main.py, so peers and assets always have a home.
func TestLooseFileIsNotPlugin(t *testing.T) {
	dir := t.TempDir()
	source := "# /// script\n# requires-scriptling = \">=0.1\"\n#\n# [tool.knot]\n# version = \"2.0\"\n# ///\n"
	if err := os.WriteFile(filepath.Join(dir, "tiny.py"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	registry, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	if len(registry.All()) != 0 {
		t.Fatalf("plugins = %+v, want none (loose files are not plugins)", registry.All())
	}
	if len(registry.Warnings()) == 0 {
		t.Error("expected a warning naming the ignored file")
	}
}

func TestResolveBinPeers(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	exec := func(name string) {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	host := runtime.GOOS + "_" + runtime.GOARCH
	// Real tokens that do not match this host, so they group as variants of
	// the same peer and are skipped (unknown tokens would read as bare
	// binaries instead — that is what TestStripVariant covers).
	otherOS := "linux"
	if runtime.GOOS == "linux" {
		otherOS = "darwin"
	}
	otherArch := "amd64"
	if runtime.GOARCH == "amd64" {
		otherArch = "arm64"
	}
	other := otherOS + "_" + otherArch
	otherArchOnly := otherArch
	if otherArchOnly == runtime.GOARCH {
		otherArchOnly = "riscv64"
	}

	// Universal bundle: only this host's variant is picked.
	exec("peer_a_" + host)
	exec("peer_a_" + other)
	// Platform bundle: arch-only variant for this host plus one that is not.
	exec("peer_b_" + runtime.GOARCH)
	exec("peer_b_" + otherArchOnly)
	// Bare binary.
	exec("peer_c")
	// Malformed: bare + variants together.
	exec("peer_d")
	exec("peer_d_" + host)
	// Not executable: warning.
	if err := os.WriteFile(filepath.Join(binDir, "peer_e_"+host), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	peers, warnings := resolveBinPeers(binDir)
	if len(peers) != 4 {
		t.Fatalf("peers = %+v, want 4", peers)
	}
	want := map[string]string{
		"peer_a": "peer_a_" + host,
		"peer_b": "peer_b_" + runtime.GOARCH,
		"peer_c": "peer_c",
		"peer_d": "peer_d", // bare wins the malformed mix
	}
	for _, peer := range peers {
		if want[peer.name] == "" {
			t.Errorf("unexpected peer %+v", peer)
			continue
		}
		if filepath.Base(peer.path) != want[peer.name] {
			t.Errorf("peer %s resolved to %s, want %s", peer.name, filepath.Base(peer.path), want[peer.name])
		}
	}
	mixedWarn, nonExecWarn := false, false
	for _, w := range warnings {
		if strings.Contains(w, "malformed") {
			mixedWarn = true
		}
		if strings.Contains(w, "not executable") {
			nonExecWarn = true
		}
	}
	if !mixedWarn {
		t.Errorf("warnings = %v, want malformed-package warning", warnings)
	}
	if !nonExecWarn {
		t.Errorf("warnings = %v, want not-executable warning", warnings)
	}
}

func TestStripVariant(t *testing.T) {
	cases := []struct {
		file string
		base string
		kind variantKind
	}{
		{"peer", "peer", variantBare},
		{"peer.exe", "peer", variantBare},
		{"peer_arm64", "peer", variantGoarch},
		{"peer_linux_amd64.exe", "peer", variantGoosGoarch},
		{"my_peer", "my_peer", variantBare}, // underscore name, not an arch suffix
		{"peer_linux", "peer_linux", variantBare},
	}
	for _, tc := range cases {
		base, kind, _, _ := stripVariant(tc.file)
		if base != tc.base || kind != tc.kind {
			t.Errorf("stripVariant(%q) = %q, %v; want %q, %v", tc.file, base, kind, tc.base, tc.kind)
		}
	}
}
