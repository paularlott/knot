package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
)

func HandleGetVolumes(w http.ResponseWriter, r *http.Request) {
	volumes, err := database.GetInstance().GetVolumes()
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Build a json array of data to return to the client
	volumeData := apiclient.VolumeInfoList{
		Count:   0,
		Volumes: []apiclient.VolumeInfo{},
	}

	for _, volume := range volumes {
		if volume.IsDeleted {
			continue
		}

		v := apiclient.VolumeInfo{
			Id:             volume.Id,
			Name:           volume.Name,
			Active:         volume.Active,
			Location:       volume.Location,
			LocalContainer: volume.LocalContainer,
		}
		volumeData.Volumes = append(volumeData.Volumes, v)
		volumeData.Count++
	}

	rest.SendJSON(http.StatusOK, w, r, volumeData)
}

func HandleUpdateVolume(w http.ResponseWriter, r *http.Request) {
	volumeId := r.PathValue("volume_id")
	if !validate.UUID(volumeId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid volume ID"})
		return
	}

	request := apiclient.VolumeUpdateRequest{}
	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.Required(request.Name) || !validate.MaxLength(request.Name, 64) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid volume name given"})
		return
	}
	if !validate.MaxLength(request.Definition, 10*1024*1024) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Volume definition must be less than 10MB"})
		return
	}

	db := database.GetInstance()
	user := r.Context().Value("user").(*model.User)

	volume, err := database.GetInstance().GetVolume(volumeId)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// If the volume is active then don't update
	if volume.Active {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Cannot update an active volume"})
		return
	}

	volume.Name = request.Name
	volume.Definition = request.Definition
	volume.UpdatedUserId = user.Id
	volume.UpdatedAt = time.Now().UTC()

	err = db.SaveVolume(volume, []string{"Name", "Definition", "UpdatedUserId"})
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipVolume(volume)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventVolumeUpdate,
		fmt.Sprintf("Updated volume %s", volume.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"volume_id":       volume.Id,
			"volume_name":     volume.Name,
		},
	)

	w.WriteHeader(http.StatusOK)
}

func HandleCreateVolume(w http.ResponseWriter, r *http.Request) {
	request := apiclient.VolumeCreateRequest{}
	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.Required(request.Name) || !validate.MaxLength(request.Name, 64) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid volume name given"})
		return
	}
	if !validate.MaxLength(request.Definition, 10*1024*1024) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Volume definition must be less than 10MB"})
		return
	}

	db := database.GetInstance()
	user := r.Context().Value("user").(*model.User)

	volume := model.NewVolume(request.Name, request.Definition, user.Id, request.LocalContainer)

	err = db.SaveVolume(volume, nil)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipVolume(volume)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventVolumeCreate,
		fmt.Sprintf("Created volume %s", volume.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"volume_id":       volume.Id,
			"volume_name":     volume.Name,
		},
	)

	// Return the ID
	rest.SendJSON(http.StatusCreated, w, r, &apiclient.VolumeCreateResponse{
		Status:   true,
		VolumeId: volume.Id,
	})
}

func HandleDeleteVolume(w http.ResponseWriter, r *http.Request) {
	volumeId := r.PathValue("volume_id")
	if !validate.UUID(volumeId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid volume ID"})
		return
	}

	db := database.GetInstance()

	volume, err := db.GetVolume(volumeId)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// If the volume is active then don't delete
	if volume.Active {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Cannot delete an active volume"})
		return
	}

	// Delete the volume
	volume.IsDeleted = true
	volume.UpdatedAt = time.Now().UTC()
	volume.UpdatedUserId = r.Context().Value("user").(*model.User).Id
	err = db.SaveVolume(volume, []string{"IsDeleted", "UpdatedAt", "UpdatedUserId"})
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipVolume(volume)

	user := r.Context().Value("user").(*model.User)
	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventVolumeDelete,
		fmt.Sprintf("Deleted volume %s", volume.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"volume_id":       volume.Id,
			"volume_name":     volume.Name,
		},
	)

	w.WriteHeader(http.StatusOK)
}

func HandleGetVolume(w http.ResponseWriter, r *http.Request) {
	volumeId := r.PathValue("volume_id")
	if !validate.UUID(volumeId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid volume ID"})
		return
	}

	db := database.GetInstance()
	volume, err := db.GetVolume(volumeId)
	if err != nil {
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	data := apiclient.VolumeDefinition{
		Name:           volume.Name,
		Definition:     volume.Definition,
		Active:         volume.Active,
		Location:       volume.Location,
		LocalContainer: volume.LocalContainer,
	}

	rest.SendJSON(http.StatusOK, w, r, &data)
}

func HandleVolumeStart(w http.ResponseWriter, r *http.Request) {
	var volume *model.Volume
	var err error

	db := database.GetInstance()

	volumeId := r.PathValue("volume_id")

	if !validate.UUID(volumeId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid volume ID"})
		return
	}

	transport := service.GetTransport()
	unlockToken := transport.LockResource(volumeId)
	if unlockToken == "" {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: "Failed to lock volume"})
		return
	}
	defer transport.UnlockResource(volumeId, unlockToken)

	volume, err = db.GetVolume(volumeId)
	if err != nil {
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// If the volume is already running then fail
	if volume.Active {
		rest.SendJSON(http.StatusLocked, w, r, ErrorResponse{Error: "volume is running"})
		return
	}

	// If the volume has a location and it is not this server then fail
	if volume.Location != "" && volume.Location != config.Location {
		rest.SendJSON(http.StatusLocked, w, r, ErrorResponse{Error: "volume is used by another server"})
		return
	}

	err = service.GetContainerService().CreateVolume(volume)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	volume.Location = config.Location
	volume.Active = true
	volume.UpdatedAt = time.Now().UTC()
	volume.UpdatedUserId = r.Context().Value("user").(*model.User).Id
	db.SaveVolume(volume, []string{"Active", "Location", "UpdatedAt", "UpdatedUserId"})
	service.GetTransport().GossipVolume(volume)

	rest.SendJSON(http.StatusOK, w, r, &apiclient.StartVolumeResponse{
		Status:   true,
		Location: volume.Location,
	})
}

func HandleVolumeStop(w http.ResponseWriter, r *http.Request) {
	var volume *model.Volume
	var err error

	db := database.GetInstance()

	volumeId := r.PathValue("volume_id")
	if !validate.UUID(volumeId) {
		rest.SendJSON(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid volume ID"})
		return
	}

	transport := service.GetTransport()
	unlockToken := transport.LockResource(volumeId)
	if unlockToken == "" {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: "Failed to lock volume"})
		return
	}
	defer transport.UnlockResource(volumeId, unlockToken)

	volume, err = db.GetVolume(volumeId)
	if err != nil {
		rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// If the volume is not running or not this server then fail
	if !volume.Active || volume.Location != config.Location {
		rest.SendJSON(http.StatusLocked, w, r, ErrorResponse{Error: "volume not running"})
		return
	}

	err = service.GetContainerService().DeleteVolume(volume)
	if err != nil && !strings.Contains(err.Error(), "volume not found") {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	volume.Location = ""
	volume.Active = false
	volume.UpdatedAt = time.Now().UTC()
	volume.UpdatedUserId = r.Context().Value("user").(*model.User).Id
	db.SaveVolume(volume, []string{"Active", "Location", "UpdatedAt", "UpdatedUserId"})
	service.GetTransport().GossipVolume(volume)

	w.WriteHeader(http.StatusOK)
}
