package apiv1

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util/rest"

	"github.com/rs/zerolog/log"
)

func HandleRegisterRemoteServer(w http.ResponseWriter, r *http.Request) {
	cache := database.GetCacheInstance()

	request := apiclient.RegisterRemoteServerRequest{}
	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
		return
	}

	log.Debug().Msgf("remote server registering url %s", request.Url)

	server := model.NewRemoteServer(request.Url)
	cache.SaveRemoteServer(server)

	response := apiclient.RegisterRemoteServerResponse{
		Status:   true,
		ServerId: server.Id,
	}
	rest.SendJSON(http.StatusCreated, w, &response)
}

func HandleUpdateRemoteServer(w http.ResponseWriter, r *http.Request) {
	cache := database.GetCacheInstance()

	// Load and save the server to update it
	server, err := cache.GetRemoteServer(chi.URLParam(r, "server_id"))
	if err != nil {
		log.Debug().Msgf("remote server %s not found", chi.URLParam(r, "server_id"))
		rest.SendJSON(http.StatusNotFound, w, ErrorResponse{Error: "remote server not found"})
		return
	}

	cache.SaveRemoteServer(server)

	rest.SendJSON(http.StatusOK, w, nil)
}
