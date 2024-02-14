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
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Validate
  if !validate.Email(request.Email) || !validate.Password(request.Password) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "invalid email or password"})
    return
  }

  // Get the user & check the password
  db := database.GetInstance()
  user, err := db.GetUserByEmail(request.Email)
  if err != nil || !user.CheckPassword(request.Password) {
    rest.SendJSON(http.StatusUnauthorized, w, ErrorResponse{Error: "invalid email or password"})
    return
  }

  // Update the last login time
  now := time.Now().UTC()
  user.LastLoginAt = &now
  err = db.SaveUser(user)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // If web auth then create a session
  var session *model.Session
  if r.URL.Path == "/api/v1/auth/web" {
    session = model.NewSession(r, user.Id)
    err = database.GetCacheInstance().SaveSession(session)
    if err != nil {
      rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
      return
    }

    cookie := &http.Cookie{
      Name: model.WEBUI_SESSION_COOKIE,
      Value: session.Id,
      Path: "/",
      HttpOnly: true,
      Secure: false,
      SameSite: http.SameSiteLaxMode,
    }

    http.SetCookie(w, cookie)
  }

  // Return the authentication token
  rest.SendJSON(http.StatusOK, w, struct {
    Status bool `json:"status"`
    Token string `json:"token"`
  }{
    Status: true,
    Token: "session-" + session.Id,
  })
}
