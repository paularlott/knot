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

func TestLoadManifest_cachesByPath(t *testing.T) {
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
		t.Error("LoadManifest did not return cached pointer for same path")
	}
}

func TestLoadManifest_externalFile(t *testing.T) {
	Reload()
	tmp := t.TempDir() + "/custom.toml"
	custom := `
version  = 1
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

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
