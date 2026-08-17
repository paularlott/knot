package agent_service_api

import (
	"fmt"
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

// gelfMessageFromMap reads the standard GELF fields out of a decoded map.
func gelfMessageFromMap(rec map[string]any) gelfMessage {
	get := func(key string) string {
		if v, ok := rec[key]; ok {
			return fmt.Sprintf("%v", v)
		}
		return ""
	}
	var level int
	if v, ok := rec["level"].(float64); ok {
		level = int(v)
	}
	var timestamp float64
	if v, ok := rec["timestamp"].(float64); ok {
		timestamp = v
	}
	var line int
	if v, ok := rec["line"].(float64); ok {
		line = int(v)
	}
	return gelfMessage{
		Version:      get("version"),
		Host:         get("host"),
		ShortMessage: get("short_message"),
		FullMessage:  get("full_message"),
		Timestamp:    timestamp,
		Level:        level,
		Facility:     get("facility"),
		Line:         line,
		File:         get("file"),
	}
}

// gelfAdditionalFields collects GELF additional fields — keys prefixed with
// an underscore — as structured data, dropping the prefix.
func gelfAdditionalFields(rec map[string]any) map[string]string {
	var fields map[string]string
	for k, v := range rec {
		if len(k) > 1 && k[0] == '_' {
			if fields == nil {
				fields = make(map[string]string)
			}
			fields[k[1:]] = fmt.Sprintf("%v", v)
		}
	}
	return fields
}

// Simple handler to accept GELF messages.
// The log level is converted from GELF to the internal log level.
// The message is then sent to the agent server.
// No validation is done on the message to ensure it is a valid GELF message.
func handleGelf(w http.ResponseWriter, r *http.Request) {

	// Decode the log message as a map so arbitrary GELF additional fields
	// (the _-prefixed kind) survive.
	var rec map[string]any
	if err := rest.DecodeRequestBody(w, r, &rec); err != nil {
		log.WithError(err).Error("failed to decode log message:")
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{"error": "invalid log message"})
		return
	}
	logMessage := gelfMessageFromMap(rec)

	// Convert the log level from a string to a byte code
	var level msg.LogLevel
	if logMessage.Level >= 0 && logMessage.Level <= 4 {
		level = msg.LogLevelError
	} else if logMessage.Level >= 5 && logMessage.Level <= 6 {
		level = msg.LogLevelInfo
	} else if logMessage.Level == 7 {
		level = msg.LogLevelDebug
	} else {
		log.Error("invalid log level:", "service_api", logMessage.Level)
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

	// Send the log message to the server, carrying _-prefixed additional
	// fields as structured data.
	sendLogMessage(service, level, message, gelfAdditionalFields(rec))

	// Write 202 Accepted response
	w.WriteHeader(http.StatusAccepted)
}
