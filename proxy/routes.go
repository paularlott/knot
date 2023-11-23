package proxy

import (
	"github.com/go-chi/chi/v5"
	"github.com/paularlott/knot/middleware"
)

func Routes() chi.Router {
  router := chi.NewRouter()

  router.Use(middleware.ApiAuth)

  router.Get("/port/{host}/{port:\\d+}", HandleWSProxyServer)

  router.Route("/spaces/{space_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", func(router chi.Router) {
    router.Get("/ssh/*", HandleSpacesSSHProxy)
  })

  return router
}
