package web

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/paularlott/knot/middleware"
	"github.com/rs/zerolog/log"
)

func HandleSimplePage(w http.ResponseWriter, r *http.Request) {
  tmpl, err := newTemplate(fmt.Sprintf("page-%s.tmpl", strings.ReplaceAll(r.URL.Path[1:], "/", "_")))
  if err != nil {
    log.Fatal().Msg(err.Error())
    w.WriteHeader(http.StatusInternalServerError)
    return
  }

  data := map[string]interface{}{
    "username": middleware.User.Username,
    "IsAdmin": middleware.User.IsAdmin,
  }

  err = tmpl.Execute(w, data)
  if err != nil {
    log.Fatal().Msg(err.Error())
  }
}
