package middleware

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/server_info"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
)

var (
	HasUsers bool
)

func Initialize() {
	if !server_info.IsLeaf {
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
	} else {
		// Server is a remote so assume users exist as remote server provides user information
		HasUsers = true
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
			cache := database.GetCacheInstance()

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

				// If remote then setup the client
				if server_info.IsLeaf {
					// Create a remote access client
					client := apiclient.NewRemoteToken(token.Id)
					client.AppendUserAgent("(token " + token.Id + ")")
					ctx = context.WithValue(ctx, "remote_client", client)
				}
			} else {

				// Get the session
				session := GetSessionFromCookie(r)
				if session == nil {
					returnUnauthorized(w, r)
					return
				}

				userId = session.UserId

				// Save the session to extend its life
				cache.SaveSession(session)

				// Add the session to the context
				ctx = context.WithValue(r.Context(), "session", session)

				// If remote then setup the client
				if server_info.IsLeaf {
					client := apiclient.NewRemoteSession(session.RemoteSessionId)
					ctx = context.WithValue(ctx, "remote_client", client)
				}
			}

			// Get the user
			user, err := db.GetUser(userId)
			if err != nil || !user.Active {
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

func checkPremission(next http.HandlerFunc, permission uint16, msg string) http.HandlerFunc {
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
	return checkPremission(next, model.PermissionManageTemplates, "No permission to manage templates")
}

func ApiPermissionManageVolumes(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value("user").(*model.User)
		if !server_info.RestrictedLeaf && !user.HasPermission(model.PermissionManageVolumes) {
			rest.SendJSON(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to manage volumes"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func ApiPermissionManageVariables(next http.HandlerFunc) http.HandlerFunc {
	return checkPremission(next, model.PermissionManageVariables, "No permission to manage variables")
}

func ApiPermissionUseTunnels(next http.HandlerFunc) http.HandlerFunc {
	return checkPremission(next, model.PermissionUseTunnels, "No permission to use tunnels")
}

func ApiPermissionViewAuditLogs(next http.HandlerFunc) http.HandlerFunc {
	return checkPremission(next, model.PermissionViewAuditLogs, "No permission to view audit logs")
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
		if !server_info.RestrictedLeaf && !user.HasPermission(model.PermissionManageSpaces) && !user.HasPermission(model.PermissionUseSpaces) {
			rest.SendJSON(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to manage or use spaces"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func ApiPermissionTransferSpaces(next http.HandlerFunc) http.HandlerFunc {
	return checkPremission(next, model.PermissionTransferSpaces, "No permission to transfer spaces")
}

func ApiPermissionManageGroups(next http.HandlerFunc) http.HandlerFunc {
	return checkPremission(next, model.PermissionManageGroups, "No permission to manage groups")
}

func ApiPermissionManageRoles(next http.HandlerFunc) http.HandlerFunc {
	return checkPremission(next, model.PermissionManageRoles, "No permission to manage roles")
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
		if err != nil || !user.Active {
			DeleteSessionCookie(w)
			http.Redirect(w, r, "/login?redirect="+r.URL.EscapedPath(), http.StatusSeeOther)
			return
		}

		// Save the session to update its life
		database.GetCacheInstance().SaveSession(session)

		ctx := context.WithValue(r.Context(), "user", user)
		ctx = context.WithValue(ctx, "session", session)

		// If authenticated, continue
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func LeafServerAuth(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// Get the auth token
		bearer := GetBearerToken(w, r)
		if bearer == "" {
			returnUnauthorized(w, r)
			return
		}
		if bearer != viper.GetString("server.shared_token") || viper.GetString("server.shared_token") == "" {

			// If leaf nodes are allowed to use API tokens then check for that
			if viper.GetBool("server.enable_leaf_api_tokens") {

				db := database.GetInstance()
				token, _ := db.GetToken(bearer)
				if token == nil {
					returnUnauthorized(w, r)
					return
				}

				// Save the token to extend its life
				db.SaveToken(token)

				// Save the user and token to the context
				ctx := context.WithValue(r.Context(), "access_token", token)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			returnUnauthorized(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}
