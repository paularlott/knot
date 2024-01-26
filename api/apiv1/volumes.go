package apiv1

import (
	"net/http"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/nomad"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"

	"github.com/go-chi/chi/v5"
)

type VolumeRequest struct {
  Name string `json:"name"`
  Definition string `json:"definition"`
}

func HandleGetVolumes(w http.ResponseWriter, r *http.Request) {
  volumes, err := database.GetInstance().GetVolumes()
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Build a json array of data to return to the client
  volumeData := make([]struct {
    Id string `json:"volume_id"`
    Name string `json:"name"`
    Active bool `json:"active"`
  }, len(volumes))

  for i, volume := range volumes {
    volumeData[i].Id = volume.Id
    volumeData[i].Name = volume.Name
    volumeData[i].Active = volume.Active
  }

  rest.SendJSON(http.StatusOK, w, volumeData)
}

func HandleUpdateVolume(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()
  user := r.Context().Value("user").(*model.User)

  volume, err := database.GetInstance().GetVolume(chi.URLParam(r, "volume_id"))
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  request := VolumeRequest{}
  err = rest.BindJSON(w, r, &request)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  if !validate.Required(request.Name) || !validate.MaxLength(request.Name, 64) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid volume name given"})
    return
  }
  if !validate.MaxLength(request.Definition, 10*1024*1024) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Volume definition must be less than 10MB"})
    return
  }

  volume.Name = request.Name
  volume.Definition = request.Definition
  volume.UpdatedUserId = user.Id

  err = db.SaveVolume(volume)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  w.WriteHeader(http.StatusOK)
}

func HandleCreateVolume(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()
  user := r.Context().Value("user").(*model.User)

  request := VolumeRequest{}
  err := rest.BindJSON(w, r, &request)
  if err != nil {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
    return
  }

  if !validate.Required(request.Name) || !validate.MaxLength(request.Name, 64) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid volume name given"})
    return
  }
  if !validate.MaxLength(request.Definition, 10*1024*1024) {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Volume definition must be less than 10MB"})
    return
  }

  volume := model.NewVolume(request.Name, request.Definition, user.Id)

  err = db.SaveVolume(volume)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Return the ID
  rest.SendJSON(http.StatusCreated, w, struct {
    Status bool `json:"status"`
    TemplateID string `json:"volume_id"`
  }{
    Status: true,
    TemplateID: volume.Id,
  })
}

func HandleDeleteVolume(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()

  volume, err := db.GetVolume(chi.URLParam(r, "volume_id"))
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // If the volume is active then don't delete
  if volume.Active {
    rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Cannot delete an active volume"})
    return
  }

  // Delete the volume
  err = database.GetInstance().DeleteVolume(volume)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  w.WriteHeader(http.StatusOK)
}

func HandleGetVolume(w http.ResponseWriter, r *http.Request) {
  volumeId := chi.URLParam(r, "volume_id")

  db := database.GetInstance()
  volume, err := db.GetVolume(volumeId)
  if err != nil {
    rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
    return
  }

  data := struct {
    Name string `json:"name"`
    Definition string `json:"definition"`
    Active bool `json:"active"`
  }{
    Name: volume.Name,
    Definition: volume.Definition,
    Active: volume.Active,
  }

  rest.SendJSON(http.StatusOK, w, data)
}

func HandleVolumeStart(w http.ResponseWriter, r *http.Request) {
//  user := r.Context().Value("user").(*model.User)
  db := database.GetInstance()

  volume, err := db.GetVolume(chi.URLParam(r, "volume_id"))

  if err != nil {
    rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
    return
  }

  // If the volume is already running then fail
  if volume.Active {
    rest.SendJSON(http.StatusLocked, w, ErrorResponse{Error: "volume is running"})
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
  err = nomadClient.CreateVolume(volume, &vars)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Mark volume as started
  volume.Active = true
  db.SaveVolume(volume)

  w.WriteHeader(http.StatusOK)
}

func HandleVolumeStop(w http.ResponseWriter, r *http.Request) {
  db := database.GetInstance()

  volume, err := db.GetVolume(chi.URLParam(r, "volume_id"))
  if err != nil {
    rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
    return
  }

  // If the volume is not running then fail
  if !volume.Active {
    rest.SendJSON(http.StatusLocked, w, ErrorResponse{Error: "volume not running"})
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
  err = nomadClient.DeleteVolume(volume, &vars)
  if err != nil {
    rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
    return
  }

  // Record the space as not deployed
  volume.Active = false
  db.SaveVolume(volume)

  w.WriteHeader(http.StatusOK)
}
