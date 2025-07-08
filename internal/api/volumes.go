package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/paularlott/gossip/hlc"
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
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
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
			Id:       volume.Id,
			Name:     volume.Name,
			Active:   volume.Active,
			Zone:     volume.Zone,
			Platform: volume.Platform,
		}
		volumeData.Volumes = append(volumeData.Volumes, v)
		volumeData.Count++
	}

	rest.WriteResponse(http.StatusOK, w, r, volumeData)
}

func HandleUpdateVolume(w http.ResponseWriter, r *http.Request) {
	volumeId := r.PathValue("volume_id")
	if !validate.UUID(volumeId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid volume ID"})
		return
	}

	request := apiclient.VolumeUpdateRequest{}
	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.Required(request.Name) || !validate.MaxLength(request.Name, 64) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid volume name given"})
		return
	}
	if !validate.MaxLength(request.Definition, 10*1024*1024) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Volume definition must be less than 10MB"})
		return
	}
	if !validate.OneOf(request.Platform, []string{model.PlatformDocker, model.PlatformPodman, model.PlatformNomad}) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid platform name given"})
		return
	}

	db := database.GetInstance()
	user := r.Context().Value("user").(*model.User)

	volume, err := database.GetInstance().GetVolume(volumeId)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// If the volume is active then don't update
	if volume.Active {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Cannot update an active volume"})
		return
	}

	volume.Name = request.Name
	volume.Definition = request.Definition
	volume.UpdatedUserId = user.Id
	volume.Platform = request.Platform
	volume.UpdatedAt = hlc.Now()

	err = db.SaveVolume(volume, []string{"Name", "Definition", "UpdatedUserId", "Platform", "UpdatedAt"})
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
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
	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.Required(request.Name) || !validate.MaxLength(request.Name, 64) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid volume name given"})
		return
	}
	if !validate.MaxLength(request.Definition, 10*1024*1024) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Volume definition must be less than 10MB"})
		return
	}
	if !validate.OneOf(request.Platform, []string{model.PlatformDocker, model.PlatformPodman, model.PlatformNomad}) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid platform name given"})
		return
	}

	db := database.GetInstance()
	user := r.Context().Value("user").(*model.User)

	volume := model.NewVolume(request.Name, request.Definition, user.Id, request.Platform)

	err = db.SaveVolume(volume, nil)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
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
	rest.WriteResponse(http.StatusCreated, w, r, &apiclient.VolumeCreateResponse{
		Status:   true,
		VolumeId: volume.Id,
	})
}

func HandleDeleteVolume(w http.ResponseWriter, r *http.Request) {
	volumeId := r.PathValue("volume_id")
	if !validate.UUID(volumeId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid volume ID"})
		return
	}

	db := database.GetInstance()

	volume, err := db.GetVolume(volumeId)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// If the volume is active then don't delete
	if volume.Active {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Cannot delete an active volume"})
		return
	}

	// Delete the volume
	volume.IsDeleted = true
	volume.UpdatedAt = hlc.Now()
	volume.UpdatedUserId = r.Context().Value("user").(*model.User).Id
	err = db.SaveVolume(volume, []string{"IsDeleted", "UpdatedAt", "UpdatedUserId"})
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
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
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid volume ID"})
		return
	}

	db := database.GetInstance()
	volume, err := db.GetVolume(volumeId)
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	data := apiclient.VolumeDefinition{
		Name:       volume.Name,
		Definition: volume.Definition,
		Active:     volume.Active,
		Zone:       volume.Zone,
		Platform:   volume.Platform,
	}

	rest.WriteResponse(http.StatusOK, w, r, &data)
}

func HandleVolumeStart(w http.ResponseWriter, r *http.Request) {
	var volume *model.Volume
	var err error

	db := database.GetInstance()

	volumeId := r.PathValue("volume_id")

	if !validate.UUID(volumeId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid volume ID"})
		return
	}

	transport := service.GetTransport()
	unlockToken := transport.LockResource(volumeId)
	if unlockToken == "" {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: "Failed to lock volume"})
		return
	}
	defer transport.UnlockResource(volumeId, unlockToken)

	volume, err = db.GetVolume(volumeId)
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// If the volume is already running then fail
	if volume.Active {
		rest.WriteResponse(http.StatusLocked, w, r, ErrorResponse{Error: "volume is running"})
		return
	}

	// If the volume has a zone and it is not this server then fail
	cfg := config.GetServerConfig()
	if volume.Zone != "" && volume.Zone != cfg.Zone {
		rest.WriteResponse(http.StatusLocked, w, r, ErrorResponse{Error: "volume is used by another server"})
		return
	}

	err = service.GetContainerService().CreateVolume(volume)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	volume.Zone = cfg.Zone
	volume.Active = true
	volume.UpdatedAt = hlc.Now()
	volume.UpdatedUserId = r.Context().Value("user").(*model.User).Id
	db.SaveVolume(volume, []string{"Active", "Zone", "UpdatedAt", "UpdatedUserId"})
	service.GetTransport().GossipVolume(volume)

	rest.WriteResponse(http.StatusOK, w, r, &apiclient.StartVolumeResponse{
		Status: true,
		Zone:   volume.Zone,
	})
}

func HandleVolumeStop(w http.ResponseWriter, r *http.Request) {
	var volume *model.Volume
	var err error

	db := database.GetInstance()

	volumeId := r.PathValue("volume_id")
	if !validate.UUID(volumeId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid volume ID"})
		return
	}

	transport := service.GetTransport()
	unlockToken := transport.LockResource(volumeId)
	if unlockToken == "" {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: "Failed to lock volume"})
		return
	}
	defer transport.UnlockResource(volumeId, unlockToken)

	volume, err = db.GetVolume(volumeId)
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// If the volume is not running or not this server then fail
	cfg := config.GetServerConfig()
	if !volume.Active || volume.Zone != cfg.Zone {
		rest.WriteResponse(http.StatusLocked, w, r, ErrorResponse{Error: "volume not running"})
		return
	}

	err = service.GetContainerService().DeleteVolume(volume)
	if err != nil && !strings.Contains(err.Error(), "volume not found") {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	volume.Zone = ""
	volume.Active = false
	volume.UpdatedAt = hlc.Now()
	volume.UpdatedUserId = r.Context().Value("user").(*model.User).Id
	db.SaveVolume(volume, []string{"Active", "Zone", "UpdatedAt", "UpdatedUserId"})
	service.GetTransport().GossipVolume(volume)

	w.WriteHeader(http.StatusOK)
}
