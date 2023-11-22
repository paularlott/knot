package apiv1

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"
)

type CreateSpaceRequest struct {
  Name string `json:"name"`
  TemplateId string `json:"template_id"`
  AgentURL string `json:"agent_url"`
}

func HandleGetSpaces(w http.ResponseWriter, r *http.Request) {
  user := r.Context().Value("user").(*model.User)

  spaces, err := database.GetInstance().GetSpacesForUser(user.Id)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Build a json array of token data to return to the client
  spaceData := make([]struct {
    Id string `json:"space_id"`
    Name string `json:"name"`
    TemplateName string `json:"template_name"`
    HasCodeServer bool `json:"has_code_server"`
    HasSSH bool `json:"has_ssh"`
  }, len(spaces))

  for i, space := range spaces {

    // TODO Lookup the template name
    var templateName string

    if space.TemplateId != "" {
      templateName = "TODO Lookup Template Name"
    } else {
      templateName = "None (" + space.AgentURL + ")"
    }

    spaceData[i].Id = space.Id
    spaceData[i].Name = space.Name
    spaceData[i].TemplateName = templateName

    // Get the state of the agent
    agentState, ok := database.AgentStateGet(space.Id)
    if ok {
      spaceData[i].HasCodeServer = agentState.HasCodeServer
      spaceData[i].HasSSH = agentState.HasSSH
    } else {
      spaceData[i].HasCodeServer = false
      spaceData[i].HasSSH = false
    }
  }

  rest.SendJSON(http.StatusOK, w, spaceData)
}

func HandleDeleteSpace(w http.ResponseWriter, r *http.Request) {
  user := r.Context().Value("user").(*model.User)

  // Load the space if not found or doesn't belong to the user then treat both as not found
  space, err := database.GetInstance().GetSpace(chi.URLParam(r, "space_id"))
  if err != nil || space.UserId != user.Id {
    rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: fmt.Sprintf("space %s not found", chi.URLParam(r, "space_id"))})
    return
  }

  // TODO If the space has a template and it's running then stop the nomad job

  // Delete the agent state
  database.AgentStateRemove(space.Id)

  // Delete the agent
  err = database.GetInstance().DeleteSpace(space)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  w.WriteHeader(http.StatusOK)
}

func HandleCreateSpace(w http.ResponseWriter, r *http.Request) {
  request := CreateSpaceRequest{}

  err := rest.BindJSON(w, r, &request)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  // If template given then ensure the address is removed
  if request.TemplateId != "" {
    request.AgentURL = ""
  }

  if(!validate.Name(request.Name) || (request.TemplateId != "" && !validate.Uri(request.AgentURL))) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid name, template, or address given for new space"})
    return
  }

  // TODO validate the template if one given
  // TODO if template given deploy the nomad job
  // TODO if template given then auto generate the address and port = 0

  user := r.Context().Value("user").(*model.User)

  // Create the agent
  space := model.NewSpace(request.Name, user.Id, request.AgentURL, request.TemplateId)
  err = database.GetInstance().SaveSpace(space)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Return the Token ID
  rest.SendJSON(http.StatusCreated, w, struct {
    Status bool `json:"status"`
    SpaceID string `json:"space_id"`
  }{
    Status: true,
    SpaceID: space.Id,
  })
}

type SpaceServiceResponse struct {
  HasCodeServer bool `json:"has_code_server"`
  HasSSH bool `json:"has_ssh"`
}

func HandleGetSpaceServiceState(w http.ResponseWriter, r *http.Request) {
  response := SpaceServiceResponse{}
  space, ok := database.AgentStateGet(chi.URLParam(r, "space_id"))
  if !ok {
    response.HasCodeServer = false
    response.HasSSH = false
  } else {
    response.HasCodeServer = space.HasCodeServer
    response.HasSSH = space.HasSSH
  }

  rest.SendJSON(http.StatusOK, w, response)
}
