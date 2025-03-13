package agent_service_api

import (
	"net/http"

	"github.com/paularlott/knot/internal/agentapi/agent_client"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/util/rest"

	"github.com/rs/zerolog/log"
)

type lokiPushRequest struct {
	Streams []stream `json:"streams"`
}

type stream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

// Simple handler to accept Loki push requests.
// The service name is taken from the stream label if present else it is set to "loki".
// The log level is always set to info.
// The message is then sent to the agent server.
// No validation is done on the message to ensure it is a valid Loki push request.
func handleLoki(w http.ResponseWriter, r *http.Request) {

	// Decode the loki push request
	var lokiPushRequest lokiPushRequest
	if err := rest.BindJSON(w, r, &lokiPushRequest); err != nil {
		log.Error().Msgf("service_api: failed to decode loki push request: %v", err)
		rest.SendJSON(http.StatusBadRequest, w, r, map[string]string{"error": "invalid loki push request"})
		return
	}

	// Process each stream
	for _, stream := range lokiPushRequest.Streams {
		// Get the service name from the stream label if present else use "loki"
		service := "loki"
		if val, ok := stream.Stream["label"]; ok {
			service = val
		}

		// Process each log message
		for _, values := range stream.Values {
			agent_client.SendLogMessage(service, msg.LogLevelInfo, values[1])
		}
	}

	// Write 204 Accepted response
	w.WriteHeader(http.StatusNoContent)
}
