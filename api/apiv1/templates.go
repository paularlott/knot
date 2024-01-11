package apiv1

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"

	"github.com/go-chi/chi/v5"
)

type TemplateRequest struct {
  Name string `json:"name"`
  Job string `json:"job"`
  Volumes string `json:"volumes"`
  Groups []string `json:"groups"`
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
    Deployed int `json:"deployed"`
    Groups []string `json:"groups"`
  }, len(templates))

  for i, template := range templates {
    templateData[i].Id = template.Id
    templateData[i].Name = template.Name
    templateData[i].Groups = template.Groups

    // Find the number of spaces using this template
    spaces, err := database.GetInstance().GetSpacesByTemplateId(template.Id)
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

    templateData[i].Usage = len(spaces)
    templateData[i].Deployed = deployed
  }

  rest.SendJSON(http.StatusOK, w, templateData)
}

func HandleUpdateTemplate(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()
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

  if !validate.Required(request.Name) || !validate.MaxLength(request.Name, 255) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid template name given"})
    return
  }
  if !validate.Required(request.Job) || !validate.MaxLength(request.Job, 10*1024*1024) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Job is required and must be less than 10MB"})
    return
  }
  if !validate.MaxLength(request.Volumes, 10*1024*1024) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Volumes must be less than 10MB"})
    return
  }

  // Check the groups are present in the system
  for _, groupId := range request.Groups {
    _, err := db.GetGroup(groupId)
    if err != nil {
      rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: fmt.Sprintf("Group %s does not exist", groupId)})
      return
    }
  }

  template.Name = request.Name
  template.Job = request.Job
  template.Volumes = request.Volumes
  template.UpdatedUserId = user.Id
  template.Groups = request.Groups
  template.UpdateHash()

  err = database.GetInstance().SaveTemplate(template)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  w.WriteHeader(http.StatusOK)
}

func HandleCreateTemplate(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()
  user := r.Context().Value("user").(*model.User)

  request := TemplateRequest{}
  err := rest.BindJSON(w, r, &request)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  if !validate.Required(request.Name) || !validate.MaxLength(request.Name, 255) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid template name given"})
    return
  }
  if !validate.Required(request.Job) || !validate.MaxLength(request.Job, 10*1024*1024) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Job is required and must be less than 10MB"})
    return
  }
  if !validate.MaxLength(request.Volumes, 10*1024*1024) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Volumes must be less than 10MB"})
    return
  }

  // Check the groups are present in the system
  for _, groupId := range request.Groups {
    _, err := db.GetGroup(groupId)
    if err != nil {
      rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: fmt.Sprintf("Group %s does not exist", groupId)})
      return
    }
  }

  template := model.NewTemplate(request.Name, request.Job, request.Volumes, user.Id, request.Groups)

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

  // Don't allow the manual template to be deleted
  if template.Id == model.MANUAL_TEMPLATE_ID {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Manual template cannot be deleted"})
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

func HandleGetTemplate(w http.ResponseWriter, r *http.Request) {
  templateId := chi.URLParam(r, "template_id")

  db := database.GetInstance()
  template, err := db.GetTemplate(templateId)
  if err != nil {
    rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Find the number of spaces using this template
  spaces, err := db.GetSpacesByTemplateId(templateId)
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

  data := struct {
    Name string `json:"name"`
    Job string `json:"job"`
    Volumes string `json:"volumes"`
    Usage int `json:"usage"`
    Deployed int `json:"deployed"`
    Groups []string `json:"groups"`
  }{
    Name: template.Name,
    Job: template.Job,
    Volumes: template.Volumes,
    Usage: len(spaces),
    Deployed: deployed,
    Groups: template.Groups,
  }

  rest.SendJSON(http.StatusOK, w, data)
}
