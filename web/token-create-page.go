package web

import (
	"net/http"
	"net/url"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

func HandleTokenCreatePage(w http.ResponseWriter, r *http.Request) {
	tmpl, err := newTemplate("api-tokens_create_named.tmpl")
	if err != nil {
		log.Fatal().Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	user, data := getCommonTemplateData(r)

	name := chi.URLParam(r, "token_name")
	name, err = url.PathUnescape(name)
	if err != nil {
		log.Error().Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	token := model.NewToken(name, user.Id)
	db := database.GetInstance()
	err = db.SaveToken(token)
	if err != nil {
		log.Error().Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	data["TokenName"] = token.Name
	data["TokenId"] = token.Id

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Fatal().Msg(err.Error())
	}
}
