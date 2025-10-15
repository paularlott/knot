package web

import (
	"encoding/json"
	"net/http"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/paularlott/knot/internal/log"
)

func HandleListSpaces(w http.ResponseWriter, r *http.Request) {
	db := database.GetInstance()

	tmpl, err := newTemplate("spaces.tmpl")
	if err != nil {
		log.Error(err.Error())
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
	cfg := config.GetServerConfig()
	if !cfg.LeafNode && !user.HasPermission(model.PermissionManageSpaces) && !user.HasPermission(model.PermissionUseSpaces) {
		http.Redirect(w, r, "/clients", http.StatusSeeOther)
		return
	}

	if userId != "" {
		forUser, err := db.GetUser(userId)
		if err != nil {
			log.Error(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		data["forUserId"] = userId
		data["forUserUsername"] = forUser.Username
	} else {
		data["forUserId"] = user.Id
		data["forUserUsername"] = ""
	}

	data["wildcard_domain"] = cfg.WildcardDomain

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Error(err.Error())
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
		log.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	iconListJSON, err := json.Marshal(service.GetIconService().GetIcons())
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	user, data := getCommonTemplateData(r)
	data["preferredShell"] = user.PreferredShell
	data["isEdit"] = false
	data["iconList"] = string(iconListJSON)

	userId := r.PathValue("user_id")
	if userId != "" && !validate.UUID(userId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if userId != "" && user.Id != userId && !user.HasPermission(model.PermissionManageSpaces) {
		showPageForbidden(w, r)
		return
	}

	template, err := db.GetTemplate(templateId)
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var forUser *model.User

	if userId != "" {
		forUser, err = db.GetUser(userId)
		if err != nil {
			log.Error(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		data["forUserUsername"] = forUser.Username
	} else {
		data["forUserUsername"] = ""
		forUser = user
	}
	data["forUserId"] = userId
	data["templateId"] = templateId

	spaceService := service.GetSpaceService()
	err = spaceService.CheckUserQuotas(forUser.Id, template)
	if err != nil {
		http.Redirect(w, r, "/space-quota-reached", http.StatusSeeOther)
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Error(err.Error())
	}
}

func HandleSpacesEdit(w http.ResponseWriter, r *http.Request) {
	db := database.GetInstance()
	user, data := getCommonTemplateData(r)

	tmpl, err := newTemplate("spaces-create-edit.tmpl")
	if err != nil {
		log.Error(err.Error())
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
		log.Error(err.Error())
		showPageNotFound(w, r)
		return
	}

	if space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		showPageForbidden(w, r)
		return
	}

	iconListJSON, err := json.Marshal(service.GetIconService().GetIcons())
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	data["iconList"] = string(iconListJSON)

	data["isEdit"] = true
	data["preferredShell"] = ""
	data["templateList"], err = getTemplateOptionList(user)
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	data["spaceId"] = spaceId

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Error(err.Error())
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
