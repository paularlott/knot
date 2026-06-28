package middleware

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/crypt"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/paularlott/knot/internal/log"
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
		log.WithError(err).Fatal("failed to get user count:")
	}

	if hasUsers || err != nil {
		HasUsers = true
	} else {
		HasUsers = false
	}
}

func returnUnauthorized(w http.ResponseWriter, r *http.Request) {
	rest.WriteResponse(http.StatusUnauthorized, w, r, struct {
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
		logger := log.WithGroup("auth")
		ctx := r.Context()

		// Check if user already in context (from MuxClient)
		if userVal := ctx.Value("user"); userVal != nil {
			if user, ok := userVal.(*model.User); ok && user != nil {
				logger.Trace("context user authenticated", "user_id", user.Id)
				next.ServeHTTP(w, r)
				return
			}
		}

		// Check if this is an internal cluster forwarded request
		cfgCheck := config.GetServerConfig()
		if clusterKey := r.Header.Get("X-Cluster-Key"); clusterKey != "" && clusterKey == cfgCheck.Cluster.Key {
			// Authenticated as cluster-internal, look up the forwarded user
			forwardedUserId := r.Header.Get("X-Cluster-User-Id")
			if forwardedUserId != "" {
				db := database.GetInstance()
				user, err := db.GetUser(forwardedUserId)
				if err == nil && user != nil && user.Active && !user.IsDeleted {
					ctx = context.WithValue(ctx, "user", user)
					apiVersion := r.Header.Get("X-Knot-Api-Version")
					if apiVersion == "" {
						apiVersion = "2025-03-10"
					}
					ctx = context.WithValue(ctx, "api_version", apiVersion)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
			returnUnauthorized(w, r)
			return
		}

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

				// Check if this is an agent token
				if crypt.IsAgentToken(bearer) {
					cfg := config.GetServerConfig()

					// Extract space ID from token
					spaceId := crypt.ExtractSpaceIdFromToken(bearer)
					if spaceId == "" {
						logger.Debug("invalid agent token format")
						returnUnauthorized(w, r)
						return
					}

					// Look up space to get userId
					space, err := db.GetSpace(spaceId)
					if err != nil {
						returnUnauthorized(w, r)
						return
					}

					// Validate token signature
					if !crypt.ValidateAgentToken(bearer, spaceId, space.UserId, cfg.Zone, cfg.EncryptionKey) {
						logger.Debug("invalid agent token signature")
						returnUnauthorized(w, r)
						return
					}

					userId = space.UserId

					// Add space ID to context
					ctx = context.WithValue(ctx, "space_id", spaceId)
				} else {
					// Regular API token
					token, _ := db.GetToken(bearer)
					if token == nil || token.IsDeleted {
						returnUnauthorized(w, r)
						return
					}

					userId = token.UserId

					// Save the token to extend its life
					expiresAfter := time.Now().Add(model.MaxTokenAge)
					token.ExpiresAfter = expiresAfter.UTC()
					token.UpdatedAt = hlc.Now()
					db.SaveToken(token)
					service.GetTransport().GossipToken(token)

					// Add the token to the context
					ctx = context.WithValue(r.Context(), "access_token", token)
				}
			} else {
				// Get the session
				session, err := GetSessionFromCookie(r)
				if err != nil {
					logger.Error("failed to get session", "error", err)
					rest.WriteResponse(http.StatusServiceUnavailable, w, r, struct {
						Error string `json:"error"`
					}{
						Error: "Session storage temporarily unavailable",
					})
					return
				}
				if session == nil {
					logger.Debug("session not found")
					returnUnauthorized(w, r)
					return
				}
				if session.ExpiresAfter.Before(time.Now().UTC()) {
					logger.Debug("session expired", "session_id", session.Id, "expires", session.ExpiresAfter)
					returnUnauthorized(w, r)
					return
				}
				if session.IsDeleted {
					logger.Debug("session deleted", "session_id", session.Id)
					returnUnauthorized(w, r)
					return
				}

				userId = session.UserId

				// Save the session to extend its life
				session.UpdatedAt = hlc.Now()
				session.ExpiresAfter = time.Now().Add(model.SessionExpiryDuration).UTC()
				if err := store.SaveSession(session); err != nil {
					logger.Error("failed to save session", "error", err, "session_id", session.Id)
				} else {
					service.GetTransport().GossipSession(session)
				}

				// Add the session to the context
				ctx = context.WithValue(r.Context(), "session", session)
			}

			// Get the user. A load error means the backing store is unreachable,
			// not that the request is unauthenticated — return 503 (as the session
			// lookup above does) so the client retries rather than being logged
			// out on a transient database hiccup. Genuinely deactivated/deleted
			// users are signed out via session invalidation (RemoveUsersSessions),
			// which the session.IsDeleted check already enforces.
			user, err := db.GetUser(userId)
			if err != nil {
				logger.Error("failed to load user, treating as transient", "error", err, "user_id", userId)
				rest.WriteResponse(http.StatusServiceUnavailable, w, r, struct {
					Error string `json:"error"`
				}{
					Error: "User store temporarily unavailable",
				})
				return
			}
			if !user.Active || user.IsDeleted {
				logger.Debug("user inactive or deleted", "user_id", userId)
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

		// Enforce token scopes. A token with a non-empty Scopes slice may
		// only reach endpoints covered by one of its scopes. Session cookies,
		// agent tokens, and unscoped API tokens bypass this — only scoped
		// user-issued API tokens are restricted.
		if token, _ := ctx.Value("access_token").(*model.Token); token != nil && len(token.Scopes) > 0 {
			if !tokenScopeAllows(token.Scopes, r.URL.Path) {
				rest.WriteResponse(http.StatusForbidden, w, r.WithContext(ctx), ErrorResponse{
					Error: "token scopes do not permit this endpoint",
				})
				return
			}
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
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: msg})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func ApiPermissionManageTemplates(next http.HandlerFunc) http.HandlerFunc {
	cfg := config.GetServerConfig()
	if cfg.LeafNode {
		return next
	}

	return checkPermission(next, model.PermissionManageTemplates, "No permission to manage templates")
}

func ApiPermissionManageVolumes(next http.HandlerFunc) http.HandlerFunc {
	cfg := config.GetServerConfig()
	if cfg.LeafNode {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value("user").(*model.User)
		if !user.HasPermission(model.PermissionManageVolumes) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to manage volumes"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func ApiPermissionUsePools(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionUsePools, "No permission to use pools")
}

func ApiPermissionManageVariables(next http.HandlerFunc) http.HandlerFunc {
	cfg := config.GetServerConfig()
	if cfg.LeafNode {
		return next
	}

	return checkPermission(next, model.PermissionManageVariables, "No permission to manage variables")
}

func ApiPermissionUseTunnels(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionUseTunnels, "No permission to use tunnels")
}

func ApiPermissionViewAuditLogs(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionViewAuditLogs, "No permission to view audit logs")
}

func ApiPermissionDownloadAuditLogs(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionDownloadAuditLogs, "No permission to download audit logs")
}

func ApiPermissionViewClusterInfo(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionClusterInfo, "No permission to view cluster info")
}

func ApiPermissionManageUsers(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if HasUsers {
			user := r.Context().Value("user").(*model.User)
			if HasUsers && !user.HasPermission(model.PermissionManageUsers) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to manage users"})
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
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to manage users"})
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
			rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid user ID"})
			return
		}

		user := r.Context().Value("user").(*model.User)
		if !user.HasPermission(model.PermissionManageUsers) && user.Id != userId {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to manage users"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func ApiPermissionUseSpaces(next http.HandlerFunc) http.HandlerFunc {
	cfg := config.GetServerConfig()
	if cfg.LeafNode {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value("user").(*model.User)
		if !user.HasPermission(model.PermissionManageSpaces) && !user.HasPermission(model.PermissionUseSpaces) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to manage or use spaces"})
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

func ApiPermissionRunCommands(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionRunCommands, "No permission to run commands")
}

func ApiPermissionCopyFiles(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionCopyFiles, "No permission to copy files")
}

func ApiPermissionUseMCPServer(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionUseMCPServer, "No permission to use the MCP server")
}

func ApiPermissionUseWebAssistant(next http.HandlerFunc) http.HandlerFunc {
	return checkPermission(next, model.PermissionUseWebAssistant, "No permission to use web assistant")
}

func ApiPermissionManageScripts(next http.HandlerFunc) http.HandlerFunc {
	cfg := config.GetServerConfig()
	if cfg.LeafNode {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value("user").(*model.User)
		if !user.HasPermission(model.PermissionManageScripts) && !user.HasPermission(model.PermissionManageOwnScripts) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to manage scripts"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// TokenScopesRequired wraps a handler with token-scope enforcement. If the
// request was authenticated via a scoped API token (i.e. the token in context
// has a non-empty Scopes slice), the request path must be covered by at least
// one of the token's scopes. Tokens with no scopes (nil/empty — the default
// for all pre-scopes tokens) are unrestricted. Session cookies and agent
// tokens bypass this check entirely.
func TokenScopesRequired(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token, _ := r.Context().Value("access_token").(*model.Token); token != nil && len(token.Scopes) > 0 {
			if !tokenScopeAllows(token.Scopes, r.URL.Path) {
				rest.WriteResponse(http.StatusForbidden, w, r, struct {
					Error string `json:"error"`
				}{
					Error: "token scopes do not permit this endpoint",
				})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// tokenScopeAllowedPaths maps each known scope to the URL prefix(es) it
// covers. Add entries here as new scopes are introduced.
var tokenScopeAllowedPaths = map[string][]string{
	model.ScopeMethods: {"/api/methods"},
	model.ScopeMCP:     {"/mcp"},
}

func tokenScopeAllows(scopes []string, path string) bool {
	for _, scope := range scopes {
		for _, prefix := range tokenScopeAllowedPaths[scope] {
			if strings.HasPrefix(path, prefix) {
				return true
			}
		}
	}
	return false
}

// OptionalWebAuth injects the authenticated user into the context if a valid
// session cookie is present, but never redirects — unauthenticated requests
// pass through with no user in context.
func OptionalWebAuth(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, err := GetSessionFromCookie(r)
		if err == nil && session != nil && !session.IsDeleted && session.ExpiresAfter.After(time.Now().UTC()) {
			db := database.GetInstance()
			user, err := db.GetUser(session.UserId)
			if err == nil && user.Active && !user.IsDeleted {
				ctx := context.WithValue(r.Context(), "user", user)
				r = r.WithContext(ctx)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func WebAuth(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := log.WithGroup("auth")

		// If no session then redirect to login
		session, err := GetSessionFromCookie(r)
		if err != nil {
			logger.Error("failed to get session", "error", err, "path", r.URL.Path)
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}
		if session == nil {
			logger.Debug("session not found", "path", r.URL.Path)
			DeleteSessionCookie(w)
			http.Redirect(w, r, "/login?redirect="+url.QueryEscape(r.URL.String()), http.StatusSeeOther)
			return
		}
		if session.ExpiresAfter.Before(time.Now().UTC()) {
			logger.Debug("session expired", "session_id", session.Id, "path", r.URL.Path, "expires", session.ExpiresAfter)
			DeleteSessionCookie(w)
			http.Redirect(w, r, "/login?redirect="+url.QueryEscape(r.URL.String()), http.StatusSeeOther)
			return
		}
		if session.IsDeleted {
			logger.Debug("session deleted", "session_id", session.Id, "path", r.URL.Path)
			DeleteSessionCookie(w)
			http.Redirect(w, r, "/login?redirect="+url.QueryEscape(r.URL.String()), http.StatusSeeOther)
			return
		}

		// Get the user from the session. A load error means the store is
		// unreachable, not that the user is unauthenticated — return 503 and
		// keep the session/cookie so a retry works, rather than logging the user
		// out (and losing in-progress work) on a transient database hiccup.
		// Deactivated/deleted users are signed out via session invalidation.
		db := database.GetInstance()
		user, err := db.GetUser(session.UserId)
		if err != nil {
			logger.Error("failed to load user, treating as transient", "error", err, "session_id", session.Id, "path", r.URL.Path)
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}
		if !user.Active || user.IsDeleted {
			logger.Debug("user inactive or deleted", "session_id", session.Id, "user_id", session.UserId, "path", r.URL.Path)
			DeleteSessionCookie(w)
			http.Redirect(w, r, "/login?redirect="+url.QueryEscape(r.URL.String()), http.StatusSeeOther)
			return
		}

		// Save the session to update its life
		session.UpdatedAt = hlc.Now()
		session.ExpiresAfter = time.Now().Add(model.SessionExpiryDuration).UTC()
		if err := database.GetSessionStorage().SaveSession(session); err != nil {
			logger.Error("failed to save session", "error", err, "session_id", session.Id)
		} else {
			service.GetTransport().GossipSession(session)
		}

		ctx := context.WithValue(r.Context(), "user", user)
		ctx = context.WithValue(ctx, "session", session)

		// If authenticated, continue
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
