package tunnel_server

import (
	"github.com/paularlott/knot/middleware"

	"github.com/go-chi/chi/v5"
)

func Routes() chi.Router {
	router := chi.NewRouter()

	router.Use(middleware.ApiAuth)

	// Tunnel server
	router.Get("/server/{tunnel_name:^[a-zA-Z][a-zA-Z0-9-]{1,63}$}", HandleTunnel)

	return router
}
