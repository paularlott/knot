package web

import (
	"net/http"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/spf13/viper"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

func HandleListSpaces(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()

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
    forUser, err := db.GetUser(userId)
    if err != nil {
      log.Error().Msg(err.Error())
      w.WriteHeader(http.StatusInternalServerError)
      return
    }
    data["forUserId"] = userId
    data["max_spaces"] = forUser.MaxSpaces
    data["forUserUsername"] = forUser.Username
  } else {
    data["forUserId"] = user.Id
    data["max_spaces"] = user.MaxSpaces
    data["forUserUsername"] = ""
  }

  // Get the number of spaces for the user
  spaces, err := db.GetSpacesForUser(data["forUserId"].(string))
  if err != nil {
    log.Fatal().Msg(err.Error())
  }
  data["num_spaces"] = len(spaces)

  data["wildcard_domain"] = viper.GetString("server.wildcard_domain")

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

  var maxSpaces int
  var usingUserId string

  if userId != "" {
    forUser, err := db.GetUser(userId)
    if err != nil {
      log.Error().Msg(err.Error())
      w.WriteHeader(http.StatusInternalServerError)
      return
    }
    data["forUserUsername"] = forUser.Username
    maxSpaces = forUser.MaxSpaces
    usingUserId = userId
  } else {
    data["forUserUsername"] = ""
    maxSpaces = user.MaxSpaces
    usingUserId = user.Id
  }

  data["forUserId"] = userId

  // Get the number of spaces for the user
  if maxSpaces > 0 {
    spaces, err := db.GetSpacesForUser(usingUserId)
    if err != nil {
      log.Error().Msg(err.Error())
      w.WriteHeader(http.StatusInternalServerError)
    }

    if len(spaces) >= maxSpaces {
      // Redirect to /spaces
      http.Redirect(w, r, "/spaces", http.StatusSeeOther)
    }
  }

  data["templateList"], err = getTemplateOptionList(user)
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
  data["templateList"], err = getTemplateOptionList(user)
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

type templateOptionList struct {
  Id string
  Name string `json:"role_name"`
}

func getTemplateOptionList(user *model.User) (*[]templateOptionList, error) {
  optionList := []templateOptionList{}

  templates, err := database.GetInstance().GetTemplates()
  if err != nil {
    return nil, err
  }

  for _, template := range templates {
    // If template doesn't have groups or one of the template groups is in the user's groups then add to optionList
    if len(template.Groups) == 0 || user.HasAnyGroup(&template.Groups) {
      optionList = append(optionList, templateOptionList{
        Id:   template.Id,
        Name: template.Name,
      })
    }
  }

  return &optionList, nil
}

