package web

import (
	"net/http"

	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/origin_leaf/server_info"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

func HandleTemplateVarCreate(w http.ResponseWriter, r *http.Request) {
	user, data := getCommonTemplateData(r)
	if !user.HasPermission(model.PermissionManageVariables) && !server_info.RestrictedLeaf {
		showPageForbidden(w, r)
		return
	}

	tmpl, err := newTemplate("variable-create-edit.tmpl")
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

func HandleTemplateVarEdit(w http.ResponseWriter, r *http.Request) {
	user, data := getCommonTemplateData(r)
	if !user.HasPermission(model.PermissionManageVariables) {
		showPageForbidden(w, r)
		return
	}

	tmpl, err := newTemplate("variable-create-edit.tmpl")
	if err != nil {
		log.Fatal().Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	data["isEdit"] = true
	data["templateVarId"] = chi.URLParam(r, "templatevar_id")

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Fatal().Msg(err.Error())
	}
}
