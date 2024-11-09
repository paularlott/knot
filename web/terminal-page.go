package web

import (
	"net/http"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/spf13/viper"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

func HandleTerminalPage(w http.ResponseWriter, r *http.Request) {
	spaceId := chi.URLParam(r, "space_id")
	user := r.Context().Value("user").(*model.User)

	// Load the space
	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		showPageNotFound(w, r)
		return
	}

	// Check if the user has access to the space
	if space.UserId != user.Id {
		showPageNotFound(w, r)
		return
	}

	tmpl, err := newTemplate("terminal.tmpl")
	if err != nil {
		log.Fatal().Msg(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var renderer string
	if viper.GetBool("server.terminal.webgl") {
		renderer = "webgl"
	} else {
		renderer = "canvas"
	}

	// If the last segment of the url is vscode-tunnel log it
	shell := space.Shell
	if chi.URLParam(r, "vsc") == "vscode-tunnel" {
		shell = "vscode-tunnel"
	}

	data := map[string]interface{}{
		"shell":    shell,
		"renderer": renderer,
		"spaceId":  spaceId,
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Fatal().Msg(err.Error())
	}
}
