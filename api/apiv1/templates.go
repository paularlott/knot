package apiv1

import (
	"errors"
	"fmt"
	"math"
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
  Description string `json:"description"`
  Volumes string `json:"volumes"`
  Groups []string `json:"groups"`
}

type TemplateResponse struct {
  Id string `json:"template_id"`
  Name string `json:"name"`
  Description string `json:"description"`
  Usage int `json:"usage"`
  Deployed int `json:"deployed"`
  Groups []string `json:"groups"`
}

func HandleGetTemplates(w http.ResponseWriter, r *http.Request) {
  user := r.Context().Value("user").(*model.User)

  // Get the query parameter user_id if present load the user
  userId := r.URL.Query().Get("user_id")
  if userId != "" {
    if !user.HasPermission(model.PermissionManageSpaces) {
      rest.SendJSON(http.StatusForbidden, w, ErrorResponse{Error: "Permission denied"})
      return
    }

    var err error
    user, err = database.GetInstance().GetUser(userId)
    if err != nil {
      rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
      return
    }
    if user == nil {
      rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: "User not found"})
      return
    }
  }

  templates, err := database.GetInstance().GetTemplates()
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Build a json array of data to return to the client
  templateResponse := []*TemplateResponse{}

  for _, template := range templates {

    // If the template has groups and no overlap with the user's groups then skip
    if !user.HasPermission(model.PermissionManageTemplates) && len(template.Groups) > 0 && !user.HasAnyGroup(&template.Groups) {
      continue
    }

    templateData := &TemplateResponse{}

    templateData.Id = template.Id
    templateData.Name = template.Name
    templateData.Description = template.Description
    templateData.Groups = template.Groups

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

    templateData.Usage = len(spaces)
    templateData.Deployed = deployed

    templateResponse = append(templateResponse, templateData)
  }

  rest.SendJSON(http.StatusOK, w, templateResponse)
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
  template.Description = request.Description
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

  template := model.NewTemplate(request.Name, request.Description, request.Job, request.Volumes, user.Id, request.Groups)

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

  volumes, _ := template.GetVolumes(nil, nil, nil, false);

  var volumeList []map[string]interface{}
  for _, volume := range volumes.Volumes {
    volumeList = append(volumeList, map[string]interface{}{
      "id":           volume.Id,
      "name":         volume.Name,
      "capacity_min": math.Max(1, math.Ceil(float64(volume.CapacityMin.(int64)) / (1024 * 1024 * 1024))),
      "capacity_max": math.Max(1, math.Ceil(float64(volume.CapacityMax.(int64)) / (1024 * 1024 * 1024))),
    })
  }

  data := struct {
    Name string `json:"name"`
    Job string `json:"job"`
    Description string `json:"description"`
    Volumes string `json:"volumes"`
    Usage int `json:"usage"`
    Deployed int `json:"deployed"`
    Groups []string `json:"groups"`
    VolumeSizes []map[string]interface{} `json:"volume_sizes"`
  }{
    Name: template.Name,
    Description: template.Description,
    Job: template.Job,
    Volumes: template.Volumes,
    Usage: len(spaces),
    Deployed: deployed,
    Groups: template.Groups,
    VolumeSizes: volumeList,
  }

  rest.SendJSON(http.StatusOK, w, data)
}
