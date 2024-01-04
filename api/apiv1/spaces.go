package apiv1

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/nomad"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"

	"github.com/go-chi/chi/v5"
)

type CreateSpaceRequest struct {
  Name string `json:"name"`
  TemplateId string `json:"template_id"`
  AgentURL string `json:"agent_url"`
  Shell string `json:"shell"`
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
    HasTerminal bool `json:"has_terminal"`
    IsDeployed bool `json:"is_deployed"`
  }, len(spaces))

  for i, space := range spaces {
    var templateName string

    if space.TemplateId != model.MANUAL_TEMPLATE_ID {
      // Lookup the template
      template, err := database.GetInstance().GetTemplate(space.TemplateId)
      if err != nil {
        templateName = "Unknown"
      } else {
        templateName = template.Name
      }
    } else {
      templateName = "None (" + space.AgentURL + ")"
    }

    spaceData[i].Id = space.Id
    spaceData[i].Name = space.Name
    spaceData[i].TemplateName = templateName
    spaceData[i].IsDeployed = space.IsDeployed

    // Get the state of the agent
    agentState, ok := database.AgentStateGet(space.Id)
    if ok {
      spaceData[i].HasCodeServer = agentState.HasCodeServer
      spaceData[i].HasSSH = agentState.SSHPort > 0
      spaceData[i].HasTerminal = agentState.HasTerminal
    } else {
      spaceData[i].HasCodeServer = false
      spaceData[i].HasSSH = false
      spaceData[i].HasTerminal = false
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

  // If the space is running then fail
  if space.IsDeployed {
    rest.SendJSON(http.StatusLocked, w, ErrorResponse{Error: "space is running"})
    return
  }

   // Get the nomad client
  nomadClient := nomad.NewClient()

  // Delete volumes
  err = nomadClient.DeleteSpaceVolumes(space)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

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
  } else {
    request.TemplateId = model.MANUAL_TEMPLATE_ID
  }

  if(!validate.Name(request.Name) || (request.TemplateId == model.MANUAL_TEMPLATE_ID && !validate.Uri(request.AgentURL))) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid name, template, or address given for new space"})
    return
  }
  if(!validate.OneOf(request.Shell, []string{"bash", "zsh", "fish", "sh"})) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid shell given for space"})
    return
  }

  if(request.TemplateId != model.MANUAL_TEMPLATE_ID) {
    // Lookup the template
    _, err := database.GetInstance().GetTemplate(request.TemplateId)
    if err != nil {
      rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Unknown template"})
      return
    }
  }

  user := r.Context().Value("user").(*model.User)

  // Create the agent
  space := model.NewSpace(request.Name, user.Id, request.AgentURL, request.TemplateId, request.Shell)
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
  IsDeployed bool `json:"is_deployed"`
}

func HandleGetSpaceServiceState(w http.ResponseWriter, r *http.Request) {
  space, err := database.GetInstance().GetSpace(chi.URLParam(r, "space_id"))
  if err != nil {
    if err.Error() == "agent not found" {
      rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
    } else {
      rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    }
    return
  }

  response := SpaceServiceResponse{}
  state, ok := database.AgentStateGet(space.Id)
  if !ok {
    response.HasCodeServer = false
    response.HasSSH = false
  } else {
    response.HasCodeServer = state.HasCodeServer
    response.HasSSH = state.SSHPort > 0
  }

  response.IsDeployed = space.IsDeployed

  rest.SendJSON(http.StatusOK, w, response)
}

func HandleSpaceStart(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()

  space, err := db.GetSpace(chi.URLParam(r, "space_id"))
  if err != nil {
    rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
    return
  }

  // If the space is already running then fail
  if space.IsDeployed {
    rest.SendJSON(http.StatusLocked, w, ErrorResponse{Error: "space is running"})
    return
  }

  // Get the template
  template, err := db.GetTemplate(space.TemplateId)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Get the nomad client
  nomadClient := nomad.NewClient()

  // Create volumes
  err = nomadClient.CreateSpaceVolumes(template, space)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // TODO deploy the space job


  w.WriteHeader(http.StatusOK)
}

func HandleSpaceStop(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()

  space, err := db.GetSpace(chi.URLParam(r, "space_id"))
  if err != nil {
    rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
    return
  }

  // If the space is not running then fail
  if !space.IsDeployed {
    rest.SendJSON(http.StatusLocked, w, ErrorResponse{Error: "space not running"})
    return
  }

  // TODO Implement stop logic but leave volumes behind
  space.IsDeployed = false
  db.SaveSpace(space)

  w.WriteHeader(http.StatusOK)
}
