package web

import (
	"net/http"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

func HandleSpacesCreate(w http.ResponseWriter, r *http.Request) {
  tmpl, err := newTemplate("spaces-create-edit.tmpl")
  if err != nil {
    log.Fatal().Msg(err.Error())
    w.WriteHeader(http.StatusInternalServerError)
    return
  }

  user, data := getCommonTemplateData(r)
  data["preferredShell"] = user.PreferredShell
  data["isEdit"] = false

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

func HandleSpacesEdit(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()
  user, data := getCommonTemplateData(r)

  tmpl, err := newTemplate("spaces-create-edit.tmpl")
  if err != nil {
    log.Fatal().Msg(err.Error())
    w.WriteHeader(http.StatusInternalServerError)
    return
  }

  // Load the space
  spaceId := chi.URLParam(r, "space_id")
  space, err := db.GetSpace(spaceId)
  if err != nil {
    log.Error().Msg(err.Error())
    showPageNotFound(w, r)
    return
  }

  if space.UserId != user.Id {
    showPageForbidden(w, r)
    return
  }

  data["isEdit"] = true
  data["templateList"], err = db.GetTemplateOptionList()
  if err != nil {
    log.Error().Msg(err.Error())
    w.WriteHeader(http.StatusInternalServerError)
    return
  }

  data["spaceName"] = space.Name
  data["spaceId"] = space.Id
  if space.TemplateId == model.MANUAL_TEMPLATE_ID {
    data["templateId"] = ""
  } else {
    data["templateId"] = space.TemplateId
  }
  data["agentUrl"] = space.AgentURL
  data["preferredShell"] = space.Shell

  err = tmpl.Execute(w, data)
  if err != nil {
    log.Fatal().Msg(err.Error())
  }
}
