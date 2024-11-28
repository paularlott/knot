package apiv1

import (
	"fmt"
	"net/http"
	"time"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/rest"

	"github.com/go-chi/chi/v5"
)

func HandleGetSessions(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	sessions, err := database.GetCacheInstance().GetSessionsForUser(user.Id)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Build a json array of session data to return to the client
	sessionData := make([]struct {
		Id           string    `json:"session_id"`
		Ip           string    `json:"ip"`
		Current      bool      `json:"current"`
		ExpiresAfter time.Time `json:"expires_at"`
		UserAgent    string    `json:"user_agent"`
	}, len(sessions))

	currentSession := r.Context().Value("session").(*model.Session)

	for i, session := range sessions {
		sessionData[i].Id = session.Id
		sessionData[i].Ip = session.Ip
		sessionData[i].ExpiresAfter = session.ExpiresAfter
		sessionData[i].UserAgent = session.UserAgent
		sessionData[i].Current = currentSession != nil && currentSession.Id == session.Id
	}

	rest.SendJSON(http.StatusOK, w, r, sessionData)
}

func HandleDeleteSessions(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	// Load the session if not found or doesn't belong to the user then treat both as not found
	session, err := database.GetCacheInstance().GetSession(chi.URLParam(r, "session_id"))
	if err != nil || session.UserId != user.Id {
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: fmt.Sprintf("token %s not found", chi.URLParam(r, "session_id"))})
		return
	}

	// Delete the session
	err = database.GetCacheInstance().DeleteSession(session)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
}
