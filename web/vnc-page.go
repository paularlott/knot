package web

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/validate"
)

// HandleVNCPage serves the browser page hosting noVNC for a KVM space's
// QEMU display; the websocket it connects to is proxied by
// proxy.HandleSpacesVNCProxy.
func HandleVNCPage(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	spaceId := r.PathValue("space_id")
	if !validate.UUID(spaceId) {
		showPageNotFound(w, r)
		return
	}

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

	tmpl, err := newTemplate("vnc.tmpl")
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Interactive access to a running space is worth recording when the
	// space may hold production data copies — same policy as the terminal.
	if cfg := config.GetServerConfig(); cfg != nil && cfg.Audit.SpaceSessions {
		audit.LogWithRequest(r,
			user.Username,
			model.AuditActorTypeUser,
			model.AuditEventSpaceSessionOpen,
			fmt.Sprintf("Opened vnc session for space %s", space.Name),
			&map[string]interface{}{
				"space_id":   space.Id,
				"space_name": space.Name,
				"method":     "vnc",
			},
		)
	}

	data := map[string]interface{}{
		"spaceId":      spaceId,
		"spaceName":    space.Name,
		"version":      build.Version,
		"assetVersion": assetVersionKey(),
	}

	if err = tmpl.Execute(w, data); err != nil {
		log.Error(err.Error())
	}
}
