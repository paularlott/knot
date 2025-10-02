package web

import (
	"net/http"

	"github.com/paularlott/knot/internal/middleware"

	"github.com/rs/zerolog/log"
)

func HandleInitialSystemSetupPage(w http.ResponseWriter, r *http.Request) {

	// If there's users then don't allow this to run, redirect to login
	if middleware.HasUsers {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	} else {
		tmpl, err := newTemplate("initial-system-setup.tmpl")
		if err != nil {
			log.Error().Msg(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		err = tmpl.Execute(w, nil)
		if err != nil {
			log.Error().Msg(err.Error())
		}
	}
}
