package apiv1

import (
	"net/http"
	"strings"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/container"
	"github.com/paularlott/knot/internal/container/docker"
	"github.com/paularlott/knot/internal/container/nomad"
	"github.com/paularlott/knot/internal/origin_leaf/origin"
	"github.com/paularlott/knot/internal/origin_leaf/server_info"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"

	"github.com/go-chi/chi/v5"
)

func HandleGetVolumes(w http.ResponseWriter, r *http.Request) {
	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil && !server_info.RestrictedLeaf {
		client := remoteClient.(*apiclient.ApiClient)

		volumes, code, err := client.GetVolumes()
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		rest.SendJSON(http.StatusOK, w, r, volumes)
	} else {
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
}

func HandleUpdateVolume(w http.ResponseWriter, r *http.Request) {
	volemeId := chi.URLParam(r, "volume_id")

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

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil && !server_info.RestrictedLeaf {
		client := remoteClient.(*apiclient.ApiClient)

		code, err := client.UpdateVolume(volemeId, request.Name, request.Definition)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		db := database.GetInstance()
		user := r.Context().Value("user").(*model.User)

		volume, err := database.GetInstance().GetVolume(volemeId)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		volume.Name = request.Name
		volume.Definition = request.Definition
		volume.UpdatedUserId = user.Id

		err = db.SaveVolume(volume)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	}

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

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil && !server_info.RestrictedLeaf {
		client := remoteClient.(*apiclient.ApiClient)

		response, code, err := client.CreateVolume(request.Name, request.Definition, request.LocalContainer)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		rest.SendJSON(http.StatusCreated, w, r, response)
	} else {
		db := database.GetInstance()
		user := r.Context().Value("user").(*model.User)

		volume := model.NewVolume(request.Name, request.Definition, user.Id, request.LocalContainer)

		err = db.SaveVolume(volume)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		// Return the ID
		rest.SendJSON(http.StatusCreated, w, r, &apiclient.VolumeCreateResponse{
			Status:   true,
			VolumeId: volume.Id,
		})
	}
}

func HandleDeleteVolume(w http.ResponseWriter, r *http.Request) {
	volumeId := chi.URLParam(r, "volume_id")

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil && !server_info.RestrictedLeaf {
		client := remoteClient.(*apiclient.ApiClient)

		code, err := client.DeleteVolume(volumeId)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
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
		err = database.GetInstance().DeleteVolume(volume)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func HandleGetVolume(w http.ResponseWriter, r *http.Request) {
	volumeId := chi.URLParam(r, "volume_id")

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil && !server_info.RestrictedLeaf {
		client := remoteClient.(*apiclient.ApiClient)

		volume, code, err := client.GetVolume(volumeId)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		rest.SendJSON(http.StatusOK, w, r, volume)
	} else {
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
}

func HandleVolumeStart(w http.ResponseWriter, r *http.Request) {
	var client *apiclient.ApiClient = nil
	var volume *model.Volume
	var err error
	var code int

	db := database.GetInstance()

	volumeId := chi.URLParam(r, "volume_id")

	// If remote client present then fetch the volume information from the remote
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil && !server_info.RestrictedLeaf {
		client = remoteClient.(*apiclient.ApiClient)

		volume, code, err = client.GetVolumeObject(volumeId)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		volume, err = db.GetVolume(volumeId)
		if err != nil {
			rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	}

	// If the volume is already running then fail
	if volume.Active {
		rest.SendJSON(http.StatusLocked, w, r, ErrorResponse{Error: "volume is running"})
		return
	}

	// If the volume has a location and it is not this server then fail
	if volume.Location != "" && volume.Location != server_info.LeafLocation {
		rest.SendJSON(http.StatusLocked, w, r, ErrorResponse{Error: "volume is used by another server"})
		return
	}

	// Add the variables
	variables, err := db.GetTemplateVars()
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	vars := model.FilterVars(variables)

	// Mark volume as started
	volume.Location = server_info.LeafLocation
	volume.Active = true

	var containerClient container.ContainerManager
	if volume.LocalContainer {
		containerClient = docker.NewClient()
	} else {
		containerClient = nomad.NewClient()
	}

	// Create volumes
	err = containerClient.CreateVolume(volume, &vars)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if client != nil {
		// Tell remote volume started
		origin.UpdateVolume(volume)
	} else {
		db.SaveVolume(volume)
	}

	rest.SendJSON(http.StatusOK, w, r, &apiclient.StartVolumeResponse{
		Status:   true,
		Location: volume.Location,
	})
}

func HandleVolumeStop(w http.ResponseWriter, r *http.Request) {
	var client *apiclient.ApiClient = nil
	var volume *model.Volume
	var err error
	var code int

	db := database.GetInstance()

	volumeId := chi.URLParam(r, "volume_id")

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil && !server_info.RestrictedLeaf {
		client = remoteClient.(*apiclient.ApiClient)

		volume, code, err = client.GetVolumeObject(volumeId)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		volume, err = db.GetVolume(volumeId)
		if err != nil {
			rest.SendJSON(http.StatusNotFound, w, r, ErrorResponse{Error: err.Error()})
			return
		}
	}

	// If the volume is not running or not this server then fail
	if !volume.Active || volume.Location != server_info.LeafLocation {
		rest.SendJSON(http.StatusLocked, w, r, ErrorResponse{Error: "volume not running"})
		return
	}

	// Add the variables
	variables, err := db.GetTemplateVars()
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	vars := model.FilterVars(variables)

	// Record the volume as not deployed
	volume.Location = ""
	volume.Active = false

	var containerClient container.ContainerManager
	if volume.LocalContainer {
		containerClient = docker.NewClient()
	} else {
		containerClient = nomad.NewClient()
	}

	// Delete the volume
	err = containerClient.DeleteVolume(volume, &vars)
	if err != nil && !strings.Contains(err.Error(), "volume not found") {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if client != nil {
		// Tell remote volume stopped
		origin.UpdateVolume(volume)
	} else {
		db.SaveVolume(volume)
	}

	w.WriteHeader(http.StatusOK)
}
