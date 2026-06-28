package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/middleware"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
)

// HandleSSE handles Server-Sent Events connections for real-time updates
func HandleSSE(w http.ResponseWriter, r *http.Request) {
	logger := log.WithGroup("sse")

	// Check authentication
	if middleware.HasUsers {
		var userId string
		var sessionId string
		// For cookie-authenticated streams, hold the session so the open stream
		// can act as a presence heartbeat and keep it from expiring while the
		// tab is open (see the refresh ticker in the event loop below).
		var refreshSession *model.Session

		db := database.GetInstance()
		store := database.GetSessionStorage()

		// Check for Authorization header first
		authorization := r.Header.Get("Authorization")
		if authorization != "" {
			bearer := middleware.GetBearerToken(w, r)
			if bearer == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			token, _ := db.GetToken(bearer)
			if token == nil || token.IsDeleted {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			userId = token.UserId
			sessionId = token.Id

			// Extend token life
			expiresAfter := time.Now().Add(model.MaxTokenAge)
			token.ExpiresAfter = expiresAfter.UTC()
			token.UpdatedAt = hlc.Now()
			db.SaveToken(token)
			if transport := service.GetTransport(); transport != nil {
				transport.GossipToken(token)
			}
	} else {
		// Get session from cookie
		session, err := middleware.GetSessionFromCookie(r)
		if err != nil {
			logger.Error("failed to get session", "error", err)
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}
		if session == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
			if session.ExpiresAfter.Before(time.Now().UTC()) || session.IsDeleted {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			userId = session.UserId
			sessionId = session.Id
			refreshSession = session

			// Extend session life
			session.UpdatedAt = hlc.Now()
			session.ExpiresAfter = time.Now().Add(model.SessionExpiryDuration).UTC()
			if err := store.SaveSession(session); err != nil {
				logger.Error("failed to save session", "error", err, "session_id", session.Id)
			} else {
				if transport := service.GetTransport(); transport != nil {
					transport.GossipSession(session)
				}
			}
		}

		// Verify user exists and is active. A load error is transient (store
		// unreachable), not unauthorized — return 503 so the EventSource
		// reconnects instead of the client treating it as a logout.
		user, err := db.GetUser(userId)
		if err != nil {
			logger.Error("failed to load user for SSE, treating as transient", "error", err, "user_id", userId)
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}
		if !user.Active || user.IsDeleted {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Set SSE headers. Note: do NOT set "Connection: keep-alive" — it's a
		// hop-by-hop header that is forbidden under HTTP/2 (RFC 7540 §8.1.2.2)
		// and can trigger ERR_HTTP2_PROTOCOL_ERROR; it's also redundant on
		// HTTP/1.1 where keep-alive is already the default.
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

		// Flush headers immediately
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}
		flusher.Flush()

		// Register client with hub
		hub := sse.GetHub()
		client := hub.NewClient(userId, sessionId)
		defer client.Close()

		logger.Debug("SSE client connected", "user_id", userId, "session_id", sessionId)

		// Send initial connected event
		fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"ok\"}\n\n")
		flusher.Flush()

		// Create a context that cancels when the client disconnects
		ctx := r.Context()

		// Main event loop. Session validity is enforced by:
		//   - The API middleware on every polling/API call (returns 401 → client
		//     redirects to /logout).
		//   - sse.InvalidateSession when a user explicitly logs out or a cluster
		//     node deletes the session (sends auth:required via the hub).
		// A periodic in-SSE session checker was removed because it was a second
		// source of truth that caused false logouts on transient store issues.
		keepAlive := time.NewTicker(5 * time.Second)
		defer keepAlive.Stop()

		// An open stream means the user is present, so periodically extend the
		// cookie session well within its expiry window. Without this, a user
		// who reads/edits a page without triggering API calls (e.g. a long
		// template edit) would have their session lapse and be logged out —
		// losing in-progress work — on their next request.
		sessionRefresh := time.NewTicker(model.SessionExpiryDuration / 4)
		defer sessionRefresh.Stop()

		for {
			select {
			case <-ctx.Done():
				return

			case <-sessionRefresh.C:
				if refreshSession != nil {
					// Re-read so we don't resurrect a session that was logged out
					// or deleted in another tab, and so we extend the latest copy.
					current, err := store.GetSession(refreshSession.Id)
					if err == nil && current != nil && !current.IsDeleted {
						current.UpdatedAt = hlc.Now()
						current.ExpiresAfter = time.Now().Add(model.SessionExpiryDuration).UTC()
						if err := store.SaveSession(current); err != nil {
							logger.Error("failed to refresh session", "error", err, "session_id", current.Id)
						} else if transport := service.GetTransport(); transport != nil {
							transport.GossipSession(current)
						}
					}
				}

			case <-keepAlive.C:
				_, err := fmt.Fprintf(w, ": keep-alive\n\n")
				if err != nil {
					return
				}
				flusher.Flush()

			case data, ok := <-client.Send():
				if !ok {
					return
				}

				_, err := fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
				if err != nil {
					logger.Warn("SSE write error", "error", err)
					return
				}
				flusher.Flush()
			}
		}
	} else {
		// No users in system, just return unauthorized
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}
}
