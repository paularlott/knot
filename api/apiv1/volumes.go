package apiv1

import (
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/nomad"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/viper"
)

func HandleGetVolumes(w http.ResponseWriter, r *http.Request) {
	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		volumes, code, err := client.GetVolumes()
		if err != nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}

		rest.SendJSON(http.StatusOK, w, volumes)
	} else {
		volumes, err := database.GetInstance().GetVolumes()
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}

		// Build a json array of data to return to the client
		volumeData := make([]apiclient.VolumeInfo, len(volumes))

		for i, volume := range volumes {
			volumeData[i].Id = volume.Id
			volumeData[i].Name = volume.Name
			volumeData[i].Active = volume.Active
			volumeData[i].Location = volume.Location
		}

		rest.SendJSON(http.StatusOK, w, volumeData)
	}
}

func HandleUpdateVolume(w http.ResponseWriter, r *http.Request) {
	volemeId := chi.URLParam(r, "volume_id")

	request := apiclient.UpdateVolumeRequest{}
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

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		code, err := client.UpdateVolume(volemeId, request.Name, request.Definition)
		if err != nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		db := database.GetInstance()
		user := r.Context().Value("user").(*model.User)

		volume, err := database.GetInstance().GetVolume(volemeId)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
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
	}

	w.WriteHeader(http.StatusOK)
}

func HandleCreateVolume(w http.ResponseWriter, r *http.Request) {
	request := apiclient.CreateVolumeRequest{}
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

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		response, code, err := client.CreateVolume(request.Name, request.Definition)
		if err != nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}

		rest.SendJSON(http.StatusCreated, w, response)
	} else {
		db := database.GetInstance()
		user := r.Context().Value("user").(*model.User)

		volume := model.NewVolume(request.Name, request.Definition, user.Id)

		err = db.SaveVolume(volume)
		if err != nil {
			rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
			return
		}

		// Return the ID
		rest.SendJSON(http.StatusCreated, w, &apiclient.VolumeCreateResponse{
			Status:   true,
			VolumeId: volume.Id,
		})
	}
}

func HandleDeleteVolume(w http.ResponseWriter, r *http.Request) {
	volumeId := chi.URLParam(r, "volume_id")

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		code, err := client.DeleteVolume(volumeId)
		if err != nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		db := database.GetInstance()

		volume, err := db.GetVolume(volumeId)
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
	}

	w.WriteHeader(http.StatusOK)
}

func HandleGetVolume(w http.ResponseWriter, r *http.Request) {
	volumeId := chi.URLParam(r, "volume_id")

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		volume, code, err := client.GetVolume(volumeId)
		if err != nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}

		rest.SendJSON(http.StatusOK, w, volume)
	} else {
		db := database.GetInstance()
		volume, err := db.GetVolume(volumeId)
		if err != nil {
			rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
			return
		}

		data := apiclient.VolumeDefinition{
			Name:       volume.Name,
			Definition: volume.Definition,
			Active:     volume.Active,
		}

		rest.SendJSON(http.StatusOK, w, &data)
	}
}

func HandleVolumeStart(w http.ResponseWriter, r *http.Request) {
	var volume *model.Volume
	var vars map[string]interface{}
	var err error

	volumeId := chi.URLParam(r, "volume_id")

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		var code int

		volume, vars, code, err = client.StartVolumeRemote(volumeId, viper.GetString("server.location"))
		if err != nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		db := database.GetInstance()

		volume, err = db.GetVolume(volumeId)
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

		// Mark volume as started
		volume.Location = viper.GetString("server.location")
		volume.Active = true
		db.SaveVolume(volume)
	}

	// Get the nomad client
	nomadClient := nomad.NewClient()

	// Create volumes
	err = nomadClient.CreateVolume(volume, &vars)
	if err != nil {

		// If remote then tell remote volume stopped
		remoteClient := r.Context().Value("remote_client")
		if remoteClient != nil {
			client := remoteClient.(*apiclient.ApiClient)
			client.StopVolumeRemote(volumeId, viper.GetString("server.location"))
		}

		rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
		return
	}

	rest.SendJSON(http.StatusOK, w, &apiclient.StartVolumeResponse{
		Status:   true,
		Location: volume.Location,
	})
}

func HandleVolumeStop(w http.ResponseWriter, r *http.Request) {
	var volume *model.Volume
	var vars map[string]interface{}
	var err error

	volumeId := chi.URLParam(r, "volume_id")

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		var code int

		volume, vars, code, err = client.StopVolumeRemote(volumeId, viper.GetString("server.location"))
		if err != nil {
			rest.SendJSON(code, w, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		db := database.GetInstance()

		volume, err = db.GetVolume(volumeId)
		if err != nil {
			rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
			return
		}

		// If the volume is not running or not this server then fail
		if !volume.Active || volume.Location != viper.GetString("server.location") {
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

		// Record the space as not deployed
		volume.Location = ""
		volume.Active = false
		db.SaveVolume(volume)
	}

	// Get the nomad client
	nomadClient := nomad.NewClient()

	// Create volumes
	err = nomadClient.DeleteVolume(volume, &vars)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
}

func HandleVolumeStartRemote(w http.ResponseWriter, r *http.Request) {
	volumeId := chi.URLParam(r, "volume_id")

	request := apiclient.VolumeStartRequest{}
	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.Required(request.Location) || !validate.MaxLength(request.Location, 64) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid location given"})
		return
	}

	db := database.GetInstance()

	volume, err := db.GetVolume(volumeId)
	if err != nil {
		rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
		return
	}

	// If the volume is already running then fail
	if volume.Active {
		rest.SendJSON(http.StatusLocked, w, ErrorResponse{Error: "volume is running"})
		return
	}

	// Create the response
	response := &apiclient.VolumeStartResponse{
		Name:       volume.Name,
		Definition: volume.Definition,
		Location:   request.Location,
		Variables:  make(map[string]interface{}),
	}

	// Add the variables
	variables, err := db.GetTemplateVars()
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
		return
	}

	for _, variable := range variables {
		response.Variables[variable.Name] = variable.Value
	}

	// Mark volume as started
	volume.Location = request.Location
	volume.Active = true
	db.SaveVolume(volume)

	rest.SendJSON(http.StatusOK, w, response)
}

func HandleVolumeStopRemote(w http.ResponseWriter, r *http.Request) {
	volumeId := chi.URLParam(r, "volume_id")

	request := apiclient.VolumeStopRequest{}
	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.Required(request.Location) || !validate.MaxLength(request.Location, 64) {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: "Invalid location given"})
		return
	}

	db := database.GetInstance()

	volume, err := db.GetVolume(volumeId)
	if err != nil {
		rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: err.Error()})
		return
	}

	// If the volume is not running then fail
	if !volume.Active {
		rest.SendJSON(http.StatusLocked, w, ErrorResponse{Error: "volume is running"})
		return
	}

	// Create the response
	response := &apiclient.VolumeStopResponse{
		Name:       volume.Name,
		Definition: volume.Definition,
		Location:   request.Location,
		Variables:  make(map[string]interface{}),
	}

	// Add the variables
	variables, err := db.GetTemplateVars()
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
		return
	}

	for _, variable := range variables {
		response.Variables[variable.Name] = variable.Value
	}

	// Mark volume as stopped
	volume.Location = ""
	volume.Active = false
	db.SaveVolume(volume)

	rest.SendJSON(http.StatusOK, w, response)
}
