package apiv1

import (
	"errors"
	"net/http"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"

	"github.com/go-chi/chi/v5"
)

type UserGroupRequest struct {
  Name string `json:"name"`
}

func HandleGetGroups(w http.ResponseWriter, r *http.Request) {
  groups, err := database.GetInstance().GetGroups()
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Build a json array of data to return to the client
  data := make([]struct {
    Id string `json:"group_id"`
    Name string `json:"name"`
  }, len(groups))

  for i, group := range groups {
    data[i].Id = group.Id
    data[i].Name = group.Name
  }

  rest.SendJSON(http.StatusOK, w, data)
}

func HandleUpdateGroup(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()
  user := r.Context().Value("user").(*model.User)

  group, err := db.GetGroup(chi.URLParam(r, "group_id"))
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  request := UserGroupRequest{}
  err = rest.BindJSON(w, r, &request)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  if !validate.Required(request.Name) || !validate.MaxLength(request.Name, 64) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid user group name"})
    return
  }

  group.Name = request.Name
  group.UpdatedUserId = user.Id

  err = db.SaveGroup(group)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  w.WriteHeader(http.StatusOK)
}

func HandleCreateGroup(w http.ResponseWriter, r *http.Request) {
  user := r.Context().Value("user").(*model.User)

  request := UserGroupRequest{}
  err := rest.BindJSON(w, r, &request)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  if !validate.Required(request.Name) || !validate.MaxLength(request.Name, 64) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid user group name"})
    return
  }

  group := model.NewGroup(request.Name, user.Id)

  err = database.GetInstance().SaveGroup(group)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Return the ID
  rest.SendJSON(http.StatusCreated, w, struct {
    Status bool `json:"status"`
    GroupID string `json:"group_id"`
  }{
    Status: true,
    GroupID: group.Id,
  })
}

func HandleDeleteGroup(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()
  group, err := db.GetGroup(chi.URLParam(r, "group_id"))
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Delete the group
  err = db.DeleteGroup(group)
  if err != nil {
    if errors.Is(err, database.ErrTemplateInUse) {
      rest.SendJSON(http.StatusLocked, w, ErrorResponse{Error: err.Error()})
    } else {
      rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    }
    return
  }

  w.WriteHeader(http.StatusOK)
}

func HandleGetGroup(w http.ResponseWriter, r *http.Request) {
  groupId := chi.URLParam(r, "group_id")

  db := database.GetInstance()
  group, err := db.GetGroup(groupId)
  if err != nil {
    rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
    return
  }

  data := struct {
    Name string `json:"name"`
  }{
    Name: group.Name,
  }

  rest.SendJSON(http.StatusOK, w, data)
}
