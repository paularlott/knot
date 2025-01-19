package web

import (
	"net/http"

	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/validate"

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

	groupId := r.PathValue("group_id")
	if !validate.UUID(groupId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	data["isEdit"] = true
	data["groupId"] = groupId

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Fatal().Msg(err.Error())
	}
}
