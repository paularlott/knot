package web

import (
	"net/http"

	"github.com/paularlott/knot/internal/util/validate"

	"github.com/rs/zerolog/log"
)

func HandleUserCreate(w http.ResponseWriter, r *http.Request) {
	tmpl, err := newTemplate("users-create-edit.tmpl")
	if err != nil {
		log.Error().Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	_, data := getCommonTemplateData(r)
	data["isEdit"] = false

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Error().Msg(err.Error())
	}
}

func HandleUserEdit(w http.ResponseWriter, r *http.Request) {
	_, data := getCommonTemplateData(r)

	userId := r.PathValue("user_id")
	if !validate.UUID(userId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	tmpl, err := newTemplate("users-create-edit.tmpl")
	if err != nil {
		log.Error().Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	data["isEdit"] = true
	data["isProfile"] = false
	data["user"] = map[string]interface{}{
		"id": userId,
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Error().Msg(err.Error())
	}
}

func HandleUserProfilePage(w http.ResponseWriter, r *http.Request) {
	user, data := getCommonTemplateData(r)

	tmpl, err := newTemplate("users-create-edit.tmpl")
	if err != nil {
		log.Error().Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	data["isEdit"] = true
	data["isProfile"] = true
	data["user"] = map[string]interface{}{
		"id": user.Id,
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Error().Msg(err.Error())
	}
}
