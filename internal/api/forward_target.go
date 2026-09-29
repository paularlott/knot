package api

import (
	"net/http"
	"strconv"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
)

// HandleForwardTargetCheck reports whether the requesting user may forward to
// a target reference and port right now. Agents call it before creating a
// port forward requested from inside a space (the agentlink path), where the
// agent cannot judge cross-user targets itself: bare names it can check
// against the owner's space list, but user--space, pools and UUIDs need the
// server's resolution. The proxy's per-connection check remains the
// authority regardless.
func HandleForwardTargetCheck(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	ref := r.URL.Query().Get("target")
	port := r.URL.Query().Get("port")

	portUInt, err := strconv.ParseUint(port, 10, 16)
	if err != nil || !validate.IsNumber(int(portUInt), 1, 65535) || !service.ValidForwardRef(ref) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "target must be a space or pool name, user--space or space ID, and port must be 1-65535"})
		return
	}

	if _, ferr := service.AuthorizeForwardTarget(user, ref, uint16(portUInt)); ferr != nil {
		rest.WriteResponse(ferr.Status, w, r, ErrorResponse{Error: ferr.Message})
		return
	}

	rest.WriteResponse(http.StatusOK, w, r, struct {
		Status bool `json:"status"`
	}{Status: true})
}
