package apiv1

import (
	"github.com/paularlott/knot/middleware"

	"github.com/go-chi/chi/v5"
)

func ApiRoutes() chi.Router {
  router := chi.NewRouter()

  // Group routes that require authentication
  router.Group(func(router chi.Router) {
    router.Use(middleware.ApiAuth)

    // Users
    router.Route("/users", func(router chi.Router) {
      router.Post("/", HandleCreateUser)
    })
  })

  // Unauthenticated routes
  router.Post("/auth/web", HandleAuthorization)
  router.Get("/ping", HandlePing)
  router.Get("/lookup/{service}", HandleLookup)

  return router
}
