package middleware

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/rest"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"golang.org/x/net/context"
)

var (
	HasUsers bool
)

func Initialize() {
	if !viper.GetBool("server.is_remote") {
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

func returnUnauthorized(w http.ResponseWriter) {
	rest.SendJSON(http.StatusUnauthorized, w, struct {
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
		returnUnauthorized(w)
		return ""
	}

	return bearer
}

func ApiAuth(next http.Handler) http.Handler {
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
					returnUnauthorized(w)
					return
				}

				token, _ := db.GetToken(bearer)
				if token == nil {
					returnUnauthorized(w)
					return
				}

				userId = token.UserId

				// Save the token to extend its life
				db.SaveToken(token)

				// Add the token to the context
				ctx = context.WithValue(r.Context(), "access_token", token)

				// If remote then setup the client
				if viper.GetBool("server.is_remote") {
					// Create the API client
					client := apiclient.NewRemoteToken(viper.GetString("server.remote_token"))
					client.AppendUserAgent("(token " + token.Id + ")")

					// If the token doesn't have a session or if the session can't be loaded create a new session
					session, err := cache.GetSession(token.SessionId)
					if err != nil || session == nil {

						// Create a remote session for the user of the token
						remoteSessionId, _, err := client.RemoteCreateUserSession(userId)
						if err != nil {
							log.Error().Msgf("failed to create remote session: %s", err.Error())
							returnUnauthorized(w)
							return
						}

						// Create a local session to tie the remote and local together
						session = model.NewSession(r, userId, remoteSessionId)

						token.SessionId = session.Id
						db.SaveToken(token)
					}

					// Save the session to keep it alive
					cache.SaveSession(session)

					// Change the API client to use the session
					client.UseSessionCookie(true).SetAuthToken(session.RemoteSessionId)
					ctx = context.WithValue(ctx, "remote_client", client)
				}
			} else {

				// Get the session
				session := GetSessionFromCookie(r)
				if session == nil {
					returnUnauthorized(w)
					return
				}

				userId = session.UserId

				// Save the session to extend its life
				cache.SaveSession(session)

				// Add the session to the context
				ctx = context.WithValue(r.Context(), "session", session)

				// If remote then setup the client
				if viper.GetBool("server.is_remote") {
					client := apiclient.NewRemoteSession(session.RemoteSessionId)
					ctx = context.WithValue(ctx, "remote_client", client)
				}
			}

			// Get the user
			user, err := db.GetUser(userId)
			if err != nil || !user.Active {
				returnUnauthorized(w)
				return
			}

			// Add the user to the context
			ctx = context.WithValue(ctx, "user", user)
		}

		// If authenticated, continue
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func ApiPermissionManageTemplates(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value("user").(*model.User)
		if !user.HasPermission(model.PermissionManageTemplates) {
			rest.SendJSON(http.StatusForbidden, w, ErrorResponse{Error: "No permission to manage templates"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func ApiPermissionManageVolumes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value("user").(*model.User)
		if !user.HasPermission(model.PermissionManageVolumes) {
			rest.SendJSON(http.StatusForbidden, w, ErrorResponse{Error: "No permission to manage volumes"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func ApiPermissionManageUsers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if HasUsers {
			user := r.Context().Value("user").(*model.User)
			if HasUsers && !user.HasPermission(model.PermissionManageUsers) {
				rest.SendJSON(http.StatusForbidden, w, ErrorResponse{Error: "No permission to manage users"})
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func ApiPermissionManageUsersOrSelf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userId := chi.URLParam(r, "user_id")
		user := r.Context().Value("user").(*model.User)
		if !user.HasPermission(model.PermissionManageUsers) && user.Id != userId {
			rest.SendJSON(http.StatusForbidden, w, ErrorResponse{Error: "No permission to manage users"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func WebAuth(next http.Handler) http.Handler {
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

func AgentAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		spaceId := chi.URLParam(r, "space_id")
		authorization := r.Header.Get("Authorization")

		// Fetch the registered space, if not found then fail
		state, err := database.GetCacheInstance().GetAgentState(spaceId)
		if err != nil || authorization == "" || state == nil {
			returnUnauthorized(w)
			return
		}

		// Get the auth token
		var token string
		fmt.Sscanf(authorization, "Bearer %s", &token)
		if len(token) != 36 || token != state.AccessToken {
			returnUnauthorized(w)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func RemoteServerAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// Get the auth token
		var bearer string
		fmt.Sscanf(r.Header.Get("Authorization"), "Bearer %s", &bearer)
		if bearer != viper.GetString("server.remote_token") || viper.GetString("server.remote_token") == "" {
			returnUnauthorized(w)
			return
		}

		next.ServeHTTP(w, r)
	})
}
