package web

import (
	"net/http"

	"github.com/paularlott/knot/internal/log"
)

func HandleUserProfilePage(w http.ResponseWriter, r *http.Request) {
	user, data := getCommonTemplateData(r)

	tmpl, err := newTemplate("page-profile.tmpl")
	if err != nil {
		log.Error(err.Error())
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
		log.Error(err.Error())
	}
}
