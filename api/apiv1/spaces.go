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
	"github.com/spf13/viper"
)

type SpaceRequest struct {
  Name string `json:"name"`
  TemplateId string `json:"template_id"`
  AgentURL string `json:"agent_url"`
  Shell string `json:"shell"`
  UserId string `json:"user_id"`
  VolumeSize map[string]int64 `json:"volume_size"`
}

func HandleGetSpaces(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()
  cache := database.GetCacheInstance()

  user := r.Context().Value("user").(*model.User)
  userId := r.URL.Query().Get("user_id")

  // If user doesn't have permission to manage spaces and filter user ID doesn't match the user return an empty list
  if !user.HasPermission(model.PermissionManageSpaces) && userId != user.Id {
    rest.SendJSON(http.StatusOK, w, []struct{}{})
    return
  }

  var spaces []*model.Space
  var err error

  if userId == "" {
    spaces, err = db.GetSpaces()
    if err != nil {
      rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
      return
    }
  } else {
    spaces, err = db.GetSpacesForUser(userId)
    if err != nil {
      rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
      return
    }
  }

  // Build a json array of token data to return to the client
  spaceData := make([]struct {
    Id string `json:"space_id"`
    Name string `json:"name"`
    TemplateName string `json:"template_name"`
    TemplateId string `json:"template_id"`
    HasCodeServer bool `json:"has_code_server"`
    HasSSH bool `json:"has_ssh"`
    HasHttpVNC bool `json:"has_http_vnc"`
    HasTerminal bool `json:"has_terminal"`
    IsDeployed bool `json:"is_deployed"`
    Username string `json:"username"`
    UserId string `json:"user_id"`
    TcpPorts []int `json:"tcp_ports"`
    HttpPorts []int `json:"http_ports"`
    VolumeSize int `json:"volume_size"`
  }, len(spaces))

  for i, space := range spaces {
    var templateName string

    if space.TemplateId != model.MANUAL_TEMPLATE_ID {
      // Lookup the template
      template, err := db.GetTemplate(space.TemplateId)
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
    spaceData[i].TemplateId = space.TemplateId
    spaceData[i].IsDeployed = space.IsDeployed

    // Get the user
    u, err := db.GetUser(space.UserId)
    if err != nil {
      rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
      return
    }
    spaceData[i].Username = u.Username
    spaceData[i].UserId = u.Id

    // Get the state of the agent
    agentState, _ := cache.GetAgentState(space.Id)
    if agentState != nil {
      spaceData[i].HasCodeServer = agentState.HasCodeServer
      spaceData[i].HasSSH = agentState.SSHPort > 0
      spaceData[i].HasTerminal = agentState.HasTerminal
      spaceData[i].HasHttpVNC = agentState.VNCHttpPort > 0
      spaceData[i].TcpPorts = agentState.TcpPorts

      // If wildcard domain is set then offer the http ports
      if viper.GetString("server.wildcard_domain") == "" {
        spaceData[i].HttpPorts = []int{}
      } else {
        spaceData[i].HttpPorts = agentState.HttpPorts
      }
    } else {
      spaceData[i].HasCodeServer = false
      spaceData[i].HasSSH = false
      spaceData[i].HasHttpVNC = false
      spaceData[i].HasTerminal = false
      spaceData[i].TcpPorts = []int{}
      spaceData[i].HttpPorts = []int{}
    }

    spaceData[i].VolumeSize, err = calcSpaceDiskUsage(space)
    if err != nil {
      rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
      return
    }
  }

  rest.SendJSON(http.StatusOK, w, spaceData)
}

func HandleDeleteSpace(w http.ResponseWriter, r *http.Request) {
  user := r.Context().Value("user").(*model.User)
  db := database.GetInstance()
  cache := database.GetCacheInstance()

  // Load the space if not found or doesn't belong to the user then treat both as not found
  space, err := db.GetSpace(chi.URLParam(r, "space_id"))
  if err != nil || (space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces)) {
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
  state, _ := cache.GetAgentState(space.Id)
  if state != nil {
    cache.DeleteAgentState(state)
  }

  // Delete the agent
  err = db.DeleteSpace(space)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  w.WriteHeader(http.StatusOK)
}

func HandleCreateSpace(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()
  request := SpaceRequest{}
  user := r.Context().Value("user").(*model.User)

  err := rest.BindJSON(w, r, &request)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  // If user give and not our ID and no permission to manage spaces then fail
  if request.UserId != "" && request.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
    rest.SendJSON(http.StatusForbidden, w, ErrorResponse{Error: "Cannot create space for another user"})
    return
  }

  // If template given then ensure the address is removed
  if request.TemplateId != model.MANUAL_TEMPLATE_ID {
    request.AgentURL = ""
  }

  if(!validate.Name(request.Name) || (request.TemplateId == model.MANUAL_TEMPLATE_ID && !validate.Uri(request.AgentURL))) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid name, template, or address given for new space"})
    return
  }
  if(!validate.OneOf(request.Shell, []string{"bash", "zsh", "fish", "sh"})) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid shell given for space"})
    return
  }

  // Lookup the template
  template, err := db.GetTemplate(request.TemplateId)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Unknown template"})
    return
  }

  // Check the user and template have overlapping groups
  if len(template.Groups) > 0 && !user.HasAnyGroup(&template.Groups) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Unknown template"})
    return
  }

  // Create the space
  forUserId := user.Id
  if request.UserId != "" {
    forUserId = request.UserId
  }
  space := model.NewSpace(request.Name, forUserId, request.AgentURL, request.TemplateId, request.Shell, &request.VolumeSize)

  // Test if over quota
  if user.MaxDiskSpace > 0 {

    // Get the size for this space
    size, err := space.GetStorageSize(template)
    if err != nil {
      rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
      return
    }

    // Get the size of storage for all the users spaces
    spaces, err := db.GetSpacesForUser(forUserId)
    if err != nil {
      rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
      return
    }

    for _, s := range spaces {
      sSize, err := calcSpaceDiskUsage(s)
      if err != nil {
        rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
        return
      }
      size += sSize
    }

    if size > user.MaxDiskSpace {
      rest.SendJSON(http.StatusInsufficientStorage, w, ErrorResponse{Error: "storage quota reached"})
      return
    }
  }

  // Save the space
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
  HasHttpVNC bool `json:"has_http_vnc"`
  HasTerminal bool `json:"has_terminal"`
  IsDeployed bool `json:"is_deployed"`
  TcpPorts []int `json:"tcp_ports"`
  HttpPorts []int `json:"http_ports"`
  UpdateAvailable bool `json:"update_available"`
}

func HandleGetSpaceServiceState(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()
  cache := database.GetCacheInstance()

  space, err := db.GetSpace(chi.URLParam(r, "space_id"))
  if err != nil || space == nil {
    if err.Error() == "space not found" {
      rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
    } else {
      rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    }
    return
  }

  response := SpaceServiceResponse{}
  state, _ := cache.GetAgentState(space.Id)
  if state == nil {
    response.HasCodeServer = false
    response.HasSSH = false
    response.HasTerminal = false
    response.HasHttpVNC = false
    response.TcpPorts = []int{}
    response.HttpPorts = []int{}
  } else {
    response.HasCodeServer = state.HasCodeServer
    response.HasSSH = state.SSHPort > 0
    response.HasTerminal = state.HasTerminal
    response.HasHttpVNC = state.VNCHttpPort > 0
    response.TcpPorts = state.TcpPorts

    // If wildcard domain is set then offer the http ports
    if viper.GetString("server.wildcard_domain") == "" {
      response.HttpPorts = []int{}
    } else {
      response.HttpPorts = state.HttpPorts
    }
  }

  response.IsDeployed = space.IsDeployed

  if space.TemplateId == model.MANUAL_TEMPLATE_ID {
    response.UpdateAvailable = false
  } else {
    template, err := db.GetTemplate(space.TemplateId)
    if err != nil {
      rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
      return
    }
    response.UpdateAvailable = space.IsDeployed && space.TemplateHash != template.Hash
  }

  rest.SendJSON(http.StatusOK, w, response)
}

func HandleSpaceStart(w http.ResponseWriter, r *http.Request) {
  user := r.Context().Value("user").(*model.User)
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

  // Add the variables
  variables, err := db.GetTemplateVars()
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  vars := make(map[string]interface{})
  for _, variable := range variables {
    vars[variable.Name] = variable.Value
  }

  // Get the nomad client
  nomadClient := nomad.NewClient()

  // Create volumes
  err = nomadClient.CreateSpaceVolumes(user, template, space, &vars)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Start the job
  err = nomadClient.CreateSpaceJob(user, template, space, &vars)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  w.WriteHeader(http.StatusOK)
}

func HandleSpaceStop(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()
  cache := database.GetCacheInstance()

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

  // Get the nomad client
  nomadClient := nomad.NewClient()

  // Stop the job
  err = nomadClient.DeleteSpaceJob(space)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Record the space as not deployed
  space.IsDeployed = false
  db.SaveSpace(space)

  // Delete the agent state
  state, _ := cache.GetAgentState(space.Id)
  if state != nil {
    cache.DeleteAgentState(state)
  }

  w.WriteHeader(http.StatusOK)
}

func HandleUpdateSpace(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()
  user := r.Context().Value("user").(*model.User)

  space, err := db.GetSpace(chi.URLParam(r, "space_id"))
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  if space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
    rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: "space not found"})
    return
  }

  request := SpaceRequest{}
  err = rest.BindJSON(w, r, &request)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  // If template given then ensure the address is removed
  if request.TemplateId != model.MANUAL_TEMPLATE_ID {
    request.AgentURL = ""
  }

  if(!validate.Name(request.Name) || (request.TemplateId == model.MANUAL_TEMPLATE_ID && !validate.Uri(request.AgentURL))) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid name, template, or address given for new space"})
    return
  }
  if(!validate.OneOf(request.Shell, []string{"bash", "zsh", "fish", "sh"})) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid shell given for space"})
    return
  }

  // Lookup the template
  _, err = db.GetTemplate(request.TemplateId)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Unknown template"})
    return
  }

  // Update the space
  space.Name = request.Name
  space.TemplateId = request.TemplateId
  space.AgentURL = request.AgentURL
  space.Shell = request.Shell

  err = db.SaveSpace(space)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  w.WriteHeader(http.StatusOK)
}

func HandleSpaceStopUsersSpaces(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()

  // Get the nomad client
  nomadClient := nomad.NewClient()

  // Stop all spaces
  spaces, err := db.GetSpacesForUser(chi.URLParam(r, "user_id"))
  if err != nil {
    rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
    return
  }

  for _, space := range spaces {
    if space.IsDeployed {
      err = nomadClient.DeleteSpaceJob(space)
      if err != nil {
        rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
        return
      }
    }
  }

  w.WriteHeader(http.StatusOK)
}

func HandleGetSpace(w http.ResponseWriter, r *http.Request) {
  user := r.Context().Value("user").(*model.User)
  spaceId := chi.URLParam(r, "space_id")
  db := database.GetInstance()
  cache := database.GetCacheInstance()

  space, err := db.GetSpace(spaceId)
  if err != nil {
    rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
    return
  }

  if space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
    rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: "space not found"})
    return
  }

  data := struct {
    Name string `json:"name"`
    AgentURL string `json:"agent_url"`
    TemplateId string `json:"template_id"`
    Shell string `json:"shell"`
    HasCodeServer bool `json:"has_code_server"`
    HasSSH bool `json:"has_ssh"`
    HasTerminal bool `json:"has_terminal"`
    IsDeployed bool `json:"is_deployed"`
    Username string `json:"username"`
    UserId string `json:"user_id"`
    VolumeSize map[string]int64 `json:"volume_size"`
  }{
    Name: space.Name,
    AgentURL: space.AgentURL,
    TemplateId: space.TemplateId,
    Shell: space.Shell,
    VolumeSize: space.VolumeSizes,
  }

  // Get the user
  u, err := db.GetUser(space.UserId)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }
  data.Username = u.Username
  data.UserId = u.Id

  // Get the state of the agent
  agentState, _ := cache.GetAgentState(space.Id)
  if agentState != nil {
    data.HasCodeServer = agentState.HasCodeServer
    data.HasSSH = agentState.SSHPort > 0
    data.HasTerminal = agentState.HasTerminal
  }

  rest.SendJSON(http.StatusOK, w, data)
}

func calcSpaceDiskUsage(space *model.Space) (int, error) {
  tmpl, err := database.GetInstance().GetTemplate(space.TemplateId)
  if err != nil {
    return 0, err
  }

  size, err := space.GetStorageSize(tmpl)
  if err != nil {
    return 0, err
  }

  return size, nil
}