package web

import (
	"net/http"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

func HandleListSpaces(w http.ResponseWriter, r *http.Request) {
  tmpl, err := newTemplate("spaces.tmpl")
  if err != nil {
    log.Fatal().Msg(err.Error())
    w.WriteHeader(http.StatusInternalServerError)
    return
  }

  userId := chi.URLParam(r, "user_id")
  user, data := getCommonTemplateData(r)
  if userId != "" && user.Id != userId && !user.HasPermission(model.PermissionManageSpaces) {
    showPageForbidden(w, r)
    return
  }

  if userId != "" {
    forUser, err := database.GetInstance().GetUser(userId)
    if err != nil {
      log.Error().Msg(err.Error())
      w.WriteHeader(http.StatusInternalServerError)
      return
    }
    data["forUserId"] = userId
    data["forUserUsername"] = forUser.Username
  } else {
    data["forUserId"] = user.Id
    data["forUserUsername"] = ""
  }

  err = tmpl.Execute(w, data)
  if err != nil {
    log.Fatal().Msg(err.Error())
  }
}

func HandleSpacesCreate(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()

  tmpl, err := newTemplate("spaces-create-edit.tmpl")
  if err != nil {
    log.Fatal().Msg(err.Error())
    w.WriteHeader(http.StatusInternalServerError)
    return
  }

  user, data := getCommonTemplateData(r)
  data["preferredShell"] = user.PreferredShell
  data["isEdit"] = false

  userId := chi.URLParam(r, "user_id")
  if userId != "" && user.Id != userId && !user.HasPermission(model.PermissionManageSpaces) {
    showPageForbidden(w, r)
    return
  }

  if userId != "" {
    forUser, err := db.GetUser(userId)
    if err != nil {
      log.Error().Msg(err.Error())
      w.WriteHeader(http.StatusInternalServerError)
      return
    }
    data["forUserUsername"] = forUser.Username
  } else {
    data["forUserUsername"] = ""
  }

  data["forUserId"] = userId

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

  if space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
    showPageForbidden(w, r)
    return
  }

  data["isEdit"] = true
  data["preferredShell"] = ""
  data["templateList"], err = db.GetTemplateOptionList()
  if err != nil {
    log.Error().Msg(err.Error())
    w.WriteHeader(http.StatusInternalServerError)
    return
  }

  data["spaceId"] = spaceId

  err = tmpl.Execute(w, data)
  if err != nil {
    log.Fatal().Msg(err.Error())
  }
}
