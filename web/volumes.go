package web

import (
	"net/http"

	"github.com/paularlott/knot/util/validate"
	"github.com/rs/zerolog/log"
)

func HandleVolumeCreate(w http.ResponseWriter, r *http.Request) {
	_, data := getCommonTemplateData(r)

	tmpl, err := newTemplate("volumes-create-edit.tmpl")
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

func HandleVolumeEdit(w http.ResponseWriter, r *http.Request) {
	_, data := getCommonTemplateData(r)

	tmpl, err := newTemplate("volumes-create-edit.tmpl")
	if err != nil {
		log.Fatal().Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	volumeId := r.PathValue("volume_id")
	if !validate.UUID(volumeId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	data["isEdit"] = true
	data["volumeId"] = volumeId

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Fatal().Msg(err.Error())
	}
}
