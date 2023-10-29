package proxy

import (
	"github.com/go-chi/chi/v5"
)

func Routes() chi.Router {
  router := chi.NewRouter()

  router.Get("/port/{host}/{port:\\d+}", HandleWSProxyServer)

  return router
}
