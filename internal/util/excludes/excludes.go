// Package excludes matches paths against exclude patterns, for the commands
// that copy directory trees: knot space mirror and knot file sync.
package excludes

import (
	"path"
	"path/filepath"
	"strings"
)

// Defaults are left out of continual and two-way syncs unless asked for:
// version control internals, which change on every command and only make
// sense whole, and the scratch files editors and the system write beside the
// files being edited.
var Defaults = []string{
	".git/", ".hg/", ".svn/",
	".DS_Store", "Thumbs.db", "desktop.ini",
	".*.swp", ".*.swo", ".*.swn", "*~", ".#*", "#*#", "4913",
}

// Compile returns a function that reports whether a slash-relative path
// should be skipped. Each pattern is matched against the basename and
// the full relative path; ancestor matches make excludes transitive (so
// "node_modules" excludes everything under it).
func Compile(patterns []string) func(rel string, isDir bool) bool {
	type compiled struct {
		glob    string
		dirOnly bool
	}
	var cs []compiled
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		dirOnly := false
		if strings.HasSuffix(p, "/") {
			dirOnly = true
			p = strings.TrimSuffix(p, "/")
		}
		p = path.Clean(p)
		if p == "." || p == "" {
			continue
		}
		cs = append(cs, compiled{glob: p, dirOnly: dirOnly})
	}
	return func(rel string, isDir bool) bool {
		base := rel
		if i := strings.LastIndexByte(rel, '/'); i >= 0 {
			base = rel[i+1:]
		}
		// Direct match.
		for _, c := range cs {
			if c.dirOnly && !isDir {
				continue
			}
			if ok, _ := filepath.Match(c.glob, rel); ok {
				return true
			}
			if base != rel {
				if ok, _ := filepath.Match(c.glob, base); ok {
					return true
				}
			}
		}
		// Ancestor match — makes excludes transitive.
		for _, c := range cs {
			if ancestorMatches(rel, c.glob) {
				return true
			}
		}
		return false
	}
}

// ancestorMatches reports whether any parent directory of rel matches glob.
// "a/b/c.txt" checks "a/b" then "a".
func ancestorMatches(rel, glob string) bool {
	p := rel
	for {
		i := strings.LastIndexByte(p, '/')
		if i < 0 {
			return false
		}
		parent := p[:i]
		parentBase := parent
		if j := strings.LastIndexByte(parent, '/'); j >= 0 {
			parentBase = parent[j+1:]
		}
		if ok, _ := filepath.Match(glob, parent); ok {
			return true
		}
		if parentBase != parent {
			if ok, _ := filepath.Match(glob, parentBase); ok {
				return true
			}
		}
		p = parent
	}
}
