package filestore

import "net/http"

// RouteMounter adds HTTP endpoints serving file storage, such as Knot Pro's
// S3 API. It is called once at start-up when file storage is enabled.
type RouteMounter func(mux *http.ServeMux, store *Store)

var mounters []RouteMounter

// RegisterRoutes registers extra endpoints for file storage; call it from an
// init function.
func RegisterRoutes(m RouteMounter) {
	mounters = append(mounters, m)
}

// MountRoutes adds every registered endpoint to mux.
func MountRoutes(mux *http.ServeMux, store *Store) {
	for _, m := range mounters {
		m(mux, store)
	}
}

// SetUntrustedContentHeaders marks file content, which users choose, as
// untrusted when it is served from the knot origin: the browser must not
// sniff it into another type, and the sandbox stops an HTML or SVG file
// shared by another user from running script against the origin.
func SetUntrustedContentHeaders(h http.Header) {
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'")
}
