package apiv1

import (
	"fmt"
	"net/http"
	"time"

	"github.com/paularlott/knot/api/agentv1"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/middleware"
	"github.com/paularlott/knot/util"
	"github.com/paularlott/knot/util/nomad"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"

	"github.com/go-chi/chi/v5"
)

type UserRequest struct {
  Username string `json:"username"`
  Password string `json:"password"`
  ServicePassword string `json:"service_password"`
  Email string `json:"email"`
  Roles []string `json:"roles"`
  Groups []string `json:"groups"`
  Active bool `json:"active"`
  MaxSpaces int `json:"max_spaces"`
  MaxDiskSpace int `json:"max_disk_space"`
  SSHPublicKey string `json:"ssh_public_key"`
  PreferredShell string `json:"preferred_shell"`
  Timezone string `json:"timezone"`
}

func HandleCreateUser(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()
  request := UserRequest{}

  err := rest.BindJSON(w, r, &request)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Validate
  if !validate.Name(request.Username) ||
    !validate.Password(request.Password) ||
    !validate.Email(request.Email) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid username, password, or email given for new user"})
    return
  }
  if !validate.OneOf(request.PreferredShell, []string{"bash", "zsh", "fish", "sh"}) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid shell"})
    return
  }
  if !validate.MaxLength(request.SSHPublicKey, 16*1024) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "SSH public key too long"})
    return
  }
  if !validate.OneOf(request.Timezone, util.Timezones) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid timezone"})
    return
  }
  if !validate.IsNumber(int(request.MaxSpaces), 0, 1000) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid max spaces"})
    return
  }

  // Check roles give are present in the system
  for _, role := range request.Roles {
    if !model.RoleExists(role) {
      rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: fmt.Sprintf("Role %s does not exist", role)})
      return
    }
  }

  // Check the groups are present in the system
  for _, groupId := range request.Groups {
    _, err := db.GetGroup(groupId)
    if err != nil {
      rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: fmt.Sprintf("Group %s does not exist", groupId)})
      return
    }
  }

  // Create the user
  userNew := model.NewUser(request.Username, request.Email, request.Password, request.Roles, request.Groups, request.SSHPublicKey, request.PreferredShell, request.Timezone, request.MaxSpaces, request.MaxDiskSpace)
  if request.ServicePassword != "" {
    userNew.ServicePassword = request.ServicePassword
  }
  err = db.SaveUser(userNew)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Tell the middleware that users are present
  middleware.HasUsers = true

  // Return the user ID
  rest.SendJSON(http.StatusCreated, w, struct {
    Status bool `json:"status"`
    UserID string `json:"user_id"`
  }{
    Status: true,
    UserID: userNew.Id,
  })
}

func HandleGetUser(w http.ResponseWriter, r *http.Request) {
  activeUser := r.Context().Value("user").(*model.User)
  userId := chi.URLParam(r, "user_id")

  db := database.GetInstance()
  user, err := db.GetUser(userId)
  if err != nil {
    rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Build a json array of data to return to the client
  userData := struct {
    Id string `json:"user_id"`
    Username string `json:"username"`
    Email string `json:"email"`
    ServicePassword string `json:"service_password"`
    Roles []string `json:"roles"`
    Groups []string `json:"groups"`
    Active bool `json:"active"`
    MaxSpaces int `json:"max_spaces"`
    MaxDiskSpace int `json:"max_disk_space"`
    SSHPublicKey string `json:"ssh_public_key"`
    PreferredShell string `json:"preferred_shell"`
    Timezone string `json:"timezone"`
    Current bool `json:"current"`
    LastLoginAt *time.Time `json:"last_login_at"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
  }{
    Id: user.Id,
    Username: user.Username,
    Email: user.Email,
    ServicePassword: user.ServicePassword,
    Roles: user.Roles,
    Groups: user.Groups,
    Active: user.Active,
    MaxSpaces: user.MaxSpaces,
    MaxDiskSpace: user.MaxDiskSpace,
    SSHPublicKey: user.SSHPublicKey,
    PreferredShell: user.PreferredShell,
    Timezone: user.Timezone,
    Current: user.Id == activeUser.Id,
    LastLoginAt: nil,
    CreatedAt: user.CreatedAt.UTC(),
    UpdatedAt: user.UpdatedAt.UTC(),
  }

  if user.LastLoginAt != nil {
    t := user.LastLoginAt.UTC()
    userData.LastLoginAt = &t
  }

  rest.SendJSON(http.StatusOK, w, userData)
}

type UserInfoResponse struct {
  Id string `json:"user_id"`
  Username string `json:"username"`
  Email string `json:"email"`
  Roles []string `json:"roles"`
  Groups []string `json:"groups"`
  Active bool `json:"active"`
  MaxSpaces int `json:"max_spaces"`
  MaxDiskSpace int `json:"max_disk_space"`
  Current bool `json:"current"`
  LastLoginAt *time.Time `json:"last_login_at"`
  NumberSpaces int `json:"number_spaces"`
  NumberSpacesDeployed int `json:"number_spaces_deployed"`
  UsedDiskSpace int `json:"used_disk_space"`
}

func HandleGetUsers(w http.ResponseWriter, r *http.Request) {
  activeUser := r.Context().Value("user").(*model.User)
  requiredState := r.URL.Query().Get("state")
  if requiredState == "" {
    requiredState = "all"
  }

  db := database.GetInstance()
  users, err := db.GetUsers()
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Build a json array of data to return to the client
  var userData []*UserInfoResponse

  for _, user := range users {
    if requiredState == "all" || (requiredState == "active" && user.Active) || (requiredState == "inactive" && !user.Active) {
      data := UserInfoResponse{}

      data.Id = user.Id
      data.Username = user.Username
      data.Email = user.Email
      data.Roles = user.Roles
      data.Groups = user.Groups
      data.Active = user.Active
      data.MaxSpaces = user.MaxSpaces
      data.MaxDiskSpace = user.MaxDiskSpace
      data.Current = user.Id == activeUser.Id

      if user.LastLoginAt != nil {
        t := user.LastLoginAt.UTC()
        data.LastLoginAt = &t
      } else {
        data.LastLoginAt = nil
      }

      // Find the number of spaces the user has
      spaces, err := db.GetSpacesForUser(user.Id)
      if err != nil {
        rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
        return
      }

      var deployed int = 0
      var diskSpace int = 0
      for _, space := range spaces {
        if space.IsDeployed {
          deployed++
        }

        sSize, err := calcSpaceDiskUsage(space)
        if err != nil {
          rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
          return
        }

        diskSpace += sSize
      }

      data.NumberSpaces = len(spaces)
      data.NumberSpacesDeployed = deployed
      data.UsedDiskSpace = diskSpace

      userData = append(userData, &data)
    }
  }

  rest.SendJSON(http.StatusOK, w, userData)
}

func HandleUpdateUser(w http.ResponseWriter, r *http.Request) {
  activeUser := r.Context().Value("user").(*model.User)
  userId := chi.URLParam(r, "user_id")
  request := UserRequest{}

  err := rest.BindJSON(w, r, &request)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Validate
  if (len(request.Password) > 0 && !validate.Password(request.Password)) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid password given"})
    return
  }
  if !validate.Email(request.Email) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid email"})
    return
  }
  if !validate.OneOf(request.PreferredShell, []string{"bash", "zsh", "fish", "sh"}) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid shell"})
    return
  }
  if !validate.MaxLength(request.SSHPublicKey, 16*1024) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "SSH public key too long"})
    return
  }
  if !validate.OneOf(request.Timezone, util.Timezones) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid timezone"})
    return
  }

  // Load the existing user
  db := database.GetInstance()
  user, err := db.GetUser(userId)
  if err != nil {
    rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Update the user password
  if len(request.Password) > 0 {
    user.SetPassword(request.Password)
  }
  user.Email = request.Email
  user.SSHPublicKey = request.SSHPublicKey
  user.PreferredShell = request.PreferredShell
  user.Timezone = request.Timezone

  if request.ServicePassword != "" {
    user.ServicePassword = request.ServicePassword
  }

  if activeUser.HasPermission(model.PermissionManageUsers) {
    if !validate.IsNumber(int(request.MaxSpaces), 0, 1000) {
      rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid max spaces"})
      return
    }

    // Check roles give are present in the system
    for _, role := range request.Roles {
      if !model.RoleExists(role) {
        rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: fmt.Sprintf("Role %s does not exist", role)})
        return
      }
    }

    // Check the groups are present in the system
    for _, groupId := range request.Groups {
      _, err := db.GetGroup(groupId)
      if err != nil {
        rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: fmt.Sprintf("Group %s does not exist", groupId)})
        return
      }
    }

    if activeUser.Id != user.Id {
      user.Active = request.Active
    }

    user.Roles = request.Roles
    user.Groups = request.Groups
    user.MaxSpaces = request.MaxSpaces
    user.MaxDiskSpace = request.MaxDiskSpace
  }

  // Save
  err = db.SaveUser(user)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  // If the user is disabled then stop all spaces
  if !user.Active {
    spaces, err := db.GetSpacesForUser(user.Id)
    if err != nil {
      rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
      return
    }

    // Get the nomad client
    nomadClient := nomad.NewClient()
    for _, space := range spaces {
      nomadClient.DeleteSpaceJob(space)
    }
  } else {
    // Update the SSH key on the agents
    go updateSpacesSSHKey(user)
  }

  w.WriteHeader(http.StatusOK)
}

func HandleDeleteUser(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()
  user := r.Context().Value("user").(*model.User)
  userId := chi.URLParam(r, "user_id")

  // If trying to delete self then fail
  if user.Id == userId {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Cannot delete self"})
    return
  }

  // Load the user to delete
  toDelete, err := db.GetUser(userId)
  if err != nil {
    rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: fmt.Sprintf("user %s not found", userId)})
    return
  }

  // Stop all spaces and delete all volumes
  spaces, err := db.GetSpacesForUser(toDelete.Id)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Get the nomad client
  nomadClient := nomad.NewClient()
  for _, space := range spaces {
    // Stop the job
    err = nomadClient.DeleteSpaceJob(space)
    if err != nil {
      rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
      return
    }

    // Delete the volumes
    err = nomadClient.DeleteSpaceVolumes(space)
    if err != nil {
      rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
      return
    }
  }

  // Delete the user
  err = db.DeleteUser(toDelete)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  w.WriteHeader(http.StatusOK)
}

func updateSpacesSSHKey(user *model.User) {
  db := database.GetInstance()
  cache := database.GetCacheInstance()

  log.Debug().Msgf("Updating agent SSH key for user %s", user.Id)

  // Load the list of spaces for the user
  spaces, err := db.GetSpacesForUser(user.Id)
  if err != nil {
    log.Debug().Msgf("Failed to get spaces for user %s: %s", user.Id, err)
    return
  }

  // Loop through all spaces updating the active ones
  for _, space := range spaces {
    if space.IsDeployed {
      // Get the agent state
      agentState, err := cache.GetAgentState(space.Id)
      if err != nil && agentState != nil {
        log.Debug().Msgf("Failed to get agent state for space %s: %s", space.Id, err)
        continue
      }

      log.Debug().Msgf("Sending SSH public key to agent %s", space.Id)
      client := rest.NewClient(util.ResolveSRVHttp(space.GetAgentURL(), viper.GetString("server.namespace")), agentState.AccessToken, viper.GetBool("tls_skip_verify"))
      if !agentv1.CallAgentUpdateAuthorizedKeys(client, user.SSHPublicKey) {
        log.Debug().Msg("Failed to send SSH public key to agent")
      }
    }
  }

  log.Debug().Msgf("Finished updating agent SSH key for user %s", user.Id)
}
