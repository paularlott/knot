package web

import (
	"net/http"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/util/validate"
	"github.com/spf13/viper"

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

	userId := r.PathValue("user_id")
	if userId != "" && !validate.UUID(userId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	user, data := getCommonTemplateData(r)
	if userId != "" && user.Id != userId && !user.HasPermission(model.PermissionManageSpaces) {
		showPageForbidden(w, r)
		return
	}

	// User doesn't have permission to manage or use spaces so send them to the clients page
	if !config.LeafNode && !user.HasPermission(model.PermissionManageSpaces) && !user.HasPermission(model.PermissionUseSpaces) {
		http.Redirect(w, r, "/clients", http.StatusSeeOther)
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

	templateId := r.PathValue("template_id")
	if !validate.UUID(templateId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	tmpl, err := newTemplate("spaces-create-edit.tmpl")
	if err != nil {
		log.Fatal().Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	user, data := getCommonTemplateData(r)
	data["preferredShell"] = user.PreferredShell
	data["isEdit"] = false

	userId := r.PathValue("user_id")
	if userId != "" && !validate.UUID(userId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

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

	// Get the groups and build a map
	groups, err := db.GetGroups()
	if err != nil {
		log.Error().Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	groupMap := make(map[string]*model.Group)
	for _, group := range groups {
		groupMap[group.Id] = group
	}

	for _, groupId := range forUser.Groups {
		group, ok := groupMap[groupId]
		if ok {
			maxSpaces += group.MaxSpaces
		}
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
	spaceId := r.PathValue("space_id")
	if !validate.UUID(spaceId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

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
