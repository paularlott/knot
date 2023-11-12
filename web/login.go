package web

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/database"
)

func HandleLoginPage(w http.ResponseWriter, r *http.Request) {

  // Check that there's at least one user in the system, if not then force us to the initial setup page
  db := database.GetInstance()
  userCount, err := db.GetUserCount()

fmt.Print("userCount: ", userCount, "\n")


  if userCount < 1 || err != nil {
    http.Redirect(w, r, "/initial-system-setup", http.StatusSeeOther)
  } else {

    w.WriteHeader(http.StatusOK)

//  w.Header().Set("Content-Type", "text/html; charset=utf-8")
    w.Write([]byte("Hello World! - login"))
  }
}
