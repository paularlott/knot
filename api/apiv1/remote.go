package apiv1

import (
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/util/rest"
)

func HandleRemoteGetTemplateVars(w http.ResponseWriter, r *http.Request) {
	templateVars, err := database.GetInstance().GetTemplateVars()
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
		return
	}

	// Build a json array of data to return to the client
	data := make([]apiclient.TemplateVarValues, len(templateVars))

	for i, variable := range templateVars {
		data[i].Name = variable.Name
		data[i].Value = variable.Value
	}

	rest.SendJSON(http.StatusOK, w, data)
}
