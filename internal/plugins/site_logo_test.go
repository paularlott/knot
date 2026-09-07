package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func pluginWithLogos(t *testing.T, dir, name, extra string) {
	t.Helper()
	writePlugin(t, dir, name, `# requires-scriptling = ">=0.1"
#
# [tool.knot]
# version = "1.0"
# logo_light = "assets/logo-light.svg"
# logo_dark = "assets/logo-dark.svg"
`+extra)
	assetDir := filepath.Join(dir, name, "assets")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, logo := range []string{"logo-light.svg", "logo-dark.svg"} {
		if err := os.WriteFile(filepath.Join(assetDir, logo), []byte("<svg/>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestSiteLogo covers the site-logo claim: a declared logo pair claims the
// site logo, resolution is deterministic (first claimant by name), and
// several pairs warn.
func TestSiteLogo(t *testing.T) {
	dir := t.TempDir()
	pluginWithLogos(t, dir, "beta", "")
	pluginWithLogos(t, dir, "alpha", "")
	pluginWithLogos(t, dir, "gamma", "")

	registry, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	if len(registry.Failed()) != 0 {
		t.Fatalf("failed = %+v", registry.Failed())
	}

	light, dark := registry.SiteLogoURLs()
	if !strings.HasPrefix(light, "/plugins/alpha/assets/") {
		t.Errorf("site logo URL = %q, want the first plugin by name (alpha)", light)
	}
	// The URL is /plugins/<name>/assets/<relative path>; the fixture's logos
	// live in an assets/ subfolder, so the relative path carries it too.
	if light != "/plugins/alpha/assets/assets/logo-light.svg" || dark != "/plugins/alpha/assets/assets/logo-dark.svg" {
		t.Errorf("site logo URLs = %q / %q", light, dark)
	}

	warned := false
	for _, w := range registry.Warnings() {
		if strings.Contains(w, "multiple plugins") && strings.Contains(w, "alpha wins") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("warnings = %v, want multi-claim warning naming the winner", registry.Warnings())
	}

	// The claim is now implicit: a declared pair claims, site_logo is an
	// unknown key.
	dir2 := t.TempDir()
	writePlugin(t, dir2, "explicit", "# [tool.knot]\nversion = \"1.0\"\nsite_logo = true\n")
	registry2, err := Load(dir2)
	if err != nil {
		t.Fatal(err)
	}
	defer registry2.Close()
	if len(registry2.Failed()) != 1 || !strings.Contains(registry2.Failed()[0].Reason, "unknown key") {
		t.Fatalf("failed = %+v", registry2.Failed())
	}
}

// TestSingleLogoServesBothThemes: declaring one logo copies it to the other
// slot, so the rendered pair is complete and the claim is made.
func TestSingleLogoServesBothThemes(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "solo")
	if err := os.MkdirAll(filepath.Join(pluginDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "assets", "logo.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	writePlugin(t, dir, "solo", `# [tool.knot]
version = "1.0"
logo_dark = "assets/logo.svg"`)

	registry, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	if len(registry.Failed()) != 0 {
		t.Fatalf("failed = %+v", registry.Failed())
	}
	p := registry.All()[0]
	if !p.SiteLogo {
		t.Error("a single declared logo should claim the site logo")
	}
	if p.LogoLight != "assets/logo.svg" || p.LogoDark != "assets/logo.svg" {
		t.Errorf("logos = %q / %q, want the single logo in both slots", p.LogoLight, p.LogoDark)
	}
	light, dark := registry.SiteLogoURLs()
	if light != dark || light != "/plugins/solo/assets/assets/logo.svg" {
		t.Errorf("site logo URLs = %q / %q", light, dark)
	}
}
