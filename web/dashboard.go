package web

import (
	"net/http"

	"github.com/paularlott/knot/middleware"
	"github.com/rs/zerolog/log"
)

func HandleDashboardPage(w http.ResponseWriter, r *http.Request) {

  tmpl, err := newTemplate("page-dashboard.tmpl")
  if err != nil {
    log.Fatal().Msg(err.Error())
    w.WriteHeader(http.StatusInternalServerError)
    return
  }


  data := map[string]interface{}{
    "username": middleware.User.Username,
  }

  err = tmpl.Execute(w, data)
  if err != nil {
    log.Fatal().Msg(err.Error())
  }
}
