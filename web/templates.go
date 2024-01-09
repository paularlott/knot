package web

import (
	"net/http"

	"github.com/paularlott/knot/database/model"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

func HandleTemplateCreate(w http.ResponseWriter, r *http.Request) {
  user, data := getCommonTemplateData(r)
  if !user.HasPermission(model.PermissionManageTemplates) {
    showPageForbidden(w, r)
    return
  }

  tmpl, err := newTemplate("templates-create-edit.tmpl")
  if err != nil {
    log.Fatal().Msg(err.Error())
    w.WriteHeader(http.StatusInternalServerError)
    return
  }

  data["isEdit"] = false

  err = tmpl.Execute(w, data)
  if err != nil {
    log.Fatal().Msg(err.Error())
  }
}

func HandleTemplateEdit(w http.ResponseWriter, r *http.Request) {
  user, data := getCommonTemplateData(r)
  if !user.HasPermission(model.PermissionManageTemplates) {
    showPageForbidden(w, r)
    return
  }

  tmpl, err := newTemplate("templates-create-edit.tmpl")
  if err != nil {
    log.Fatal().Msg(err.Error())
    w.WriteHeader(http.StatusInternalServerError)
    return
  }

  data["isEdit"] = true
  data["templateId"] = chi.URLParam(r, "template_id")

  err = tmpl.Execute(w, data)
  if err != nil {
    log.Fatal().Msg(err.Error())
  }
}
