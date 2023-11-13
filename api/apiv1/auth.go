package apiv1

import (
	"net/http"
	"time"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"
)

type AuthorizationRequest struct {
  Password string `json:"password"`
  Email string `json:"email"`
}

func HandleAuthorization(w http.ResponseWriter, r *http.Request) {
  request := AuthorizationRequest{}

  err := rest.BindJSON(w, r, &request)
  if err != nil {
    w.WriteHeader(http.StatusBadRequest)
    rest.SendJSON(w, ErrorResponse{Error: err.Error()})
    return
  }

  // Validate
  if !validate.Email(request.Email) || !validate.Password(request.Password) {
    w.WriteHeader(http.StatusBadRequest)
    rest.SendJSON(w, ErrorResponse{Error: "Invalid email or password"})
    return
  }

  // Get the user & check the password
  db := database.GetInstance()
  user, err := db.GetUserByEmail(request.Email)
  if err != nil || !user.CheckPassword(request.Password) {
    w.WriteHeader(http.StatusUnauthorized)
    rest.SendJSON(w, ErrorResponse{Error: "Invalid email or password"})
    return
  }

  // Update the last login time
  user.LastLoginAt = time.Now().UTC()
  err = db.SaveUser(user)
  if err != nil {
    w.WriteHeader(http.StatusInternalServerError)
    rest.SendJSON(w, ErrorResponse{Error: err.Error()})
    return
  }

  // If web auth then create a session
  var session *model.Session
  if r.URL.Path == "/api/v1/auth/web" {
    session = model.NewSession(r, user.Id)
    err = db.SaveSession(session)
    if err != nil {
      w.WriteHeader(http.StatusInternalServerError)
      rest.SendJSON(w, ErrorResponse{Error: err.Error()})
      return
    }

    cookie := &http.Cookie{
      Name: model.WEBUI_SESSION_COOKIE,
      Value: session.Id,
      Path: "/",
      HttpOnly: true,
      Secure: false,
      SameSite: http.SameSiteStrictMode,
    }

    http.SetCookie(w, cookie)
  }

  // Return the authentication token
  w.WriteHeader(http.StatusOK)
  rest.SendJSON(w, struct {
    Status bool `json:"status"`
    Token string `json:"token"`
  }{
    Status: true,
    Token: "session-" + session.Id,
  })
}
