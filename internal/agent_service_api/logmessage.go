package agent_service_api

import (
	"fmt"
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
	var rec map[string]any
	if err := rest.DecodeRequestBody(w, r, &rec); err != nil {
		log.WithError(err).Error("failed to decode log message:")
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{"error": "invalid log message"})
		return
	}

	// Convert the log level from a string to a byte code
	var level msg.LogLevel
	switch strings.ToLower(mapStr(rec, "level")) {
	case "debug":
		level = msg.LogLevelDebug

	case "info":
		level = msg.LogLevelInfo

	case "error":
		level = msg.LogLevelError

	default:
		log.Error("invalid log level:", "service_api", mapStr(rec, "level"))
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{"error": "invalid log level"})
		return
	}

	// Send the log message to the server
	sendLogMessage(mapStr(rec, "service"), level, mapStr(rec, "message"), nativeExtraFields(rec))

	// Write 202 Accepted response
	w.WriteHeader(http.StatusAccepted)
}

// mapStr reads a string-ish value from a decoded log record.
func mapStr(rec map[string]any, key string) string {
	if v, ok := rec[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// nativeExtraFields collects keys beyond the fixed service/level/message
// schema as structured fields; older senders sending only the three keys
// get nil.
func nativeExtraFields(rec map[string]any) map[string]string {
	var fields map[string]string
	for k, v := range rec {
		if k == "service" || k == "level" || k == "message" {
			continue
		}
		if fields == nil {
			fields = make(map[string]string)
		}
		fields[k] = fmt.Sprintf("%v", v)
	}
	return fields
}
