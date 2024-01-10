package apiv1

import (
	"fmt"
	"net/http"
	"time"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/middleware"
	"github.com/paularlott/knot/util/nomad"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"

	"github.com/go-chi/chi/v5"
)

type UserRequest struct {
  Username string `json:"username"`
  Password string `json:"password"`
  Email string `json:"email"`
  Roles []string `json:"roles"`
  Active bool `json:"active"`
  SSHPublicKey string `json:"ssh_public_key"`
  PreferredShell string `json:"preferred_shell"`
}

func HandleCreateUser(w http.ResponseWriter, r *http.Request) {
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

  // Check roles give are present in the system
  for _, role := range request.Roles {
    if !model.RoleExists(role) {
      rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: fmt.Sprintf("Role %s does not exist", role)})
      return
    }
  }

  // Create the user
  userNew := model.NewUser(request.Username, request.Email, request.Password, request.Roles, request.SSHPublicKey, request.PreferredShell)
  err = database.GetInstance().SaveUser(userNew)
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
    Roles []string `json:"roles"`
    Active bool `json:"active"`
    SSHPublicKey string `json:"ssh_public_key"`
    PreferredShell string `json:"preferred_shell"`
    Current bool `json:"current"`
    LastLoginAt *time.Time `json:"last_login_at"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
  }{
    Id: user.Id,
    Username: user.Username,
    Email: user.Email,
    Roles: user.Roles,
    Active: user.Active,
    SSHPublicKey: user.SSHPublicKey,
    PreferredShell: user.PreferredShell,
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
  Active bool `json:"active"`
  Current bool `json:"current"`
  LastLoginAt *time.Time `json:"last_login_at"`
  NumberSpaces int `json:"number_spaces"`
  NumberSpacesDeployed int `json:"number_spaces_deployed"`
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
      data.Active = user.Active
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
      for _, space := range spaces {
        if space.IsDeployed {
          deployed++
        }
      }

      data.NumberSpaces = len(spaces)
      data.NumberSpacesDeployed = deployed

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

  // Check roles give are present in the system
  for _, role := range request.Roles {
    if !model.RoleExists(role) {
      rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: fmt.Sprintf("Role %s does not exist", role)})
      return
    }
  }

  // Load the existing user
  db := database.GetInstance()
  user, err := db.GetUser(userId)
  if err != nil {
    rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Update the user
  if len(request.Password) > 0 {
    user.SetPassword(request.Password)
  }
  user.Email = request.Email
  user.SSHPublicKey = request.SSHPublicKey
  user.PreferredShell = request.PreferredShell

  if activeUser.HasPermission(model.PermissionManageUsers) {
    user.Active = request.Active
    user.Roles = request.Roles
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
