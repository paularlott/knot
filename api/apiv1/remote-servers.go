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
	var remoteServer *model.RemoteServer

	cache := database.GetCacheInstance()

	request := apiclient.RegisterRemoteServerRequest{}
	err := rest.BindJSON(w, r, &request)
	if err != nil {
		rest.SendJSON(http.StatusBadRequest, w, ErrorResponse{Error: err.Error()})
		return
	}

	log.Debug().Msgf("remote server registering url %s", request.Url)

	// Load the current list of remote servers
	servers, err := cache.GetRemoteServers()
	if err != nil {
		log.Error().Msgf("error loading remote servers: %s", err)
		rest.SendJSON(http.StatusInternalServerError, w, ErrorResponse{Error: "error loading remote servers"})
		return
	}

	// Check if the server is already registered
	for _, server := range servers {
		if server.Url == request.Url {
			log.Debug().Msgf("remote server %s already registered", request.Url)
			remoteServer = server
			break
		}
	}

	// If the server is not already registered, create a new one
	if remoteServer == nil {
		remoteServer = model.NewRemoteServer(request.Url)
	}

	// Save or force update of server access time
	cache.SaveRemoteServer(remoteServer)

	response := apiclient.RegisterRemoteServerResponse{
		Status:   true,
		ServerId: remoteServer.Id,
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

	// Save to update the access time
	cache.SaveRemoteServer(server)

	rest.SendJSON(http.StatusOK, w, nil)
}
