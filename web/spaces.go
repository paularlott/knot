package web

import (
	"net/http"

	"github.com/paularlott/knot/database"

	"github.com/rs/zerolog/log"
)

func HandleSpacesCreate(w http.ResponseWriter, r *http.Request) {
  tmpl, err := newTemplate("page-spaces_create.tmpl")
  if err != nil {
    log.Fatal().Msg(err.Error())
    w.WriteHeader(http.StatusInternalServerError)
    return
  }

  _, data := getCommonTemplateData(r)

  db := database.GetInstance()
  data["templateList"], err = db.GetTemplateOptionList()
  if err != nil {
    log.Error().Msg(err.Error())
    w.WriteHeader(http.StatusInternalServerError)
    return
  }

  err = tmpl.Execute(w, data)
  if err != nil {
    log.Fatal().Msg(err.Error())
  }
}
