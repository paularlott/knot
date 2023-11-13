package apiv1

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/middleware"
	"github.com/paularlott/knot/util/rest"
)

func HandleGetSessions(w http.ResponseWriter, r *http.Request) {
  sessions, err := database.GetInstance().GetSessions(middleware.User.Id)
  if err != nil {
    w.WriteHeader(http.StatusInternalServerError)
    rest.SendJSON(w, ErrorResponse{Error: err.Error()})
    return
  }

  // Build a json array of the IP address and expire time from each session
  sessionData := make([]struct {
    Id string `json:"session_id"`
    Ip string `json:"ip"`
    ExpiresAfter time.Time `json:"expires_at"`
  }, len(sessions))

  for i, session := range sessions {
    sessionData[i].Id = session.Id
    sessionData[i].Ip = session.Ip
    sessionData[i].ExpiresAfter = session.ExpiresAfter
  }

  w.WriteHeader(http.StatusOK)
  rest.SendJSON(w, sessionData)
}

func HandleDeleteSessions(w http.ResponseWriter, r *http.Request) {

  // Load the sessions if not found or doesn't belong to the user then treat both as not found
  session, err := database.GetInstance().GetSession(chi.URLParam(r, "session_id"))
  if err != nil || session.UserId != middleware.User.Id {
    w.WriteHeader(http.StatusNotFound)
    rest.SendJSON(w, ErrorResponse{Error: fmt.Sprintf("Session %s not found", chi.URLParam(r, "session_id"))})
    return
  }

  // Delete the session
  err = database.GetInstance().DeleteSession(session)
  if err != nil {
    w.WriteHeader(http.StatusInternalServerError)
    rest.SendJSON(w, ErrorResponse{Error: err.Error()})
    return
  }

  w.WriteHeader(http.StatusOK)
}
