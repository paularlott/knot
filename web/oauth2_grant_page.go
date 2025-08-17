package web

import (
	"net/http"
	"net/url"

	"github.com/rs/zerolog/log"
)

func HandleOAuth2GrantPage(w http.ResponseWriter, r *http.Request) {
	tmpl, err := newTemplate("oauth2_grant.tmpl")
	if err != nil {
		log.Fatal().Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	_, data := getCommonTemplateData(r)

	// Get OAuth2 parameters from query
	data["clientId"] = r.URL.Query().Get("client_id")
	data["redirectURI"] = r.URL.Query().Get("redirect_uri")
	data["scope"] = r.URL.Query().Get("scope")
	data["state"] = r.URL.Query().Get("state")

	if u, err := url.Parse(r.URL.Query().Get("redirect_uri")); err == nil {
		data["redirectDomain"] = u.Hostname()
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Fatal().Msg(err.Error())
	}
}
