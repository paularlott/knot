package apiv1

import (
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/middleware"

	"github.com/go-chi/chi/v5"
)

func ApiRoutes() chi.Router {

  // Initialize agent information storage required by the API
  database.InitializeAgentInformation();

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

    // Spaces
    router.Route("/spaces", func(router chi.Router) {
      router.Get("/", HandleGetSpaces)
      router.Post("/", HandleCreateSpace)
      router.Delete("/{space_id}", HandleDeleteSpace)
    })
  })

  // Group routes that require authentication via agent token
  router.Group(func(router chi.Router) {
    router.Use(middleware.AgentAuth)

    router.Post("/agents/{space_id}/status", HandleAgentStatus)
  })

  // Unauthenticated routes
  router.Post("/auth/web", HandleAuthorization)
  router.Post("/agents/{space_id}", HandleRegisterAgent)

  return router
}
