package agent_service_api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/util/rest"

	"github.com/paularlott/knot/internal/log"
)

type lokiPushRequest struct {
	Streams []stream `json:"streams"`
}

type stream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

// lokiLine extracts structured data from a Loki log line: when the line is
// a JSON object with a msg/message field the remaining keys are carried as
// structured fields; otherwise the line is the message verbatim.
func lokiLine(line string) (string, map[string]string) {
	var rec map[string]any
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		return line, nil
	}

	msgText := ""
	if m, ok := rec["msg"].(string); ok {
		msgText = m
	} else if m, ok := rec["message"].(string); ok {
		msgText = m
	} else {
		return line, nil
	}

	var fields map[string]string
	for k, v := range rec {
		if k == "msg" || k == "message" {
			continue
		}
		if fields == nil {
			fields = make(map[string]string)
		}
		fields[k] = fmt.Sprintf("%v", v)
	}
	return msgText, fields
}

// Simple handler to accept Loki push requests.
// The service name is taken from the stream label if present else it is set to "loki".
// The log level is always set to info.
// The message is then sent to the agent server.
// No validation is done on the message to ensure it is a valid Loki push request.
func handleLoki(w http.ResponseWriter, r *http.Request) {

	// Decode the loki push request
	var lokiPushRequest lokiPushRequest
	if err := rest.DecodeRequestBody(w, r, &lokiPushRequest); err != nil {
		log.WithError(err).Error("failed to decode loki push request:")
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{"error": "invalid loki push request"})
		return
	}

	// Process each stream
	for _, stream := range lokiPushRequest.Streams {
		// Get the service name from a conventional label if present else
		// use "loki": knot's documented "label", or the standard
		// "service" / "job" label names most shippers set.
		service := "loki"
		for _, label := range []string{"label", "service", "job"} {
			if val, ok := stream.Stream[label]; ok && val != "" {
				service = val
				break
			}
		}

		// Process each log message
		for _, values := range stream.Values {
			message, fields := lokiLine(values[1])
			sendLogMessage(service, msg.LogLevelInfo, message, fields)
		}
	}

	// Write 204 Accepted response
	w.WriteHeader(http.StatusNoContent)
}
