package middleware

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
)

var (
  HasUsers bool
  Session *model.Session = nil
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

func ApiAuth(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    if(HasUsers) {
      fmt.Println("Users are present in the system so we should be checking for authentication")

    // Check if the user is authenticated
/*    // If not authenticated, redirect to login
      http.Redirect(w, r, "/login", http.StatusSeeOther)
      return
    } */

    } else {
      fmt.Println("No users are present in the system so we should not be checking for authentication")
    }

    fmt.Println("AuthMiddleware", r.URL.Path, r.Header.Get("Authorization"))

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
      http.Redirect(w, r, "/login", http.StatusSeeOther)
      return
    }

    // Get the user from the session
    db := database.GetInstance()
    var err error
    User, err = db.GetUser(Session.Values["user_id"].(string))
    if err != nil || !User.Active {
      DeleteSessionCookie(w)
      http.Redirect(w, r, "/login", http.StatusSeeOther)
      return
    }

    // Ensure we save the session at the end of the request
    defer db.SaveSession(Session)

    // If authenticated, continue
    next.ServeHTTP(w, r)
  })
}
