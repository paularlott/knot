package util

import (
	"net/http"
	"strings"
)

// KnotCookiePrefix starts the name of every cookie knot sets, after the
// "__Host-" prefix an https server adds.
const KnotCookiePrefix = "__KNOT_"

// StripKnotCookies removes knot's own cookies from a request headed for a
// space or tunnel. What runs there is the user's code: given the session
// cookie of whoever opened the page, it could act as them. The space's own
// cookies pass through unchanged.
func StripKnotCookies(h http.Header) {
	lines := h.Values("Cookie")
	if len(lines) == 0 {
		return
	}

	var kept []string
	for _, line := range lines {
		for _, part := range strings.Split(line, ";") {
			part = strings.TrimSpace(part)
			name, _, _ := strings.Cut(part, "=")
			name = strings.TrimPrefix(strings.TrimSpace(name), "__Host-")
			if part == "" || strings.HasPrefix(name, KnotCookiePrefix) {
				continue
			}
			kept = append(kept, part)
		}
	}

	h.Del("Cookie")
	if len(kept) > 0 {
		h.Set("Cookie", strings.Join(kept, "; "))
	}
}
