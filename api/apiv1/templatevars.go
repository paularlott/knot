package apiv1

import (
	"net/http"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"

	"github.com/go-chi/chi/v5"
)

type TemplateVarRequest struct {
  Name string `json:"name"`
  Value string `json:"value"`
}

func HandleGetTemplateVars(w http.ResponseWriter, r *http.Request) {
  templateVars, err := database.GetInstance().GetTemplateVars()
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Build a json array of data to return to the client
  data := make([]struct {
    Id string `json:"templatevar_id"`
    Name string `json:"name"`
    Value string `json:"value"`
  }, len(templateVars))

  for i, variable := range templateVars {
    data[i].Id = variable.Id
    data[i].Name = variable.Name
    data[i].Value = variable.Value
  }

  rest.SendJSON(http.StatusOK, w, data)
}

func HandleUpdateTemplateVar(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()
  user := r.Context().Value("user").(*model.User)

  templateVar, err := db.GetTemplateVar(chi.URLParam(r, "templatevar_id"))
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  request := TemplateVarRequest{}
  err = rest.BindJSON(w, r, &request)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  if !validate.Required(request.Name) || !validate.VarName(request.Name) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid template variable name given"})
    return
  }
  if !validate.MaxLength(request.Value, 10*1024*1024) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Value must be less than 10MB"})
    return
  }

  templateVar.Name = request.Name
  templateVar.Value = request.Value
  templateVar.UpdatedUserId = user.Id

  err = db.SaveTemplateVar(templateVar)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  w.WriteHeader(http.StatusOK)
}

func HandleCreateTemplateVar(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()
  user := r.Context().Value("user").(*model.User)

  request := TemplateVarRequest{}
  err := rest.BindJSON(w, r, &request)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  if !validate.Required(request.Name) || !validate.VarName(request.Name) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid template variable name given"})
    return
  }
  if !validate.MaxLength(request.Value, 10*1024*1024) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Value must be less than 10MB"})
    return
  }

  templateVar := model.NewTemplateVar(request.Name, request.Value, user.Id)

  err = db.SaveTemplateVar(templateVar)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Return the ID
  rest.SendJSON(http.StatusCreated, w, struct {
    Status bool `json:"status"`
    TemplateVarID string `json:"templatevar_id"`
  }{
    Status: true,
    TemplateVarID: templateVar.Id,
  })
}

func HandleDeleteTemplateVar(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()
  templateVar, err := db.GetTemplateVar(chi.URLParam(r, "templatevar_id"))
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Delete the template variable
  err = db.DeleteTemplateVar(templateVar)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  w.WriteHeader(http.StatusOK)
}

func HandleGetTemplateVar(w http.ResponseWriter, r *http.Request) {
  templateVarId := chi.URLParam(r, "templatevar_id")

  db := database.GetInstance()
  templateVar, err := db.GetTemplateVar(templateVarId)
  if err != nil {
    rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
    return
  }

  data := struct {
    Name string `json:"name"`
    Value string `json:"value"`
  }{
    Name: templateVar.Name,
    Value: templateVar.Value,
  }

  rest.SendJSON(http.StatusOK, w, data)
}
