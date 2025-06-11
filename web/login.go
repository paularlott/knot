package web

import (
	"net/http"
	"net/url"
	"time"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/middleware"
	"github.com/paularlott/knot/internal/service"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func HandleLoginPage(w http.ResponseWriter, r *http.Request) {

	if !middleware.HasUsers && viper.GetString("server.origin.server") == "" && viper.GetString("server.origin.token") == "" {
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
		if u.Path == "" || u.Path == "/logout" {
			redirect = "/spaces"
		} else if u.Path[0:1] != "/" {
			redirect = "/" + u.Path
		} else {
			redirect = u.Path
		}

		data := map[string]interface{}{
			"redirect":    redirect,
			"version":     build.Version,
			"totpEnabled": viper.GetBool("server.totp.enabled"),
			"logoURL":     viper.GetString("server.ui.logo_url"),
			"logoInvert":  viper.GetBool("server.ui.logo_invert"),
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
		session.IsDeleted = true
		session.ExpiresAfter = time.Now().Add(model.SessionExpiryDuration).UTC()
		session.UpdatedAt = time.Now().UTC()
		database.GetSessionStorage().SaveSession(session)
		service.GetTransport().GossipSession(session)
	}

	middleware.DeleteSessionCookie(w)

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
