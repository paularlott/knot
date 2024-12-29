package web

import (
	"net/http"

	"github.com/paularlott/knot/database/model"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

func HandleRoleCreate(w http.ResponseWriter, r *http.Request) {
	user, data := getCommonTemplateData(r)
	if !user.HasPermission(model.PermissionManageRoles) {
		showPageForbidden(w, r)
		return
	}

	tmpl, err := newTemplate("role-create-edit.tmpl")
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

func HandleRoleEdit(w http.ResponseWriter, r *http.Request) {
	user, data := getCommonTemplateData(r)
	if !user.HasPermission(model.PermissionManageRoles) {
		showPageForbidden(w, r)
		return
	}

	tmpl, err := newTemplate("role-create-edit.tmpl")
	if err != nil {
		log.Fatal().Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	data["isEdit"] = true
	data["roleId"] = chi.URLParam(r, "role_id")

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Fatal().Msg(err.Error())
	}
}
