package web

import (
	"net/http"

	"github.com/paularlott/knot/internal/util/validate"

	"github.com/paularlott/knot/internal/log"
)

func HandleTemplateVarCreate(w http.ResponseWriter, r *http.Request) {
	_, data := getCommonTemplateData(r)

	tmpl, err := newTemplate("variable-create-edit.tmpl")
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	data["isEdit"] = false

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Error(err.Error())
	}
}

func HandleTemplateVarEdit(w http.ResponseWriter, r *http.Request) {
	_, data := getCommonTemplateData(r)

	tmpl, err := newTemplate("variable-create-edit.tmpl")
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	templateVarId := r.PathValue("templatevar_id")
	if !validate.UUID(templateVarId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	data["isEdit"] = true
	data["templateVarId"] = templateVarId

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Error(err.Error())
	}
}
