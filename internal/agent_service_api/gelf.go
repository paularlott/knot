package agent_service_api

import (
	"net/http"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/util/rest"

	"github.com/paularlott/knot/internal/log"
)

type gelfMessage struct {
	Version      string  `json:"version"`
	Host         string  `json:"host"`
	ShortMessage string  `json:"short_message"`
	FullMessage  string  `json:"full_message"`
	Timestamp    float64 `json:"timestamp"`
	Level        int     `json:"level"`
	Facility     string  `json:"facility"`
	Line         int     `json:"line"`
	File         string  `json:"file"`
}

// Simple handler to accept GELF messages.
// The log level is converted from GELF to the internal log level.
// The message is then sent to the agent server.
// No validation is done on the message to ensure it is a valid GELF message.
func handleGelf(w http.ResponseWriter, r *http.Request) {

	// Decode the log message
	var logMessage gelfMessage
	if err := rest.DecodeRequestBody(w, r, &logMessage); err != nil {
		log.WithError(err).Error("service_api: failed to decode log message:")
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{"error": "invalid log message"})
		return
	}

	// Convert the log level from a string to a byte code
	var level msg.LogLevel
	if logMessage.Level >= 0 && logMessage.Level <= 4 {
		level = msg.LogLevelError
	} else if logMessage.Level >= 5 && logMessage.Level <= 6 {
		level = msg.LogLevelInfo
	} else if logMessage.Level == 7 {
		level = msg.LogLevelDebug
	} else {
		log.Error("service_api: invalid log level:", "service_api", logMessage.Level)
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{"error": "invalid log level"})
		return
	}

	// Pick the short message if the full message is empty
	var message string = logMessage.ShortMessage
	if logMessage.FullMessage != "" {
		message = message + "\n\n" + logMessage.FullMessage
	}

	// Use the facility as the service name if it's present
	service := logMessage.Facility
	if service == "" {
		service = "gelf"
	}

	// Send the log message to the server
	agentClient.SendLogMessage(service, level, message)

	// Write 202 Accepted response
	w.WriteHeader(http.StatusAccepted)
}
