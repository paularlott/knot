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
			session := middleware.GetSessionFromCookie(r)
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

		// Verify user exists and is active
		user, err := db.GetUser(userId)
		if err != nil || !user.Active || user.IsDeleted {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Set SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
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

		// Start a goroutine to periodically check session validity
		sessionCheckDone := make(chan struct{})
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					// Check if session is still valid
					session := middleware.GetSessionFromCookie(r)
					if session == nil || session.IsDeleted || session.ExpiresAfter.Before(time.Now().UTC()) {
						// Session invalid, notify client
						fmt.Fprintf(w, "event: message\ndata: {\"type\":\"auth:required\"}\n\n")
						flusher.Flush()
						return
					}
				case <-ctx.Done():
					return
				case <-sessionCheckDone:
					return
				}
			}
		}()
		defer close(sessionCheckDone)

		// Main event loop
		keepAlive := time.NewTicker(5 * time.Second)
		defer keepAlive.Stop()
		for {
			select {
			case <-ctx.Done():
				return

			case <-keepAlive.C:
				// Send a keep-alive comment every 5 seconds to prevent proxy timeouts
				_, err := fmt.Fprintf(w, ": keep-alive\n\n")
				if err != nil {
					return
				}
				flusher.Flush()

			case data, ok := <-client.Send():
				if !ok {
					return
				}

				// Write the event
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
