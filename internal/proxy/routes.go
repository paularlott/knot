package proxy

import (
	"net"
	"net/http"
	"strings"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/middleware"
)

func Routes(router *http.ServeMux, cfg *config.ServerConfig) {
	router.HandleFunc("GET /proxy/spaces/{space_id}/code-server/", middleware.ApiAuth(HandleSpacesCodeServerProxy))
	router.HandleFunc("GET /proxy/spaces/{space_id}/terminal/{shell}", middleware.ApiAuth(HandleSpacesTerminalProxy))

	router.HandleFunc("GET /proxy/spaces/{space_name}/port/{port}", middleware.ApiAuth(HandleSpacesPortProxy))
	router.HandleFunc("GET /proxy/spaces/{space_name}/ssh/", middleware.ApiAuth(HandleSpacesSSHProxy))

	router.HandleFunc("GET /tunnel/spaces/{space_name}/{port}", middleware.ApiAuth(handlePortTunnel))
}

// Setup proxying of URLs to ports within spaces
func PortRoutes() *http.ServeMux {
	router := http.NewServeMux()
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// VNC subdomains require an authenticated session (same as the SSH and
		// terminal proxies). Regular web ports remain reachable by URL.
		if isVNCSubdomain(r.Host) {
			middleware.ApiAuth(HandleSpacesWebPortProxy)(w, r)
			return
		}
		HandleSpacesWebPortProxy(w, r)
	})
	return router
}

// isVNCSubdomain reports whether the wildcard host targets web VNC, i.e. the
// subdomain has the shape "<owner>--<space>--vnc".
func isVNCSubdomain(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	first := strings.SplitN(host, ".", 2)[0]
	parts := strings.Split(first, "--")
	return len(parts) == 3 && parts[2] == "vnc"
}
