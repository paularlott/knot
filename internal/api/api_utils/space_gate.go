package api_utils

import (
	"net/http"

	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/rest"
)

// RequestSpaceTemplate returns the template of the space a request comes
// from, or nil when the request isn't from a space (only the agent's token
// carries one) or the template can't be found. It is read on every call, so
// changing a template's settings applies to its spaces straight away.
func RequestSpaceTemplate(r *http.Request) *model.Template {
	spaceId, _ := r.Context().Value("space_id").(string)
	if spaceId == "" {
		return nil
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil || space == nil || space.TemplateId == "" {
		return nil
	}
	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		return nil
	}
	return template
}

// FileStorageAllowed refuses file storage requests from spaces whose
// template switches file storage off.
func FileStorageAllowed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if template := RequestSpaceTemplate(r); template != nil && template.DisableFileStorage {
			rest.WriteResponse(http.StatusForbidden, w, r, struct {
				Error string `json:"error" msgpack:"error"`
			}{Error: "file storage is switched off for spaces made from this template"})
			return
		}
		next(w, r)
	}
}
