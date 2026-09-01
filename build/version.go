package build

import (
	"runtime/debug"
	"strings"
)

var (
	// Current version of knot
	Version string = "0.33.0"

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

// IsCompatible reports whether other can talk to a build of this version.
// The major and minor parts must match; the patch part and any pre-release
// suffix are ignored. An empty or malformed version is never compatible —
// callers use that to reject peers too old to report a version at all.
func IsCompatible(other string) bool {
	ourParts := strings.Split(Version, ".")
	theirParts := strings.Split(other, ".")
	if len(ourParts) < 2 || len(theirParts) < 2 {
		return false
	}
	return ourParts[0] == theirParts[0] && ourParts[1] == theirParts[1]
}
