package tunnel_server

import (
	"net/http"

	"github.com/paularlott/knot/internal/middleware"
)

func Routes(router *http.ServeMux) {
	router.HandleFunc("GET /tunnel/server/{tunnel_name}", middleware.ApiAuth(HandleTunnel))
	router.HandleFunc("/", HandleWebTunnel)
}
