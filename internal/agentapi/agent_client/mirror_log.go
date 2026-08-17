package agent_client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/paularlott/knot/internal/agentapi/msg"

	"github.com/paularlott/knot/internal/log"
)

// mirrorTarget builds the local log service base URL for a sink port.
// A var so tests can point it at a stub server.
var mirrorTarget = func(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

var mirrorHTTPClient = &http.Client{Timeout: 10 * time.Second}

// mirrorPostAttempts is the backoff between delivery attempts for a mirrored
// batch — retry yes, disk buffering no: if the local service stays down the
// batch is dropped with a warning.
var mirrorPostAttempts = []time.Duration{500 * time.Millisecond, 1 * time.Second, 2 * time.Second}

// handleMirrorLog receives a batch of log records mirrored from the knot
// server and writes it to the local log service on the advertised port, in
// the advertised format (vl jsonline, Loki push, GELF or the native format).
func handleMirrorLog(agentClient *AgentClient, batch *msg.MirrorLogMessage) {
	port := agentClient.GetLogSinkPort()
	if port == 0 || len(batch.Entries) == 0 {
		return
	}
	base := mirrorTarget(port)

	auth := sinkAuth{
		token:    agentClient.logSinkToken,
		username: agentClient.logSinkUsername,
		password: agentClient.logSinkPassword,
	}

	var err error
	switch agentClient.GetLogSinkFormat() {
	case "loki":
		err = postMirror(base+"/loki/api/v1/push", "application/json", encodeMirrorLoki(batch), auth)
	case "gelf":
		err = postMirrorGelf(base+"/gelf", batch, auth)
	default: // vl and json both speak the agent's own ingest dialects
		if agentClient.GetLogSinkFormat() == "json" {
			err = postMirrorJSON(base+"/logs", batch, auth)
		} else {
			// service and level are declared as stream fields so sink-side
			// VictoriaLogs indexes them as streams.
			err = postMirror(base+"/insert/jsonline?_stream_fields=service,level", "application/stream+json", encodeMirrorVL(batch), auth)
		}
	}
	if err != nil {
		log.WithError(err).Warn("log sink delivery to local service failed", "port", port, "entries", len(batch.Entries))
	}
}

// sinkAuth carries the optional credentials for the local log service, from
// KNOT_LOG_SINK_TOKEN (bearer) or KNOT_LOG_SINK_USERNAME / KNOT_LOG_SINK_PASSWORD
// (basic). A token takes precedence over basic auth, matching the server's
// log.output semantics.
type sinkAuth struct {
	token    string
	username string
	password string
}

func (a sinkAuth) apply(req *http.Request) {
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	} else if a.username != "" {
		req.SetBasicAuth(a.username, a.password)
	}
}

// postMirror delivers a payload with retries on transport errors and 5xx.
func postMirror(url, contentType string, body []byte, auth sinkAuth) error {
	if body == nil {
		return nil
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", contentType)
		auth.apply(req)

		resp, err := mirrorHTTPClient.Do(req)
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
			if resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
				return fmt.Errorf("log service rejected payload: %s", resp.Status)
			}
			err = fmt.Errorf("log service returned %s", resp.Status)
		}
		if attempt >= len(mirrorPostAttempts) {
			return err
		}
		time.Sleep(mirrorPostAttempts[attempt])
	}
}

// encodeMirrorVL renders a batch as VictoriaLogs jsonline NDJSON — each line
// a self-contained record tagged with the source space.
func encodeMirrorVL(batch *msg.MirrorLogMessage) []byte {
	var buf bytes.Buffer
	for _, e := range batch.Entries {
		rec := map[string]any{
			"_msg":  e.Message,
			"_time": e.Date.UTC().Format(time.RFC3339Nano),
			// Audit event payload style: actor + properties.space_id /
			// properties.space_name.
			"actor": e.User,
			"properties": map[string]any{
				"space_id":   e.SpaceId,
				"space_name": e.SpaceName,
			},
			"service": e.Service,
			"level":   mirrorLevelName(e.Level),
		}
		mergeFields(rec, e.Fields)
		b, _ := json.Marshal(rec)
		buf.Write(b)
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

// encodeMirrorLoki renders a batch as a Loki push payload, one stream per
// source space and service: a stream's labels must be constant for every
// entry in it, so entries from the same space but different services get
// their own stream instead of overwriting each other's service label.
func encodeMirrorLoki(batch *msg.MirrorLogMessage) []byte {
	type lokiStream struct {
		Stream map[string]string `json:"stream"`
		Values [][2]string       `json:"values"`
	}
	type lokiPayload struct {
		Streams []lokiStream `json:"streams"`
	}

	streams := map[string]*lokiStream{}
	for _, e := range batch.Entries {
		key := e.SpaceId + "\x00" + e.Service
		s, ok := streams[key]
		if !ok {
			s = &lokiStream{Stream: map[string]string{"space": e.SpaceId, "space_name": e.SpaceName, "user": e.User, "service": e.Service}}
			streams[key] = s
		}
		lineRec := map[string]any{"msg": e.Message, "level": mirrorLevelName(e.Level)}
		mergeFields(lineRec, e.Fields)
		line, _ := json.Marshal(lineRec)
		s.Values = append(s.Values, [2]string{fmt.Sprintf("%d", e.Date.UnixNano()), string(line)})
	}

	payload := lokiPayload{Streams: make([]lokiStream, 0, len(streams))}
	for _, s := range streams {
		payload.Streams = append(payload.Streams, *s)
	}
	b, _ := json.Marshal(payload)
	return b
}

// postMirrorGelf sends each entry as its own GELF message — the protocol has
// no batch form the agent's own /gelf endpoint accepts.
func postMirrorGelf(url string, batch *msg.MirrorLogMessage, auth sinkAuth) error {
	for _, e := range batch.Entries {
		gelf := map[string]any{
			"version":       "1.1",
			"short_message": e.Message,
			"timestamp":     float64(e.Date.UnixNano()) / 1e9,
			"host":          e.SpaceId,
			"_space_name":   e.SpaceName,
			"_user":         e.User,
			"facility":      e.Service,
			"level":         mirrorGelfLevel(e.Level),
		}
		for k, v := range e.Fields {
			gelf["_"+k] = v
		}
		body, _ := json.Marshal(gelf)
		if err := postMirror(url, "application/json", body, auth); err != nil {
			return err
		}
	}
	return nil
}

// postMirrorJSON sends each entry to the native /logs endpoint. Keys beyond
// the endpoint's service/level/message schema are kept as structured fields
// by the receiving side, so the space/user context and any entry fields ride
// along instead of being dropped.
func postMirrorJSON(url string, batch *msg.MirrorLogMessage, auth sinkAuth) error {
	for _, e := range batch.Entries {
		rec := map[string]string{
			"service":    e.Service,
			"level":      mirrorLevelName(e.Level),
			"message":    e.Message,
			"space_id":   e.SpaceId,
			"space_name": e.SpaceName,
			"user":       e.User,
		}
		mergeFields(rec, e.Fields)
		body, _ := json.Marshal(rec)
		if err := postMirror(url, "application/json", body, auth); err != nil {
			return err
		}
	}
	return nil
}

// mergeFields copies entry fields into a rendered record without touching
// keys knot itself set: those carry the record's origin (space, user,
// service, level), and an app field sharing a name would misattribute the
// record at the sink — and on the VL path, silently re-route its stream. A
// colliding app field is dropped from the mirrored copy; the original entry
// inside knot still carries it.
func mergeFields[V any](rec map[string]V, fields map[string]string) {
	for k, v := range fields {
		if _, taken := rec[k]; taken {
			continue
		}
		rec[k] = any(v).(V)
	}
}

func mirrorLevelName(level msg.LogLevel) string {
	switch level {
	case msg.LogLevelDebug:
		return "debug"
	case msg.LogLevelError:
		return "error"
	default:
		return "info"
	}
}

func mirrorGelfLevel(level msg.LogLevel) int {
	switch level {
	case msg.LogLevelDebug:
		return 7
	case msg.LogLevelError:
		return 3
	default:
		return 6
	}
}
