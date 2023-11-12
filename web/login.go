package web

import (
	"net/http"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/middleware"
	"github.com/rs/zerolog/log"
)

func HandleLoginPage(w http.ResponseWriter, r *http.Request) {

  if !middleware.HasUsers {
    http.Redirect(w, r, "/initial-system-setup", http.StatusSeeOther)
  } else {

    // If session present then redirect to dashboard
    if middleware.Session != nil {
      http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
      return
    }

    tmpl, err := newTemplate("page-login.tmpl")
    if err != nil {
      log.Fatal().Msg(err.Error())
      w.WriteHeader(http.StatusInternalServerError)
      return
    }

    data := map[string]interface{}{
      "version": build.Version + " (" + build.Date + ")",
    }

    err = tmpl.Execute(w, data)
    if err != nil {
      log.Fatal().Msg(err.Error())
    }
  }
}
