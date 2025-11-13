package web

import (
	"net/http"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
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
	http.Redirect(w, r, "/spaces", http.StatusSeeOther)
}

func HandleSpacesEdit(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/spaces", http.StatusSeeOther)
}
