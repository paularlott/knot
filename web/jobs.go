package web

import (
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
)

// HandleJobsList returns the space's jobs live status (running state, next
// run, last run) from the agent. Job definitions live on the space record
// (/api/spaces/{id}/jobs) and are available for stopped spaces too; this
// endpoint needs a running agent.
func HandleJobsList(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	spaceId := r.PathValue("space_id")
	if !validate.UUID(spaceId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil || space == nil || (space.UserId != user.Id && !space.IsSharedWith(user.Id)) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	agentSession := agent_server.GetSession(spaceId)
	if agentSession == nil {
		writeJSONError(w, r, http.StatusConflict, "Space is not running")
		return
	}

	response, err := agentSession.SendJobsList()
	if err != nil {
		writeJSONError(w, r, http.StatusInternalServerError, "Failed to send command to agent")
		return
	}

	rest.WriteResponse(http.StatusOK, w, r, response)
}

// HandleJobsRun triggers a job immediately by name. Manual triggering works
// for disabled and manual-only jobs.
func HandleJobsRun(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	spaceId := r.PathValue("space_id")
	if !validate.UUID(spaceId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil || space == nil || (space.UserId != user.Id && !space.IsSharedWith(user.Id)) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	var request apiclient.JobRunRequest
	if err := rest.DecodeRequestBody(w, r, &request); err != nil {
		return
	}
	if request.Name == "" {
		writeJSONError(w, r, http.StatusBadRequest, "Job name is required")
		return
	}

	agentSession := agent_server.GetSession(spaceId)
	if agentSession == nil {
		writeJSONError(w, r, http.StatusConflict, "Space is not running")
		return
	}

	response, err := agentSession.SendJobsRun(request.Name)
	if err != nil {
		writeJSONError(w, r, http.StatusInternalServerError, "Failed to send command to agent")
		return
	}

	if response.Success {
		rest.WriteResponse(http.StatusOK, w, r, response)
	} else {
		writeJSONError(w, r, http.StatusInternalServerError, response.Error)
	}
}
