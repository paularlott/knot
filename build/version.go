package build

import "runtime/debug"

var (
	// Current version of knot
	Version string = "0.28.0"

	// The date the binary was built
	Date string
)

const scriptlingModulePath = "github.com/paularlott/scriptling"

// ScriptlingVersion returns the version of the embedded scriptling module,
// read from the binary's build metadata. Returns "unknown" if it can't be
// determined, or "local" when a replace directive points at a working copy.
func ScriptlingVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	norm := func(v string) string {
		if v == "" || v == "(devel)" {
			return "local"
		}
		return v
	}
	for _, dep := range bi.Deps {
		if dep.Path != scriptlingModulePath {
			continue
		}
		if dep.Replace != nil {
			return norm(dep.Replace.Version)
		}
		return norm(dep.Version)
	}
	return "unknown"
}

// FullVersion returns the knot version annotated with the embedded scriptling
// runtime version, e.g. "0.27.0 (scriptling v0.14.0)".
func FullVersion() string {
	return Version + " (scriptling " + ScriptlingVersion() + ")"
}
