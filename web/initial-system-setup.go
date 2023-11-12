package web

import (
	"net/http"

	"github.com/paularlott/knot/database"
	"github.com/rs/zerolog/log"
)

func HandleInitialSystemSetupPage(w http.ResponseWriter, r *http.Request) {

  // If there's users then don't allow this to run, redirect to login
  db := database.GetInstance()
  userCount, err := db.GetUserCount()
  if userCount > 0 || err != nil {
    http.Redirect(w, r, "/login", http.StatusSeeOther)
  } else {
    tmpl, err := newTemplate("page-initial-system-setup.tmpl")
    if err != nil {
      w.WriteHeader(http.StatusInternalServerError)
      return
    }

    err = tmpl.Execute(w, nil)
    if err != nil {
      log.Fatal().Msg(err.Error())
    }
  }
}
