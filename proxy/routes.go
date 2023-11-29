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

  router.Route("/spaces/{space_name:^[a-zA-Z][a-zA-Z0-9\\-]{1,63}$}", func(router chi.Router) {
    router.Get("/ssh/*", HandleSpacesSSHProxy)
    router.Get("/port/{port}", HandleSpacesPortProxy)
    router.Get("/code-server/*", HandleSpacesCodeServerProxy)
    router.Get("/terminal/{shell:^[a-z]+$}", HandleSpacesTerminalProxy)
  })

  return router
}
