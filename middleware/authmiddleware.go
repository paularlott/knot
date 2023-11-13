package middleware

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/rest"
)

var (
  HasUsers bool
  Session *model.Session = nil
  Token *model.Token = nil
  User *model.User = nil
)

func Initialize() {
  // Test if there's users present in the system
  db := database.GetInstance()
  userCount, err := db.GetUserCount()

  if userCount > 0 || err != nil {
    HasUsers = true
  } else {
    HasUsers = false
  }
}

func returnUnauthorized(w http.ResponseWriter) {
  rest.SendJSON(http.StatusUnauthorized, w, struct {
    Error string `json:"error"`
  }{
    Error: "Authentication token is not valid",
  })
}

func ApiAuth(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

    // If there's no users in the system then we don't check for authentication
    if(HasUsers) {
      var userId string
      var err error

      db := database.GetInstance()

      // If have an Authorization header then we use that for authentication
      authorization := r.Header.Get("Authorization")
      if authorization != "" {

        // Get the auth token
        var token string
        fmt.Sscanf(authorization, "Bearer %s", &token)
        if len(token) != 36 {
          returnUnauthorized(w)
          return
        }

        Token, _ = db.GetToken(token)
        if Token == nil {
          returnUnauthorized(w)
          return
        }

        userId = Token.UserId

        // Save the token to extend its life
        db.SaveToken(Token)
      } else {

        // Get the session
        Session = GetSessionFromCookie(r)
        if Session == nil {
          returnUnauthorized(w)
          return
        }

        userId = Session.UserId

        // Save the session to extend its life
        db.SaveSession(Session)
      }

      // Get the user
      User, err = db.GetUser(userId)
      if err != nil || !User.Active {
        returnUnauthorized(w)
        return
      }
    }

    // If authenticated, continue
    next.ServeHTTP(w, r)
  })
}

func WebAuth(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

    // If no session then redirect to login
    Session = GetSessionFromCookie(r)
    if Session == nil {
      DeleteSessionCookie(w)
      http.Redirect(w, r, "/login?redirect=" + r.URL.EscapedPath(), http.StatusSeeOther)
      return
    }

    // Get the user from the session
    db := database.GetInstance()
    var err error
    User, err = db.GetUser(Session.UserId)
    if err != nil || !User.Active {
      DeleteSessionCookie(w)
      http.Redirect(w, r, "/login?redirect=" + r.URL.EscapedPath(), http.StatusSeeOther)
      return
    }

    // Ensure we save the session at the end of the request
    defer db.SaveSession(Session)

    // If authenticated, continue
    next.ServeHTTP(w, r)
  })
}
