package scriptling

import (
	"embed"
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

//go:embed lib/knot/*.py
var shadowGuardFS embed.FS

// Nearly every knot library module exposes a public function named list
// (knot.template.list, knot.space.list, ...). Inside such a module the
// definition shadows the list builtin — standard Python semantics — so any
// use of the bare name expecting the builtin (a copy via list(x), or
// isinstance(x, list)) silently calls the module function instead. That
// exact mistake produced a runaway recursion in knot.template once. This
// test scans the embedded libs so the next one fails here instead.
//
// Calls to the module's own function are legitimate (audit.search
// delegates to audit.list), so call sites are checked against a small
// allowlist; type positions (isinstance's second argument) can never be
// legitimate with a shadowed name.
var shadowedBuiltins = map[string]bool{
	"list": true, "dict": true, "set": true, "tuple": true, "str": true,
	"int": true, "float": true, "bool": true, "len": true, "id": true,
	"type": true, "range": true, "object": true, "sorted": true,
	"any": true, "all": true, "min": true, "max": true, "sum": true,
	"map": true, "filter": true, "zip": true, "repr": true, "bytes": true,
}

// allowedSelfCalls lists "file:name" pairs where a module deliberately
// calls its own function of a shadowed-builtin name.
var allowedSelfCalls = map[string]bool{
	"audit.py:list": true, // search() delegates to the module's list()
}

var defRe = regexp.MustCompile(`(?m)^def ([A-Za-z_][A-Za-z0-9_]*)\s*\(`)

func TestLibsDoNotUseShadowedBuiltins(t *testing.T) {
	entries, err := fs.ReadDir(shadowGuardFS, "lib/knot")
	if err != nil {
		t.Fatal(err)
	}

	callRe := func(name string) *regexp.Regexp {
		// A call is `name(` with no space (prose like "list (optional)"
		// and the definition itself don't count); a leading dot means a
		// method on another object, which is fine.
		return regexp.MustCompile(`(^|[^.\w])` + name + `\(`)
	}
	isinstanceRe := func(name string) *regexp.Regexp {
		return regexp.MustCompile(`isinstance\s*\([^,]+,\s*\(?\s*` + name + `\b`)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".py") {
			continue
		}
		data, err := fs.ReadFile(shadowGuardFS, "lib/knot/"+entry.Name())
		if err != nil {
			t.Fatal(err)
		}

		// Module-level definitions that shadow a builtin.
		shadowed := map[string]bool{}
		for _, match := range defRe.FindAllStringSubmatch(string(data), -1) {
			if shadowedBuiltins[match[1]] {
				shadowed[match[1]] = true
			}
		}
		if len(shadowed) == 0 {
			continue
		}

		for name := range shadowed {
			// Type positions: never legitimate with a shadowed name.
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "#") {
					continue
				}
				if isinstanceRe(name).MatchString(line) {
					t.Errorf("%s: isinstance with shadowed %q in: %s", entry.Name(), name, strings.TrimSpace(line))
				}
			}

			// Call positions: legitimate self-calls are allowlisted, and
			// the definition line itself is not a call.
			for i, line := range strings.Split(string(data), "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "def ") {
					continue
				}
				if callRe(name).MatchString(line) && !allowedSelfCalls[entry.Name()+":"+name] {
					t.Errorf("%s:%d: call to shadowed %q in: %s (use a comprehension/loop, or rename the local)", entry.Name(), i+1, name, trimmed)
				}
			}
		}
	}
}
