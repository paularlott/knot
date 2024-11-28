package apiv1

import (
	"net/http"

	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/rest"
)

func HandleGetRoles(w http.ResponseWriter, r *http.Request) {
	rest.SendJSON(http.StatusOK, w, r, model.RoleNames)
}
