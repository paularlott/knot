package apiv1

import (
	"github.com/paularlott/knot/internal/origin_leaf"
	"github.com/paularlott/knot/internal/origin_leaf/server_info"
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
		router.Post("/auth/logout", HandleLogout)

		// Users
		router.Route("/users", func(router chi.Router) {
			router.Use(middleware.ApiPermissionManageUsers)

			router.Post("/", HandleCreateUser)
			router.Get("/", HandleGetUsers)
		})
		router.Get("/users/whoami", HandleWhoAmI)
		router.Route("/users/{user_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", func(router chi.Router) {
			router.Use(middleware.ApiPermissionManageUsersOrSelf)

			router.Get("/", HandleGetUser)
			router.Put("/", HandleUpdateUser)
			router.Delete("/", HandleDeleteUser)
			router.Get("/quota", HandleGetUserQuota)
		})

		// Groups
		router.Route("/groups", func(router chi.Router) {
			router.Use(middleware.ApiPermissionManageUsers)

			router.Post("/", HandleCreateGroup)
			router.Put("/{group_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleUpdateGroup)
			router.Delete("/{group_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleDeleteGroup)
			router.Get("/{group_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleGetGroup)
		})
		router.Get("/groups", HandleGetGroups)

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
			router.Put("/{space_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleUpdateSpace)
			router.Delete("/{space_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleDeleteSpace)
			router.Get("/{space_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleGetSpace)
			router.Get("/{space_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}/service-state", HandleGetSpaceServiceState)
			router.Post("/{space_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}/start", HandleSpaceStart)
			router.Post("/{space_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}/stop", HandleSpaceStop)
			router.Post("/stop-for-user/{user_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleSpaceStopUsersSpaces)
		})

		// Templates
		router.Route("/templates", func(router chi.Router) {
			router.Group(func(router chi.Router) {
				router.Use(middleware.ApiPermissionManageTemplates)

				router.Get("/{template_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleGetTemplate)
				router.Post("/", HandleCreateTemplate)
				router.Put("/{template_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleUpdateTemplate)
				router.Delete("/{template_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleDeleteTemplate)
			})

			router.Get("/", HandleGetTemplates)
		})

		// Volumes
		router.Route("/volumes", func(router chi.Router) {
			router.Use(middleware.ApiPermissionManageVolumes)

			router.Get("/", HandleGetVolumes)
			router.Post("/", HandleCreateVolume)
			router.Put("/{volume_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleUpdateVolume)
			router.Delete("/{volume_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleDeleteVolume)
			router.Get("/{volume_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleGetVolume)
			router.Post("/{volume_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}/start", HandleVolumeStart)
			router.Post("/{volume_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}/stop", HandleVolumeStop)
		})

		// Template Variables
		router.Route("/templatevars", func(router chi.Router) {
			router.Use(middleware.ApiPermissionManageTemplates)

			router.Get("/", HandleGetTemplateVars)
			router.Post("/", HandleCreateTemplateVar)
			router.Put("/{templatevar_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleUpdateTemplateVar)
			router.Delete("/{templatevar_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleDeleteTemplateVar)
			router.Get("/{templatevar_id:^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$}", HandleGetTemplateVar)
		})
	})

	// Unauthenticated routes
	router.Route("/auth", func(router chi.Router) {
		router.Post("/", HandleAuthorization)
		router.Post("/web", HandleAuthorization)
	})

	// Additional endpoints exposed by origin servers
	if server_info.IsOrigin {
		// Remote server authenticated routes
		router.Route("/leaf-server", func(router chi.Router) {
			router.Use(middleware.LeafServerAuth)

			// Listen for leaf server connections
			router.Get("/", origin_leaf.OriginListenAndServe)
		})
	}

	return router
}
