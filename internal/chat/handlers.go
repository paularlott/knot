package chat

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/rest"
)

var chatService *Service

func SetChatService(service *Service) {
	chatService = service
}

func HandleChatStream(w http.ResponseWriter, r *http.Request) {
	if chatService == nil {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, map[string]string{
			"error": "Chat service not configured",
		})
		return
	}

	user := r.Context().Value("user").(*model.User)
	if user == nil {
		rest.WriteResponse(http.StatusUnauthorized, w, r, map[string]string{
			"error": "User not found",
		})
		return
	}

	// Parse request
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{
			"error": "Invalid request body",
		})
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create message history (in a real implementation, you might want to get this from the request)
	messages := []ChatMessage{
		{
			Role:      "user",
			Content:   req.Message,
			Timestamp: time.Now().Unix(),
		},
	}

	// Stream response
	err := chatService.StreamChat(r.Context(), messages, user, w)
	if err != nil {
		event := SSEEvent{
			Type: "error",
			Data: map[string]string{
				"error": err.Error(),
			},
		}
		data, _ := json.Marshal(event)
		w.Write([]byte("data: " + string(data) + "\n\n"))
	}

	// Send done event
	event := SSEEvent{
		Type: "done",
		Data: nil,
	}
	data, _ := json.Marshal(event)
	w.Write([]byte("data: " + string(data) + "\n\n"))
}

func HandleChatConfig(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	if user == nil {
		rest.WriteResponse(http.StatusUnauthorized, w, r, map[string]string{
			"error": "User not found",
		})
		return
	}

	// Only allow admin users to view/modify chat config
	if !user.HasPermission(model.PermissionManageUsers) {
		rest.WriteResponse(http.StatusForbidden, w, r, map[string]string{
			"error": "Insufficient permissions",
		})
		return
	}

	switch r.Method {
	case http.MethodGet:
		if chatService == nil {
			rest.WriteResponse(http.StatusOK, w, r, ChatConfig{})
			return
		}
		
		// Return config without sensitive data
		config := chatService.config
		config.OpenAIAPIKey = "" // Don't expose API key
		rest.WriteResponse(http.StatusOK, w, r, config)

	case http.MethodPost:
		var config ChatConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{
				"error": "Invalid request body",
			})
			return
		}

		// Set defaults
		if config.OpenAIBaseURL == "" {
			config.OpenAIBaseURL = "https://api.openai.com/v1"
		}
		if config.Model == "" {
			config.Model = "gpt-4"
		}
		if config.MaxTokens == 0 {
			config.MaxTokens = 4096
		}
		if config.Temperature == 0 {
			config.Temperature = 0.7
		}

		// Update service
		if chatService != nil {
			chatService.config = config
		}

		rest.WriteResponse(http.StatusOK, w, r, map[string]string{
			"message": "Chat configuration updated",
		})

	default:
		rest.WriteResponse(http.StatusMethodNotAllowed, w, r, map[string]string{
			"error": "Method not allowed",
		})
	}
}