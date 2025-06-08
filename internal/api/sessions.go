package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
)

type sessionResponse struct {
	Id           string    `json:"session_id"`
	Ip           string    `json:"ip"`
	Current      bool      `json:"current"`
	ExpiresAfter time.Time `json:"expires_at"`
	UserAgent    string    `json:"user_agent"`
}

func HandleGetSessions(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	sessions, err := database.GetSessionStorage().GetSessionsForUser(user.Id)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Build a json array of session data to return to the client
	sessionData := []*sessionResponse{}

	currentSession := r.Context().Value("session").(*model.Session)

	for _, session := range sessions {
		if session.ExpiresAfter.Before(time.Now().UTC()) {
			continue
		}

		sessionData = append(sessionData, &sessionResponse{
			Id:           session.Id,
			Ip:           session.Ip,
			Current:      currentSession != nil && currentSession.Id == session.Id,
			ExpiresAfter: session.ExpiresAfter,
			UserAgent:    session.UserAgent,
		})
	}

	rest.SendJSON(http.StatusOK, w, r, sessionData)
}

func HandleDeleteSessions(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	sessionId := r.PathValue("session_id")

	if !validate.Required(sessionId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid session ID"})
		return
	}

	store := database.GetSessionStorage()

	// Load the session if not found or doesn't belong to the user then treat both as not found
	session, err := store.GetSession(sessionId)
	if err != nil || session.UserId != user.Id {
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: fmt.Sprintf("token %s not found", sessionId)})
		return
	}

	// Expire the session
	session.UpdatedAt = time.Now().UTC()
	session.ExpiresAfter = time.Now().UTC().Add(-model.SessionExpiryDuration) // Set to a time in the past to expire it
	err = store.DeleteSession(session)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	service.GetTransport().GossipSession(session)

	w.WriteHeader(http.StatusOK)
}
