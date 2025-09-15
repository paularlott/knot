package web

import (
	"net/http"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/validate"

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
	cfg := config.GetServerConfig()
	if cfg.TerminalWebGL {
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
		"version":  build.Version,
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Fatal().Msg(err.Error())
	}
}
