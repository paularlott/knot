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

    // Core
    router.Get("/lookup/{service}", HandleLookup)
    router.Get("/ping", HandlePing)

    // Users
    router.Route("/users", func(router chi.Router) {
      router.Post("/", HandleCreateUser)
    })

    // Sessions
    router.Route("/sessions", func(router chi.Router) {
      router.Get("/", HandleGetSessions)
      router.Delete("/{session_id}", HandleDeleteSessions)
    })

    // Tokens
    router.Route("/tokens", func(router chi.Router) {
      router.Get("/", HandleGetTokens)
      router.Post("/", HandleCreateToken)
      router.Delete("/{token_id}", HandleDeleteToken)
    })

    // Agents
    router.Route("/spaces", func(router chi.Router) {
      router.Get("/", HandleGetSpaces)
      router.Post("/", HandleCreateSpace)
      router.Delete("/{space_id}", HandleDeleteSpace)
    })
  })

  // Unauthenticated routes
  router.Post("/auth/web", HandleAuthorization)

  return router
}
