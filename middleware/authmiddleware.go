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

      // Get the auth token
      var token string
      fmt.Sscanf(r.Header.Get("Authorization"), "Bearer %s", &token)

      // If token not 36 or 44 characters long then fail
      if len(token) != 36 && len(token) != 44 {
        returnUnauthorized(w)
        return
      }

      db := database.GetInstance()

      // If token starts session- then it's a session token
      if token[0:8] == "session-" {
        token = token[8:]

        Session, _ = db.GetSession(token)
        if Session == nil {
          returnUnauthorized(w)
          return
        }

        var err error
        User, err = db.GetUser(Session.UserId)
        if err != nil || !User.Active {
          returnUnauthorized(w)
          return
        }
      } else {
        Token, _ = db.GetToken(token)
        if Token == nil {
          returnUnauthorized(w)
          return
        }

        var err error
        User, err = db.GetUser(Token.UserId)
        if err != nil || !User.Active {
          returnUnauthorized(w)
          return
        }

        // Save the token to extend its life
        err = db.SaveToken(Token)
        if err != nil {
          returnUnauthorized(w)
          return
        }
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
