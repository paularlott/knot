package agent_service_api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/log"
)

type eventRequest struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload"`
}

func handleEvent(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req eventRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.Type == "" {
		http.Error(w, "type is required", http.StatusBadRequest)
		return
	}

	eventId, err := uuid.NewV7()
	if err != nil {
		http.Error(w, "failed to generate event id", http.StatusInternalServerError)
		return
	}

	payloadBytes, _ := json.Marshal(req.Payload)

	event := &msg.Event{
		EventId:   eventId.String(),
		EventType: req.Type,
		Payload:   payloadBytes,
	}

	if agentClient == nil {
		log.Warn("agent client not initialized, event dropped")
		http.Error(w, "agent client not available", http.StatusServiceUnavailable)
		return
	}

	if err := agentClient.ReportEvent(event); err != nil {
		log.Error("failed to deliver event to any server", "error", err)
		http.Error(w, "failed to deliver event: "+err.Error(), http.StatusBadGateway)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}
