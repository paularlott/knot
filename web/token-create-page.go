package web

import (
	"net/http"
	"net/url"

	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/paularlott/knot/internal/log"
)

func HandleTokenCreatePage(w http.ResponseWriter, r *http.Request) {
	tmpl, err := newTemplate("api-tokens_create_named.tmpl")
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	user, data := getCommonTemplateData(r)

	name := r.PathValue("token_name")
	if !validate.TokenName(name) {
		showPageNotFound(w, r)
		return
	}

	name, err = url.PathUnescape(name)
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	token := model.NewToken(name, user.Id)
	db := database.GetInstance()
	err = db.SaveToken(token)
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	service.GetTransport().GossipToken(token)

	data["TokenName"] = token.Name
	data["TokenId"] = token.Id

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Error(err.Error())
	}
}
