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
      if middleware.HasUsers {
        router.Use(middleware.ApiPermissionManageUsers)
      }

      router.Post("/", HandleCreateUser)
      router.Get("/", HandleGetUsers)
    })
    router.Route("/users/{user_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", func(router chi.Router) {
      router.Use(middleware.ApiPermissionManageUsersOrSelf)

      router.Get("/", HandleGetUser)
      router.Post("/", HandleUpdateUser)
      router.Delete("/", HandleDeleteUser)
    })

    // Roles
    router.Get("/roles", HandleGetRoles)

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
      router.Post("/{space_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleUpdateSpace)
      router.Delete("/{space_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleDeleteSpace)
      router.Get("/{space_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}/service-state", HandleGetSpaceServiceState)
      router.Post("/{space_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}/start", HandleSpaceStart)
      router.Post("/{space_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}/stop", HandleSpaceStop)
      router.Post("/stop-for-user/{user_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleSpaceStopUsersSpaces)
    })

    // Templates
    router.Route("/templates", func(router chi.Router) {
      router.Use(middleware.ApiPermissionManageTemplates)

      router.Get("/", HandleGetTemplates)
      router.Post("/", HandleCreateTemplate)
      router.Post("/{template_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleUpdateTemplate)
      router.Delete("/{template_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleDeleteTemplate)
    })
  })

  // Group routes that require authentication via agent token
  router.Group(func(router chi.Router) {
    router.Use(middleware.AgentAuth)

    router.Post("/agents/{space_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}/status", HandleAgentStatus)
  })

  // Unauthenticated routes
  router.Post("/auth/web", HandleAuthorization)
  router.Post("/agents/{space_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleRegisterAgent)

  return router
}
