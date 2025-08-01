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

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{
			"error": "Invalid request body",
		})
		return
	}

	messages := []ChatMessage{
		{
			Role:      "user",
			Content:   req.Message,
			Timestamp: time.Now().Unix(),
		},
	}

	err := chatService.StreamChat(r.Context(), messages, user, w, r)
	if err != nil {
		// Don't create a new SSE writer here since StreamChat already handles SSE setup
		// Just write a simple error response
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
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
			if err := chatService.UpdateConfig(config); err != nil {
				rest.WriteResponse(http.StatusInternalServerError, w, r, map[string]string{
					"error": "Failed to update chat configuration",
				})
				return
			}
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