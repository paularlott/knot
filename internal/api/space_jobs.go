package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/spacejobs"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
)

// JobsErrorResponse is the 400 body from the jobs update endpoint: JobErrors
// maps each invalid job name to its validation error.
type JobsErrorResponse struct {
	Error     string            `json:"error"`
	JobErrors map[string]string `json:"job_errors,omitempty"`
}

// HandleGetSpaceJobs returns the space's persisted job definitions and runner
// state. Works for stopped spaces too — the space record is the source of
// truth, only live run status needs the agent.
func HandleGetSpaceJobs(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	spaceId := r.PathValue("space_id")

	// Support lookup by both ID and name
	db := database.GetInstance()
	var space *model.Space
	var err error
	if validate.UUID(spaceId) {
		space, err = db.GetSpace(spaceId)
	} else {
		space, err = db.GetSpaceByName(user.Id, spaceId)
	}
	if err != nil || space == nil || space.IsDeleted || (space.UserId != user.Id && !space.IsSharedWith(user.Id) && !user.HasPermission(model.PermissionManageSpaces)) {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "space not found"})
		return
	}

	response := &apiclient.SpaceJobsResponse{
		Jobs:    space.Jobs,
		Enabled: space.JobsEnabled,
	}
	if response.Jobs == nil {
		response.Jobs = []model.SpaceJob{}
	}

	rest.WriteResponse(http.StatusOK, w, r, response)
}

// HandleUpdateSpaceJobs replaces the space's job definitions and runner
// state, then pushes the change to the agent when the space is running.
func HandleUpdateSpaceJobs(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	spaceId := r.PathValue("space_id")

	// Support lookup by both ID and name. Writing requires ownership (the
	// service checks permissions again for the resolved ID).
	db := database.GetInstance()
	var space *model.Space
	var err error
	if validate.UUID(spaceId) {
		space, err = db.GetSpace(spaceId)
	} else {
		space, err = db.GetSpaceByName(user.Id, spaceId)
	}
	if err != nil || space == nil || space.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "space not found"})
		return
	}
	spaceId = space.Id

	request := apiclient.SpaceJobsRequest{}
	if err := rest.DecodeRequestBody(w, r, &request); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	if request.Jobs == nil {
		request.Jobs = []model.SpaceJob{}
	}

	if errs := spacejobs.ValidateJobs(request.Jobs); len(errs) > 0 {
		rest.WriteResponse(http.StatusBadRequest, w, r, JobsErrorResponse{
			Error:     "invalid job definitions: " + summarizeJobErrors(errs),
			JobErrors: errs,
		})
		return
	}

	spaceService := service.GetSpaceService()
	if err := spaceService.SetSpaceJobs(spaceId, request.Jobs, request.Enabled, user); err != nil {
		log.WithError(err).Error("HandleUpdateSpaceJobs:")
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	space.Jobs = request.Jobs
	space.JobsEnabled = request.Enabled

	// Push the new definitions to a connected agent; a stopped or unreachable
	// agent picks them up in its next registration response.
	if session := agent_server.GetSession(spaceId); session != nil {
		if err := session.SendUpdateJobs(space.Jobs, space.JobsEnabled); err != nil {
			log.WithError(err).Error("HandleUpdateSpaceJobs: failed to push jobs to agent")
		}
	}

	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventSpaceUpdate,
		fmt.Sprintf("Updated jobs on space %s", space.Name),
		&map[string]interface{}{
			"space_id":   space.Id,
			"space_name": space.Name,
			"jobs":       len(space.Jobs),
			"enabled":    space.JobsEnabled,
		},
	)

	rest.WriteResponse(http.StatusOK, w, r, &apiclient.SpaceJobsResponse{
		Jobs:    space.Jobs,
		Enabled: space.JobsEnabled,
	})
}

// summarizeJobErrors renders per-job validation errors as one sorted string.
func summarizeJobErrors(errs map[string]string) string {
	names := make([]string, 0, len(errs))
	for name := range errs {
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		if name == "" {
			parts = append(parts, errs[name])
		} else {
			parts = append(parts, fmt.Sprintf("%s: %s", name, errs[name]))
		}
	}
	return strings.Join(parts, "; ")
}
