package proxy

import (
	"github.com/paularlott/knot/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/viper"
)

func Routes() chi.Router {
  router := chi.NewRouter()

  router.Use(middleware.ApiAuth)

  if !viper.GetBool("server.disable_proxy") {
    router.Get("/port/{host}/{port:\\d+}", HandleWSProxyServer)
  }

  router.Route("/spaces/{space_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", func(router chi.Router) {
    router.Get("/ssh/*", HandleSpacesSSHProxy)
    router.Get("/port/{port}", HandleSpacesPortProxy)
    router.Get("/code-server/*", HandleSpacesCodeServerProxy)
    router.Get("/terminal/{shell:^[a-zA-Z0-9]+$}", HandleSpacesTerminalProxy)
  })

  return router
}
