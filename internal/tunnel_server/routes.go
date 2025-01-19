package tunnel_server

import (
	"net/http"

	"github.com/paularlott/knot/middleware"
)

func Routes(router *http.ServeMux) {
	router.HandleFunc("GET /tunnel/server/{tunnel_name}", middleware.ApiAuth(HandleTunnel))
}
