package apiv1

import (
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/util/rest"

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
