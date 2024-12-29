package web

import (
	"net/http"

	"github.com/paularlott/knot/database/model"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

func HandleGroupCreate(w http.ResponseWriter, r *http.Request) {
	user, data := getCommonTemplateData(r)
	if !user.HasPermission(model.PermissionManageGroups) {
		showPageForbidden(w, r)
		return
	}

	tmpl, err := newTemplate("group-create-edit.tmpl")
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

func HandleGroupEdit(w http.ResponseWriter, r *http.Request) {
	user, data := getCommonTemplateData(r)
	if !user.HasPermission(model.PermissionManageGroups) {
		showPageForbidden(w, r)
		return
	}

	tmpl, err := newTemplate("group-create-edit.tmpl")
	if err != nil {
		log.Fatal().Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	data["isEdit"] = true
	data["groupId"] = chi.URLParam(r, "group_id")

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Fatal().Msg(err.Error())
	}
}
