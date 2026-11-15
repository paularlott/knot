// Package scriptdocs embeds the documentation an AI assistant reads to write
// scripts for the run_script tool: an OKF 0.2 bundle (okf/knot-scripting)
// holding the guide (writing-scripts.md) and one page per knot.* library a
// script may import (libraries/<module>.md). The script_docs MCP tool and
// docs() inside a script serve it.
package scriptdocs

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed okf/knot-scripting
var bundle embed.FS

const (
	root       = "okf/knot-scripting"
	guidePage  = root + "/writing-scripts.md"
	librarySub = root + "/libraries"
)

// NotForScripts are the knot.* modules AI-written scripts may not import,
// and so have no page in the bundle. Keep in step with aiScriptLibraries in
// internal/service/ai_script.go; the drift test checks every
// module under internal/scriptling/lib/knot is documented or listed here.
var NotForScripts = map[string]string{
	"knot.ai":         "calls the AI model",
	"knot.mcp":        "calls other MCP tools",
	"knot.event":      "event sink accessors, and imports requests",
	"knot.secret":     "reads secret values",
	"knot.token":      "manages API tokens",
	"knot.mcp_server": "MCP server configurations hold credentials, and listing tools reaches the remote server",
}

// LibraryInfo names one documented library and what it is for.
type LibraryInfo struct {
	Name        string // full module name, e.g. "knot.space"
	Description string // the page's frontmatter description
}

// FS returns the embedded bundle, rooted at its index.md.
func FS() fs.FS {
	sub, err := fs.Sub(bundle, root)
	if err != nil {
		panic(err)
	}
	return sub
}

// Guide returns the guide to writing scripts, followed by the list of
// libraries with their descriptions.
func Guide() string {
	_, body, err := readPage(guidePage)
	if err != nil {
		return "The guide to writing scripts is missing from this build."
	}
	var b strings.Builder
	b.WriteString(strings.TrimRight(body, "\n"))
	b.WriteString("\n\n")
	for _, lib := range Libraries() {
		fmt.Fprintf(&b, "- `%s`: %s\n", lib.Name, lib.Description)
	}
	b.WriteString("\nFor a library's functions and parameters, call script_docs with its name, or use help() in a script.\n")
	return b.String()
}

// Library returns the page of one library, named with or without the
// "knot." prefix. An unknown name is an error listing the valid names.
func Library(name string) (string, error) {
	module := ModuleName(name)
	if module == "" {
		return "", fmt.Errorf("no library named; the libraries are %s", strings.Join(Names(), ", "))
	}
	_, body, err := readPage(librarySub + "/" + strings.TrimPrefix(module, "knot.") + ".md")
	if err != nil {
		return "", fmt.Errorf("no documentation for %q; the libraries are %s", strings.TrimSpace(name), strings.Join(Names(), ", "))
	}
	return strings.TrimRight(body, "\n") + "\n", nil
}

// Lookup serves the script_docs tool: spec is empty (the guide) or one or
// more comma-separated names, each a library ("space", "knot.pool") or
// "guide". The pages are returned in the order named, separated by rules.
func Lookup(spec string) (string, error) {
	var pages []string
	seen := map[string]bool{}
	for _, part := range strings.Split(spec, ",") {
		name := strings.TrimSpace(part)
		key := ModuleName(name)
		if isGuideName(name) {
			key = "guide"
		}
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if key == "guide" {
			pages = append(pages, Guide())
			continue
		}
		page, err := Library(name)
		if err != nil {
			return "", err
		}
		pages = append(pages, page)
	}
	if len(pages) == 0 {
		return Guide(), nil
	}
	return strings.Join(pages, "\n\n---\n\n"), nil
}

// isGuideName reports whether a script_docs name asks for the guide.
func isGuideName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "guide", "rules", "writing-scripts", "writing scripts":
		return true
	}
	return false
}

// ModuleName normalises a library name to its module ("pool", "knot.pool"
// and " Knot.Pool " are all "knot.pool"); "" for a blank name.
func ModuleName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	return "knot." + strings.TrimPrefix(name, "knot.")
}

// Libraries lists the documented libraries, sorted by name, with the
// description from each page's frontmatter.
func Libraries() []LibraryInfo {
	entries, err := fs.ReadDir(bundle, librarySub)
	if err != nil {
		return nil
	}
	var libs []LibraryInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		meta, _, err := readPage(librarySub + "/" + e.Name())
		if err != nil {
			continue
		}
		description, _ := meta["description"].(string)
		libs = append(libs, LibraryInfo{
			Name:        "knot." + strings.TrimSuffix(e.Name(), ".md"),
			Description: strings.TrimSpace(description),
		})
	}
	sort.Slice(libs, func(i, j int) bool { return libs[i].Name < libs[j].Name })
	return libs
}

// Names lists the documented library names.
func Names() []string {
	var names []string
	for _, lib := range Libraries() {
		names = append(names, lib.Name)
	}
	return names
}

// readPage reads a bundle page and splits its YAML frontmatter from the body.
func readPage(name string) (map[string]any, string, error) {
	data, err := bundle.ReadFile(name)
	if err != nil {
		return nil, "", err
	}
	meta, body, err := SplitFrontmatter(string(data))
	return meta, body, err
}

// SplitFrontmatter separates a page's YAML frontmatter from its body. A page
// without frontmatter has a nil map; frontmatter that does not parse is an
// error.
func SplitFrontmatter(page string) (map[string]any, string, error) {
	page = strings.TrimPrefix(page, "\ufeff")
	page = strings.ReplaceAll(page, "\r\n", "\n")
	rest, ok := strings.CutPrefix(page, "---\n")
	if !ok {
		return nil, page, nil
	}
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		if strings.HasSuffix(rest, "\n---") {
			end = len(rest) - len("\n---")
		} else {
			return nil, page, fmt.Errorf("frontmatter is not closed")
		}
	}
	meta := map[string]any{}
	if err := yaml.Unmarshal([]byte(rest[:end]), &meta); err != nil {
		return nil, page, fmt.Errorf("frontmatter: %w", err)
	}
	body := ""
	if end+len("\n---\n") <= len(rest) {
		body = rest[end+len("\n---\n"):]
	}
	return meta, strings.TrimLeft(body, "\n"), nil
}
