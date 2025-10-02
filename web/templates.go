package web

import (
	"encoding/json"
	"net/http"

	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/rs/zerolog/log"
)

func HandleTemplateCreate(w http.ResponseWriter, r *http.Request) {
	_, data := getCommonTemplateData(r)

	tmpl, err := newTemplate("templates-create-edit.tmpl")
	if err != nil {
		log.Error().Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	iconListJSON, err := json.Marshal(service.GetIconService().GetIcons())
	if err != nil {
		log.Error().Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	data["isEdit"] = false
	data["iconList"] = string(iconListJSON)

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Error().Msg(err.Error())
	}
}

func HandleTemplateEdit(w http.ResponseWriter, r *http.Request) {
	_, data := getCommonTemplateData(r)

	tmpl, err := newTemplate("templates-create-edit.tmpl")
	if err != nil {
		log.Error().Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	templateId := r.PathValue("template_id")
	if !validate.UUID(templateId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	iconListJSON, err := json.Marshal(service.GetIconService().GetIcons())
	if err != nil {
		log.Error().Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	data["isEdit"] = true
	data["templateId"] = templateId
	data["iconList"] = string(iconListJSON)

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Error().Msg(err.Error())
	}
}
