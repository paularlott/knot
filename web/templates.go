package web

import (
	"net/http"

	"github.com/paularlott/knot/util/validate"
	"github.com/rs/zerolog/log"
)

func HandleTemplateCreate(w http.ResponseWriter, r *http.Request) {
	_, data := getCommonTemplateData(r)

	tmpl, err := newTemplate("templates-create-edit.tmpl")
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

func HandleTemplateEdit(w http.ResponseWriter, r *http.Request) {
	_, data := getCommonTemplateData(r)

	tmpl, err := newTemplate("templates-create-edit.tmpl")
	if err != nil {
		log.Fatal().Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	templateId := r.PathValue("template_id")
	if !validate.UUID(templateId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	data["isEdit"] = true
	data["templateId"] = templateId

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Fatal().Msg(err.Error())
	}
}
