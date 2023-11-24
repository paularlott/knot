package apiv1

import (
	"net/http"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/middleware"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"
)

type CreateUserRequest struct {
  Username string `json:"username"`
  Password string `json:"password"`
  Email string `json:"email"`
  IsAdmin bool `json:"is_admin"`
}

func HandleCreateUser(w http.ResponseWriter, r *http.Request) {
  request := CreateUserRequest{}

  err := rest.BindJSON(w, r, &request)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Validate
  if(!validate.Name(request.Username) ||
    !validate.Password(request.Password) ||
    !validate.Email(request.Email)) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid username, password, or email given for new user"})
    return
  }

  // If users in the system then only admins can create users
  if middleware.HasUsers {
    user := r.Context().Value("user").(*model.User)
    if !user.IsAdmin {
      rest.SendJSON(http.StatusForbidden, w, ErrorResponse{Error: "Users can only be created by admins"})
      return
    }
  }

  // Create the user
  userNew := model.NewUser(request.Username, request.Email, request.Password, request.IsAdmin)
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
