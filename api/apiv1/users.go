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
}

func HandleCreateUser(w http.ResponseWriter, r *http.Request) {
  request := CreateUserRequest{}

  err := rest.BindJSON(w, r, &request)
  if err != nil {
    w.WriteHeader(http.StatusBadRequest)
    rest.SendJSON(w, ErrorResponse{Error: err.Error()})
    return
  }

  // Validate
  if(!validate.Username(request.Username) ||
    !validate.Password(request.Password) ||
    !validate.Email(request.Email)) {
    w.WriteHeader(http.StatusBadRequest)
    rest.SendJSON(w, ErrorResponse{Error: "Invalid username, password, or email"})
    return
  }

  // Create the user
  user := model.NewUser(request.Username, request.Email, request.Password)
  err = database.GetInstance().SaveUser(user)

  if err != nil {
    w.WriteHeader(http.StatusBadRequest)
    rest.SendJSON(w, ErrorResponse{Error: err.Error()})
    return
  }

  // Tell the middleware that users are present
  middleware.HasUsers = true

  // Return the user ID
  w.WriteHeader(http.StatusCreated)
  rest.SendJSON(w, struct {
    Status bool `json:"status"`
    UserID string `json:"user_id"`
  }{
    Status: true,
    UserID: user.Id,
  })
}
