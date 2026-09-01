package agent_service_api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/util/rest"

	"github.com/paularlott/knot/internal/log"
)

// Handler to accept VictoriaLogs jsonline insert requests, e.g.
// POST /insert/jsonline?_msg_field=_msg&_time_field=_time&_stream_fields=service
//
// Each body line is a self-contained JSON object. The field names for the
// message and stream are taken from the query params (matching VictoriaLogs
// semantics) with the same defaults, so shippers configured for VictoriaLogs
// work unchanged when pointed at the agent.
//
// The service name is taken from a "service" field if present, else from the
// first stream field, else it is set to "victorialogs". The log level is taken
// from a "level" field if present, else it is set to info.
func handleVictoriaLogs(w http.ResponseWriter, r *http.Request) {

	// Resolve the field names from the query params, falling back to the
	// VictoriaLogs defaults.
	msgField := r.URL.Query().Get("_msg_field")
	if msgField == "" {
		msgField = "_msg"
	}
	var streamFields []string
	if sf := r.URL.Query().Get("_stream_fields"); sf != "" {
		for _, f := range strings.Split(sf, ",") {
			if f = strings.TrimSpace(f); f != "" {
				streamFields = append(streamFields, f)
			}
		}
	}

	scanner := bufio.NewScanner(r.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}

		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			log.WithError(err).Error("failed to decode victoria logs jsonline:")
			rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{"error": "invalid victoria logs jsonline"})
			return
		}

		// The message is whatever field the shipper designated as the msg
		// field; fall back to the raw line so nothing is silently dropped.
		var message string
		if m, ok := record[msgField].(string); ok {
			message = m
		} else if m, ok := record["_msg"].(string); ok {
			message = m
		} else if m, ok := record["msg"].(string); ok {
			message = m
		} else {
			message = string(line)
		}

		sendLogMessage(victoriaServiceName(record, streamFields), victoriaLogLevel(record), message, victoriaExtraFields(record, msgField, streamFields))
	}
	if err := scanner.Err(); err != nil {
		log.WithError(err).Error("failed to read victoria logs jsonline:")
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{"error": "invalid victoria logs jsonline"})
		return
	}

	// VictoriaLogs answers 204 on successful insert
	w.WriteHeader(http.StatusNoContent)
}

// victoriaExtraFields collects the structured fields that aren't mapped to
// message/service/level so they ride along to sinks and forwarding.
func victoriaExtraFields(record map[string]any, msgField string, streamFields []string) map[string]string {
	skip := map[string]bool{
		msgField:  true,
		"_msg":    true,
		"msg":     true,
		"_time":   true,
		"time":    true,
		"service": true,
		"level":   true,
	}
	for _, f := range streamFields {
		skip[f] = true
	}

	var fields map[string]string
	for k, v := range record {
		if skip[k] {
			continue
		}
		if fields == nil {
			fields = make(map[string]string)
		}
		fields[k] = fmt.Sprintf("%v", v)
	}
	return fields
}

func victoriaServiceName(record map[string]any, streamFields []string) string {
	if s, ok := record["service"].(string); ok && s != "" {
		return s
	}
	for _, f := range streamFields {
		if s, ok := record[f].(string); ok && s != "" {
			return s
		}
	}
	return "knot_syslog"
}

func victoriaLogLevel(record map[string]any) msg.LogLevel {
	switch strings.ToLower(stringOrNumber(record["level"])) {
	case "debug":
		return msg.LogLevelDebug
	case "error", "fatal":
		return msg.LogLevelError
	default:
		return msg.LogLevelInfo
	}
}

// stringOrNumber maps numeric syslog severities (0 emergency … 7 debug) to
// the level names shippers use, returning "" when the value isn't a
// recognised level. Warning (4) and notice (5) have no knot equivalent and
// round down to info.
func stringOrNumber(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		switch t {
		case 0, 1, 2, 3:
			return "error"
		case 4, 5, 6:
			return "info"
		case 7:
			return "debug"
		}
		return ""
	default:
		return ""
	}
}
