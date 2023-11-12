package middleware

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/database"
)

var (
  hasUsers bool
)

func InitializeAuth() {
  // Test if there's users present in the system
  db := database.GetInstance()
  userCount, err := db.GetUserCount()

  if userCount > 0 || err != nil {
    hasUsers = true
  } else {
    hasUsers = false
  }
}

func SetHasUsers() {
  hasUsers = true
}

func Auth(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    if(hasUsers) {
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