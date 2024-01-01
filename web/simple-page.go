package web

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
)

func HandleSimplePage(w http.ResponseWriter, r *http.Request) {
  tmpl, err := newTemplate(fmt.Sprintf("page-%s.tmpl", strings.ReplaceAll(r.URL.Path[1:], "/", "_")))
  if err != nil {
    log.Fatal().Msg(err.Error())
    w.WriteHeader(http.StatusInternalServerError)
    return
  } else if tmpl == nil {
    showPageNotFound(w, r)
    return
  }

  _, data := getCommonTemplateData(r)

  err = tmpl.Execute(w, data)
  if err != nil {
    log.Fatal().Msg(err.Error())
  }
}

func HandleHealthPage(w http.ResponseWriter, r *http.Request) {
  w.WriteHeader(http.StatusOK)
  fmt.Fprintf(w, "OK")
}
