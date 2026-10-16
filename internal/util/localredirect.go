package util

import (
	"net/url"
	"strings"
)

// LocalRedirect reduces a user-supplied post-login redirect target to a
// path on this server: any scheme, host or fragment is dropped and only
// the path and query survive. It returns "" when the target can't safely
// be one, or is /logout.
//
// The path stays percent-encoded, so feeding the result back in returns it
// unchanged: a target kept in a cookie and checked again later can't
// decode a second time into something else, such as "/%2509/evil.com"
// becoming "/\t/evil.com", which browsers read as "//evil.com".
func LocalRedirect(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}

	path := u.EscapedPath()
	if path == "" || u.Path == "/logout" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// A path of "//host", or one browsers turn into it by dropping tabs
	// and newlines or reading a backslash as a slash, would make the
	// Location header protocol-relative. Check the path as sent, decoded,
	// and decoded again, so no layer of encoding hides one.
	for _, p := range []string{path, u.Path} {
		if !safeLocalPath(p) {
			return ""
		}
		if twice, err := url.PathUnescape(p); err == nil && !safeLocalPath(twice) {
			return ""
		}
	}

	if u.RawQuery != "" {
		return path + "?" + u.RawQuery
	}
	return path
}

func safeLocalPath(p string) bool {
	if strings.HasPrefix(p, "//") {
		return false
	}
	for _, c := range p {
		if c < 0x20 || c == 0x7f || c == '\\' {
			return false
		}
	}
	return true
}
