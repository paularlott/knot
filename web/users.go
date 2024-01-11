package web

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

func HandleUserCreate(w http.ResponseWriter, r *http.Request) {
  tmpl, err := newTemplate("users-create-edit.tmpl")
  if err != nil {
    log.Fatal().Msg(err.Error())
    w.WriteHeader(http.StatusInternalServerError)
    return
  }

  _, data := getCommonTemplateData(r)
  data["isEdit"] = false

  err = tmpl.Execute(w, data)
  if err != nil {
    log.Fatal().Msg(err.Error())
  }
}

func HandleUserEdit(w http.ResponseWriter, r *http.Request) {
  _, data := getCommonTemplateData(r)
  userId := chi.URLParam(r, "user_id")

  tmpl, err := newTemplate("users-create-edit.tmpl")
  if err != nil {
    log.Fatal().Msg(err.Error())
    w.WriteHeader(http.StatusInternalServerError)
    return
  }

  data["isEdit"] = true
  data["user"] = map[string]interface{}{
    "id":       userId,
  }

  err = tmpl.Execute(w, data)
  if err != nil {
    log.Fatal().Msg(err.Error())
  }
}
