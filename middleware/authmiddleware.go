package middleware

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"
	"github.com/rs/zerolog/log"
	"golang.org/x/net/context"
)

var (
	HasUsers bool
)

func Initialize() {
	// Test if there's users present in the system
	db := database.GetInstance()
	hasUsers, err := db.HasUsers()
	if err != nil {
		log.Fatal().Msgf("failed to get user count: %s", err.Error())
	}

	if hasUsers || err != nil {
		HasUsers = true
	} else {
		HasUsers = false
	}
}

func returnUnauthorized(w http.ResponseWriter, r *http.Request) {
	rest.SendJSON(http.StatusUnauthorized, w, r, struct {
		Error string `json:"error"`
	}{
		Error: "Authentication token is not valid",
	})
}

func GetBearerToken(w http.ResponseWriter, r *http.Request) string {
	// Get the auth token
	var bearer string
	fmt.Sscanf(r.Header.Get("Authorization"), "Bearer %s", &bearer)
	if len(bearer) < 1 {
		returnUnauthorized(w, r)
		return ""
	}

	return bearer
}

func ApiAuth(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// If there's no users in the system then we don't check for authentication
		if HasUsers {
			var userId string = ""

			db := database.GetInstance()
			store := database.GetSessionStorage()

			// If have an Authorization header then we use that for authentication
			authorization := r.Header.Get("Authorization")
			if authorization != "" {

				// Get the auth token
				bearer := GetBearerToken(w, r)
				if bearer == "" {
					returnUnauthorized(w, r)
					return
				}

				token, _ := db.GetToken(bearer)
				if token == nil {
					returnUnauthorized(w, r)
					return
				}

				userId = token.UserId

				// Save the token to extend its life
				db.SaveToken(token)

				// Add the token to the context
				ctx = context.WithValue(r.Context(), "access_token", token)
			} else {
				// Get the session
				session := GetSessionFromCookie(r)
				if session == nil {
					returnUnauthorized(w, r)
					return
				}

				userId = session.UserId

				// Save the session to extend its life
				store.SaveSession(session)

				// Add the session to the context
				ctx = context.WithValue(r.Context(), "session", session)
			}

			// Get the user
			user, err := db.GetUser(userId)
			if err != nil || !user.Active || user.IsDeleted {
				returnUnauthorized(w, r)
				return
			}

			// Add the user to the context
			ctx = context.WithValue(ctx, "user", user)

			// Get the API version
			apiVersion := r.Header.Get("X-Knot-Api-Version")
			if apiVersion == "" {
				apiVersion = "2025-03-10"
			}
			ctx = context.WithValue(ctx, "api_version", apiVersion)
		}

		// If authenticated, continue
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func checkPermission(next http.HandlerFunc, permission uint16, msg string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value("user").(*model.User)
		if !user.HasPermission(permission) {
			rest.SendJSON(http.StatusForbidden, w, r, ErrorResponse{Error: msg})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func ApiPermissionManageTemplates(next http.HandlerFunc) http.HandlerFunc {
	if config.LeafNode {
		return next
	}

	return checkPermission(next, model.PermissionManageTemplates, "No permission to manage templates")
}

func ApiPermissionManageVolumes(next http.HandlerFunc) http.HandlerFunc {
	if config.LeafNode {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value("user").(*model.User)
		if !user.HasPermission(model.PermissionManageVolumes) {
			rest.SendJSON(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to manage volumes"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func ApiPermissionManageVariables(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionManageVariables, "No permission to manage variables")
}

func ApiPermissionUseTunnels(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionUseTunnels, "No permission to use tunnels")
}

func ApiPermissionViewAuditLogs(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionViewAuditLogs, "No permission to view audit logs")
}

func ApiPermissionViewClusterInfo(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionClusterInfo, "No permission to view cluster info")
}

func ApiPermissionManageUsers(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if HasUsers {
			user := r.Context().Value("user").(*model.User)
			if HasUsers && !user.HasPermission(model.PermissionManageUsers) {
				rest.SendJSON(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to manage users"})
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func ApiPermissionManageUsersOrSpaces(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if HasUsers {
			user := r.Context().Value("user").(*model.User)
			if HasUsers && !user.HasPermission(model.PermissionManageUsers) && !user.HasPermission(model.PermissionManageSpaces) && !user.HasPermission(model.PermissionTransferSpaces) {
				rest.SendJSON(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to manage users"})
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func ApiPermissionManageUsersOrSelf(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userId := r.PathValue("user_id")
		if !validate.UUID(userId) {
			rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid user ID"})
			return
		}

		user := r.Context().Value("user").(*model.User)
		if !user.HasPermission(model.PermissionManageUsers) && user.Id != userId {
			rest.SendJSON(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to manage users"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func ApiPermissionUseSpaces(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value("user").(*model.User)
		if !user.HasPermission(model.PermissionManageSpaces) && !user.HasPermission(model.PermissionUseSpaces) {
			rest.SendJSON(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to manage or use spaces"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func ApiPermissionTransferSpaces(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionTransferSpaces, "No permission to transfer spaces")
}

func ApiPermissionManageGroups(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionManageGroups, "No permission to manage groups")
}

func ApiPermissionManageRoles(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionManageRoles, "No permission to manage roles")
}

func WebAuth(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// If no session then redirect to login
		session := GetSessionFromCookie(r)
		if session == nil {
			http.Redirect(w, r, "/login?redirect="+r.URL.EscapedPath(), http.StatusSeeOther)
			return
		}

		// Get the user from the session
		db := database.GetInstance()
		var err error
		user, err := db.GetUser(session.UserId)
		if err != nil || !user.Active || user.IsDeleted {
			DeleteSessionCookie(w)
			http.Redirect(w, r, "/login?redirect="+r.URL.EscapedPath(), http.StatusSeeOther)
			return
		}

		// Save the session to update its life
		database.GetSessionStorage().SaveSession(session)

		ctx := context.WithValue(r.Context(), "user", user)
		ctx = context.WithValue(ctx, "session", session)

		// If authenticated, continue
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
