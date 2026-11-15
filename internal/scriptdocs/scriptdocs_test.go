package scriptdocs

import (
	"io/fs"
	"sort"
	"strings"
	"testing"

	knotscriptling "github.com/paularlott/knot/internal/scriptling"
)

// Every page of the bundle has frontmatter that parses with a non-empty
// type (OKF 0.2 conformance); index.md has only okf_version.
func TestPagesHaveValidFrontmatter(t *testing.T) {
	err := fs.WalkDir(FS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		data, err := fs.ReadFile(FS(), path)
		if err != nil {
			return err
		}
		meta, body, perr := SplitFrontmatter(string(data))
		if perr != nil {
			t.Errorf("%s: %v", path, perr)
			return nil
		}
		if path == "index.md" {
			if v, _ := meta["okf_version"].(string); v != "0.2" {
				t.Errorf("index.md: okf_version = %v, want \"0.2\"", meta["okf_version"])
			}
			if len(meta) != 1 {
				t.Errorf("index.md: only okf_version is allowed in its frontmatter, got %v", meta)
			}
			return nil
		}
		if meta == nil {
			t.Errorf("%s: no frontmatter", path)
			return nil
		}
		if typ, _ := meta["type"].(string); strings.TrimSpace(typ) == "" {
			t.Errorf("%s: frontmatter has no type", path)
		}
		if desc, _ := meta["description"].(string); strings.TrimSpace(desc) == "" {
			t.Errorf("%s: frontmatter has no description (the guide lists it)", path)
		}
		if strings.TrimSpace(body) == "" {
			t.Errorf("%s: empty body", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Every knot.* module under internal/scriptling/lib/knot has a library page
// or is listed in NotForScripts. A new module fails here until it gets a
// page (scripts/generate-scripting-docs.mjs converts the website's) or is
// deliberately kept out of AI scripts.
func TestEveryKnotModuleHasALibraryPage(t *testing.T) {
	documented := map[string]bool{}
	for _, name := range Names() {
		documented[name] = true
	}
	entries, err := fs.ReadDir(knotscriptling.EmbeddedLibs, "lib/knot")
	if err != nil {
		t.Fatal(err)
	}
	var missing []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".py") || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		module := "knot." + strings.TrimSuffix(e.Name(), ".py")
		if documented[module] || NotForScripts[module] != "" {
			continue
		}
		missing = append(missing, module)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("knot modules with no page in internal/scriptdocs/okf/knot-scripting/libraries and not in NotForScripts: %s\n"+
			"Convert their website pages with: node scripts/generate-scripting-docs.mjs ../knot-website %s",
			strings.Join(missing, ", "), strings.Join(trimKnot(missing), " "))
	}
	for name := range NotForScripts {
		if documented[name] {
			t.Errorf("%s is documented but listed in NotForScripts", name)
		}
	}
}

func trimKnot(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = strings.TrimPrefix(n, "knot.")
	}
	return out
}

func TestGuideListsLibraries(t *testing.T) {
	guide := Guide()
	if !strings.Contains(guide, "# Writing scripts for run_script") {
		t.Fatalf("guide body missing:\n%s", guide[:min(len(guide), 200)])
	}
	if strings.HasPrefix(guide, "---") {
		t.Error("guide still carries its frontmatter")
	}
	for _, lib := range Libraries() {
		if !strings.Contains(guide, "`"+lib.Name+"`: "+lib.Description) {
			t.Errorf("guide does not list %s with its description", lib.Name)
		}
	}
}

func TestLookupNames(t *testing.T) {
	space, err := Library("space")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(space, "# knot.space") {
		t.Errorf("space page starts %q", space[:min(len(space), 40)])
	}
	for _, name := range []string{"knot.space", " Knot.Space ", "SPACE"} {
		if page, err := Library(name); err != nil || page != space {
			t.Errorf("Library(%q) = err %v", name, err)
		}
	}

	if got, err := Lookup(""); err != nil || got != Guide() {
		t.Errorf("Lookup(\"\") is not the guide (err %v)", err)
	}
	if got, _ := Lookup(" , "); got != Guide() {
		t.Error("blank names should give the guide")
	}

	both, err := Lookup("space, knot.pool")
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := Library("pool")
	if both != space+"\n\n---\n\n"+pool {
		t.Error("two libraries are not joined in order")
	}

	withGuide, err := Lookup("guide, space, space, knot.space")
	if err != nil {
		t.Fatal(err)
	}
	if withGuide != Guide()+"\n\n---\n\n"+space {
		t.Error("guide plus a repeated library should give the guide and the page once")
	}

	_, err = Lookup("space, nosuchlib")
	if err == nil || !strings.Contains(err.Error(), "nosuchlib") || !strings.Contains(err.Error(), "knot.space") {
		t.Errorf("unknown library error should name it and list the libraries, got %v", err)
	}
	for _, refused := range []string{"ai", "knot.token", "../writing-scripts", "libraries/space"} {
		if _, err := Library(refused); err == nil {
			t.Errorf("Library(%q) should fail", refused)
		}
	}
}

func TestSplitFrontmatter(t *testing.T) {
	meta, body, err := SplitFrontmatter("---\ntype: Guide\ntags:\n  - a\n---\n# Title\n")
	if err != nil || meta["type"] != "Guide" || body != "# Title\n" {
		t.Errorf("got %v %q %v", meta, body, err)
	}
	if meta, body, err := SplitFrontmatter("# No frontmatter\n"); err != nil || meta != nil || body != "# No frontmatter\n" {
		t.Errorf("got %v %q %v", meta, body, err)
	}
	if _, _, err := SplitFrontmatter("---\ntype: [\n---\nx"); err == nil {
		t.Error("bad YAML should fail")
	}
	if _, _, err := SplitFrontmatter("---\ntype: x\n"); err == nil {
		t.Error("unclosed frontmatter should fail")
	}
}
