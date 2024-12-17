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
		data["forUserUsername"] = forUser.Username
	} else {
		data["forUserId"] = user.Id
		data["forUserUsername"] = ""
	}

	data["wildcard_domain"] = viper.GetString("server.wildcard_domain")

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Fatal().Msg(err.Error())
	}
}

func HandleSpacesCreate(w http.ResponseWriter, r *http.Request) {
	db := database.GetInstance()

	templateId := chi.URLParam(r, "template_id")

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

	var maxSpaces uint32
	var usingUserId string
	var forUser *model.User

	if userId != "" {
		forUser, err = db.GetUser(userId)
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
		forUser = user
	}

	data["forUserId"] = userId
	data["templateId"] = templateId

	// Get the number of spaces for the user
	if maxSpaces > 0 {
		spaces, err := db.GetSpacesForUser(usingUserId)
		if err != nil {
			log.Error().Msg(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
		}

		if uint32(len(spaces)) >= maxSpaces {
			http.Redirect(w, r, "/space-quota-reached", http.StatusSeeOther)
		}
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
	Id   string
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
		if user.HasPermission(model.PermissionManageTemplates) || len(template.Groups) == 0 || user.HasAnyGroup(&template.Groups) {
			optionList = append(optionList, templateOptionList{
				Id:   template.Id,
				Name: template.Name,
			})
		}
	}

	return &optionList, nil
}
