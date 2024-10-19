package web

import (
	"net/http"
	"net/url"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/middleware"

	"github.com/rs/zerolog/log"
)

func HandleLoginPage(w http.ResponseWriter, r *http.Request) {

	if !middleware.HasUsers {
		http.Redirect(w, r, "/initial-system-setup", http.StatusSeeOther)
	} else {
		session := middleware.GetSessionFromCookie(r)

		// If session present then redirect to dashboard
		if session != nil {
			http.Redirect(w, r, "/spaces", http.StatusSeeOther)
			return
		}

		tmpl, err := newTemplate("login.tmpl")
		if err != nil {
			log.Fatal().Msg(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Parse the URL to redirect to to get just the path
		var redirect string
		u, _ := url.Parse(r.URL.Query().Get("redirect"))
		if u.Path == "" {
			redirect = "/spaces"
		} else if u.Path[0:1] != "/" {
			redirect = "/" + u.Path
		} else {
			redirect = u.Path
		}

		data := map[string]interface{}{
			"redirect": redirect,
			"version":  build.Version,
		}

		err = tmpl.Execute(w, data)
		if err != nil {
			log.Fatal().Msg(err.Error())
		}
	}
}

func HandleLogoutPage(w http.ResponseWriter, r *http.Request) {
	session := r.Context().Value("session").(*model.Session)
	if session != nil {

		// Logout of the core server if this is a remote
		if session.RemoteSessionId != "" {
			client := apiclient.NewRemoteSession(session.RemoteSessionId)
			err := client.Logout()
			if err != nil {
				log.Error().Msgf("failed to logout: %s", err.Error())
			}
		}

		middleware.DeleteSessionCookie(w)
		database.GetCacheInstance().DeleteSession(session)
	}

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
