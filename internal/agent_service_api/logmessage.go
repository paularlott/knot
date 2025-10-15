package agent_service_api

import (
	"net/http"
	"strings"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/util/rest"

	"github.com/paularlott/knot/internal/log"
)

type LogMessage struct {
	Service string `json:"service" msgpack:"service"`
	Level   string `json:"level" msgpack:"level"`
	Message string `json:"message" msgpack:"message"`
}

// Handler to accept native log messages.
func handleLogMessage(w http.ResponseWriter, r *http.Request) {

	// Decode the log message
	var logMessage LogMessage
	if err := rest.DecodeRequestBody(w, r, &logMessage); err != nil {
		log.WithError(err).Error("service_api: failed to decode log message:")
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{"error": "invalid log message"})
		return
	}

	// Convert the log level from a string to a byte code
	var level msg.LogLevel
	switch strings.ToLower(logMessage.Level) {
	case "debug":
		level = msg.LogLevelDebug

	case "info":
		level = msg.LogLevelInfo

	case "error":
		level = msg.LogLevelError

	default:
		log.Error("service_api: invalid log level:", "service_api", logMessage.Level)
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{"error": "invalid log level"})
		return
	}

	// Send the log message to the server
	agentClient.SendLogMessage(logMessage.Service, level, logMessage.Message)

	// Write 202 Accepted response
	w.WriteHeader(http.StatusAccepted)
}
