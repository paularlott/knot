package web

import (
	"net/http"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/validate"
	"github.com/spf13/viper"

	"github.com/rs/zerolog/log"
)

func HandleTerminalPage(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	spaceId := r.PathValue("space_id")
	if !validate.UUID(spaceId) {
		showPageNotFound(w, r)
		return
	}

	// Load the space
	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		showPageNotFound(w, r)
		return
	}

	// Check if the user has access to the space
	if space.UserId != user.Id && space.SharedWithUserId != user.Id {
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
	if r.PathValue("vsc") == "vscode-tunnel" {
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
