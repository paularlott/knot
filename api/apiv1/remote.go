package apiv1

import (
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/util/rest"
	"github.com/paularlott/knot/util/validate"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func HandleRemoteGetTemplateVars(w http.ResponseWriter, r *http.Request) {
	templateVars, err := database.GetInstance().GetTemplateVars()
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
		return
	}

	// Build a json array of data to return to the client
	data := make([]apiclient.TemplateVarValues, len(templateVars))

	for i, variable := range templateVars {
		data[i].Name = variable.Name
		data[i].Value = variable.Value
	}

	rest.SendJSON(http.StatusOK, w, data)
}

func HandleUpdateVolumeRemote(w http.ResponseWriter, r *http.Request) {
	volemeId := chi.URLParam(r, "volume_id")

	request := apiclient.VolumeDefinition{}
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

	db := database.GetInstance()

	volume, err := database.GetInstance().GetVolume(volemeId)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
		return
	}

	volume.Name = request.Name
	volume.Definition = request.Definition
	volume.Location = request.Location
	volume.Active = request.Active

	err = db.SaveVolume(volume)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
}

func HandleGetTemplateHashes(w http.ResponseWriter, r *http.Request) {
	templates, err := database.GetInstance().GetTemplates()
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: err.Error()})
		return
	}

	data := make(map[string]string, len(templates))
	for _, template := range templates {
		data[template.Id] = template.Hash
	}

	rest.SendJSON(http.StatusOK, w, data)
}

func HandleNotifyUserUpdate(w http.ResponseWriter, r *http.Request) {
	userId := chi.URLParam(r, "user_id")

	// Fetch the user within a go routine
	go func() {
		client := apiclient.NewRemoteToken(viper.GetString("server.remote_token"))

		user, err := client.RemoteGetUser(userId)
		if err != nil || user == nil {
			log.Error().Msgf("notify: error fetching user %s: %s", userId, err)
		} else {
			log.Debug().Msgf("notify: fetched user %s", user.Username)
			database.GetInstance().SaveUser(user)

			UpdateUserSpaces(user)
		}
	}()

	w.WriteHeader(http.StatusOK)
}

func HandleNotifyUserDelete(w http.ResponseWriter, r *http.Request) {
	userId := chi.URLParam(r, "user_id")
	db := database.GetInstance()

	// Load the user from the local database
	user, err := db.GetUser(userId)
	if err != nil && err.Error() != "user not found" {
		log.Debug().Msgf("notify: delete user %s: %s", userId, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// User loaded so delete it
	if user != nil {
		go DeleteUser(db, user)
	}

	w.WriteHeader(http.StatusOK)
}
