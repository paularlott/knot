package apiv1

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/rest"
)

type TemplateRequest struct {
  Name string `json:"name"`
  Job string `json:"job"`
}

func HandleGetTemplates(w http.ResponseWriter, r *http.Request) {
  templates, err := database.GetInstance().GetTemplates()
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Build a json array of data to return to the client
  templateData := make([]struct {
    Id string `json:"template_id"`
    Name string `json:"name"`
    Usage int `json:"usage"`
  }, len(templates))

  for i, template := range templates {
    templateData[i].Id = template.Id
    templateData[i].Name = template.Name

    // Find the number of spaces using this template
    spaces, err := database.GetInstance().GetSpacesByTemplateId(template.Id)
    if err != nil {
      rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
      return
    }

    templateData[i].Usage = len(spaces)
  }

  rest.SendJSON(http.StatusOK, w, templateData)
}

func HandleUpdateTemplate(w http.ResponseWriter, r *http.Request) {
  user := r.Context().Value("user").(*model.User)

  template, err := database.GetInstance().GetTemplate(chi.URLParam(r, "template_id"))
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  request := TemplateRequest{}
  err = rest.BindJSON(w, r, &request)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  template.Name = request.Name
  template.Job = request.Job
  template.UpdatedUserId = user.Id

  err = database.GetInstance().SaveTemplate(template)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  w.WriteHeader(http.StatusOK)
}

func HandleCreateTemplate(w http.ResponseWriter, r *http.Request) {
  user := r.Context().Value("user").(*model.User)

  request := TemplateRequest{}
  err := rest.BindJSON(w, r, &request)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  template := model.NewTemplate(request.Name, request.Job, user.Id)

  err = database.GetInstance().SaveTemplate(template)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Return the ID
  rest.SendJSON(http.StatusCreated, w, struct {
    Status bool `json:"status"`
    TemplateID string `json:"template_id"`
  }{
    Status: true,
    TemplateID: template.Id,
  })
}

func HandleDeleteTemplate(w http.ResponseWriter, r *http.Request) {
  template, err := database.GetInstance().GetTemplate(chi.URLParam(r, "template_id"))
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Delete the template
  err = database.GetInstance().DeleteTemplate(template)
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
