package agent_service_api

import (
	"net/http"

	"github.com/paularlott/knot/internal/agentapi/agent_client"
	"github.com/paularlott/knot/util/rest"

	"github.com/rs/zerolog/log"
)

type SpaceNote struct {
	Note string `json:"note" msgpack:"note"`
}

func handleNote(w http.ResponseWriter, r *http.Request) {
	// Decode the message
	var message SpaceNote
	if err := rest.BindJSON(w, r, &message); err != nil {
		log.Error().Msgf("service_api: failed to decode message: %v", err)
		rest.SendJSON(http.StatusBadRequest, w, r, map[string]string{"error": "invalid message"})
		return
	}

	// Send the message to the server
	agent_client.SendSpaceNote(message.Note)

	w.WriteHeader(http.StatusOK)
}
