package specwizard

import (
	"os"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/config"
)

func TestLoadManifest_embedded(t *testing.T) {
	Reload()
	m, err := LoadManifest(&config.ServerConfig{})
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if m.Version != 1 {
		t.Errorf("Version = %d, want 1", m.Version)
	}
	if m.ManifestVersion == "" {
		t.Error("ManifestVersion is empty; embedded catalog must carry a yyyymmddbb revision")
	}
	if len(m.Images) == 0 {
		t.Fatal("no images in bundled manifest")
	}
	for _, img := range m.Images {
		if img.Name == "" || img.Image == "" {
			t.Errorf("image entry missing required field: %+v", img)
		}
		if img.Category == "" {
			t.Errorf("image %q has empty category after normalise", img.Name)
		}
	}
}

func TestLoadManifest_cachesEmbedded(t *testing.T) {
	Reload()
	cfg1 := &config.ServerConfig{}
	m1, err := LoadManifest(cfg1)
	if err != nil {
		t.Fatalf("LoadManifest #1: %v", err)
	}
	m2, err := LoadManifest(cfg1)
	if err != nil {
		t.Fatalf("LoadManifest #2: %v", err)
	}
	if m1 != m2 {
		t.Error("LoadManifest did not return cached pointer for embedded manifest")
	}
}

func TestLoadManifest_externalFileReReadEachCall(t *testing.T) {
	Reload()
	tmp := t.TempDir() + "/custom.toml"
	custom := `
version  = 1
manifest_version = "2026010101"
description = "test"

[[image]]
name         = "test"
display_name = "Test"
description  = "Test image"
image        = "registry.example.com/test:1"
icon         = "https://example.com/icon.svg"
`
	if err := writeFile(tmp, custom); err != nil {
		t.Fatalf("write tmp manifest: %v", err)
	}

	cfg := &config.ServerConfig{BaseImagesManifest: tmp}
	m, err := LoadManifest(cfg)
	if err != nil {
		t.Fatalf("LoadManifest external: %v", err)
	}
	if len(m.Images) != 1 || m.Images[0].Name != "test" {
		t.Fatalf("got %+v, want single image named 'test'", m.Images)
	}
	if m.Images[0].Category != "general" {
		t.Errorf("Category = %q, want default 'general'", m.Images[0].Category)
	}

	// Mutate the name on disk and confirm the next call sees the change
	// (external files are NOT cached).
	if err := writeFile(tmp, strings.Replace(custom, "name         = \"test\"", "name         = \"changed\"", 1)); err != nil {
		t.Fatalf("rewrite tmp manifest: %v", err)
	}
	m2, err := LoadManifest(cfg)
	if err != nil {
		t.Fatalf("LoadManifest external #2: %v", err)
	}
	if m2.Images[0].Name != "changed" {
		t.Errorf("external manifest was cached: got name %q, want %q", m2.Images[0].Name, "changed")
	}
}

func TestLoadManifest_missingFile(t *testing.T) {
	Reload()
	cfg := &config.ServerConfig{BaseImagesManifest: "/nonexistent/path/manifest.toml"}
	if _, err := LoadManifest(cfg); err == nil {
		t.Fatal("LoadManifest returned nil error for missing file")
	} else if !strings.Contains(err.Error(), "read base images manifest") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSetFetchedManifest_storesWhenNewer(t *testing.T) {
	Reload()
	embedded := EmbeddedManifestVersion()

	// An older manifest must NOT replace the active one.
	older := `
version = 1
manifest_version = "0000000001"
description = "older"

[[image]]
name         = "old"
display_name = "Old"
description  = "old"
image        = "registry.example.com/old:1"
`
	stored, err := SetFetchedManifest([]byte(older), "fetch")
	if err != nil {
		t.Fatalf("SetFetchedManifest older: %v", err)
	}
	if stored {
		t.Fatal("stored an older manifest; should have been ignored")
	}
	if ActiveManifestVersion() != embedded {
		t.Errorf("active version changed after older fetch: got %q, want %q", ActiveManifestVersion(), embedded)
	}

	// A newer manifest replaces the active one.
	newer := `
version = 1
manifest_version = "9999123199"
description = "newer"

[[image]]
name         = "new"
display_name = "New"
description  = "new"
image        = "registry.example.com/new:1"
`
	stored, err = SetFetchedManifest([]byte(newer), "fetch")
	if err != nil {
		t.Fatalf("SetFetchedManifest newer: %v", err)
	}
	if !stored {
		t.Fatal("did not store a newer manifest")
	}
	if ActiveManifestVersion() != "9999123199" {
		t.Errorf("active version = %q, want 9999123199", ActiveManifestVersion())
	}
	if FetchedManifestSource() != "fetch" {
		t.Errorf("source = %q, want fetch", FetchedManifestSource())
	}

	// Same version is not stored again.
	stored, err = SetFetchedManifest([]byte(newer), "gossip")
	if err != nil {
		t.Fatalf("SetFetchedManifest dup: %v", err)
	}
	if stored {
		t.Fatal("stored an equal-version manifest; should have been ignored")
	}
	if FetchedManifestSource() != "fetch" {
		t.Errorf("source changed on equal-version store: got %q", FetchedManifestSource())
	}
}

func TestSetFetchedManifest_fileIsBaseline(t *testing.T) {
	tmp := t.TempDir() + "/custom.toml"

	// An external file with an OLD version: a fetched newer copy must overlay it.
	oldFile := `
version  = 1
manifest_version = "2026010101"
description = "old file"

[[image]]
name         = "file-old"
display_name = "File Old"
description  = "old"
image        = "registry.example.com/old:1"
`
	if err := writeFile(tmp, oldFile); err != nil {
		t.Fatalf("write tmp manifest: %v", err)
	}
	config.SetServerConfig(&config.ServerConfig{BaseImagesManifest: tmp})
	defer config.SetServerConfig(nil)
	Reload()

	// LoadManifest serves the file as baseline.
	if m, err := LoadManifest(config.GetServerConfig()); err != nil || m.Images[0].Name != "file-old" {
		t.Fatalf("baseline should be the file: %+v %v", m, err)
	}
	if ActiveManifestVersion() != "2026010101" {
		t.Errorf("baseline version = %q, want 2026010101", ActiveManifestVersion())
	}

	// Fetch a NEWER remote → stored, and served instead of the file.
	newer := `
version = 1
manifest_version = "2026020202"
description = "newer remote"

[[image]]
name         = "remote-new"
display_name = "Remote New"
description  = "new"
image        = "registry.example.com/new:1"
`
	stored, err := SetFetchedManifest([]byte(newer), SourceAPI)
	if err != nil || !stored {
		t.Fatalf("expected newer fetch to overlay the file: stored=%v err=%v", stored, err)
	}
	if m, err := LoadManifest(config.GetServerConfig()); err != nil || m.Images[0].Name != "remote-new" {
		t.Fatalf("fetched should overlay file: %+v %v", m, err)
	}
	if ActiveManifestVersion() != "2026020202" {
		t.Errorf("active version = %q, want 2026020202", ActiveManifestVersion())
	}

	// Now bump the file to NEWER than the fetch (admin updated the file). The
	// file must win again on the next request because it's re-read each call.
	newerFile := `
version  = 1
manifest_version = "2026030303"
description = "bumped file"

[[image]]
name         = "file-bumped"
display_name = "File Bumped"
description  = "bumped"
image        = "registry.example.com/bumped:1"
`
	if err := writeFile(tmp, newerFile); err != nil {
		t.Fatalf("rewrite tmp manifest: %v", err)
	}
	if m, err := LoadManifest(config.GetServerConfig()); err != nil || m.Images[0].Name != "file-bumped" {
		t.Fatalf("bumped file should win over the stale fetch: %+v %v", m, err)
	}
	if ActiveManifestVersion() != "2026030303" {
		t.Errorf("active version = %q, want 2026030303", ActiveManifestVersion())
	}

	// A fetch older than the bumped file must NOT be stored.
	stored, err = SetFetchedManifest([]byte(newer), SourceAPI) // newer is 2026020202 < file 2026030303
	if err != nil || stored {
		t.Fatalf("fetch older than file must be ignored: stored=%v err=%v", stored, err)
	}
}

func TestManifestVolumes_parsed(t *testing.T) {
	Reload()
	m, err := LoadManifest(&config.ServerConfig{})
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	// The embedded catalog declares volumes on every image; spot-check a
	// datastore (Valkey/Redis → /data) and a web image (PHP → /home).
	sawData := false
	sawHome := false
	for _, img := range m.Images {
		for _, vol := range img.Volumes {
			// No kind in the bundled catalog → defaults to "" (→ "volume" in the wizard).
			if vol.Kind != "" {
				t.Errorf("embedded volume %q on %q has kind %q; bundled catalog should leave it unset", vol.Path, img.Name, vol.Kind)
			}
			if vol.Path == "/data" {
				sawData = true
			}
			if vol.Path == "/home" {
				sawHome = true
			}
		}
	}
	if !sawData {
		t.Error("no image declares a /data volume (expected for Valkey/Redis)")
	}
	if !sawHome {
		t.Error("no image declares a /home volume (expected for Ubuntu/PHP)")
	}
}

func TestManifestVolumeKindNormalised(t *testing.T) {
	Reload()
	tmp := t.TempDir() + "/kinds.toml"
	custom := `
version  = 1
manifest_version = "2026010101"
description = "kinds"

[[image]]
name         = "k"
display_name = "K"
description  = "k"
image        = "registry.example.com/k:1"

[[image.volume]]
path = "/v"
kind = "volume"

[[image.volume]]
path = "/b"
kind = "bind"

[[image.volume]]
path = "/p"
kind = "path"

[[image.volume]]
path = "/u"
# no kind → defaults to volume

[[image.volume]]
path = "/x"
kind = "garbage"
`
	if err := writeFile(tmp, custom); err != nil {
		t.Fatalf("write tmp manifest: %v", err)
	}
	m, err := LoadManifest(&config.ServerConfig{BaseImagesManifest: tmp})
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	want := map[string]string{"/v": "volume", "/b": "bind", "/p": "path", "/u": "", "/x": ""}
	got := map[string]string{}
	for _, vol := range m.Images[0].Volumes {
		got[vol.Path] = vol.Kind
	}
	for path, kind := range want {
		if got[path] != kind {
			t.Errorf("kind for %q = %q, want %q", path, got[path], kind)
		}
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
