package apiv1

import (
	"github.com/go-chi/chi/v5"
)

func ApiRoutes() chi.Router {
  router := chi.NewRouter()

  router.Get("/ping", HandlePing)
  router.Get("/lookup/{service}", HandleLookup)

  return router
}
