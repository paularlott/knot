package build

import "strings"

// VersionHeader carries the server's version on every reply, so the knot
// command can check it talks to a server it matches.
const VersionHeader = "X-Knot-Version"

// MajorMinor returns the "major.minor" part of a version such as "0.41.2",
// or "" when it has none.
func MajorMinor(version string) string {
	parts := strings.SplitN(strings.TrimPrefix(version, "v"), ".", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return parts[0] + "." + parts[1]
}

// SameMajorMinor reports whether two versions share major and minor
// version: the knot command, agents and servers work together only then,
// as their wire formats change between minor versions.
func SameMajorMinor(a, b string) bool {
	mm := MajorMinor(a)
	return mm != "" && mm == MajorMinor(b)
}
