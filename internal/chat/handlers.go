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
