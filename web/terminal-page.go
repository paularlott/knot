package web

import (
	"fmt"
	"github.com/paularlott/knot/internal/util/audit"
	"net/http"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/paularlott/knot/internal/log"
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
	if space.UserId != user.Id && !space.IsSharedWith(user.Id) {
		showPageNotFound(w, r)
		return
	}

	tmpl, err := newTemplate("terminal.tmpl")
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Interactive access to a running space is worth recording when the
	// space may hold production data copies — gated because terminal opens
	// are routine on a local dev machine.
	if cfg := config.GetServerConfig(); cfg != nil && cfg.Audit.SpaceSessions {
		method := "terminal"
		switch r.PathValue("vsc") {
		case "vscode-tunnel":
			method = "vscode-tunnel"
		case "console":
			method = "console"
		}
		audit.LogWithRequest(r,
			user.Username,
			model.AuditActorTypeUser,
			model.AuditEventSpaceSessionOpen,
			fmt.Sprintf("Opened %s session for space %s", method, space.Name),
			&map[string]interface{}{
				"space_id":   space.Id,
				"space_name": space.Name,
				"method":     method,
			},
		)
	}

	var renderer string
	cfg := config.GetServerConfig()
	if cfg.TerminalWebGL {
		renderer = "webgl"
	} else {
		renderer = "canvas"
	}

	// If the last segment of the url is vscode-tunnel or console select it
	shell := space.Shell
	consoleView := false
	if r.PathValue("vsc") == "vscode-tunnel" {
		shell = "vscode-tunnel"
	} else if r.PathValue("vsc") == "console" {
		shell = "console"
		consoleView = true
	}

	data := map[string]interface{}{
		"shell":        shell,
		"renderer":     renderer,
		"spaceId":      spaceId,
		"logView":      false,
		"consoleView":  consoleView,
		"version":      build.Version,
		"assetVersion": assetVersionKey(),
	}

	err = tmpl.Execute(w, data)
	if err != nil {
		log.Error(err.Error())
	}
}
