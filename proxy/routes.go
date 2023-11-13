package proxy

import (
	"github.com/go-chi/chi/v5"
	"github.com/paularlott/knot/middleware"
)

func Routes() chi.Router {
  router := chi.NewRouter()

  router.Use(middleware.ApiAuth)

  router.Get("/port/{host}/{port:\\d+}", HandleWSProxyServer)

  return router
}
