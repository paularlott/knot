package chat

import (
	"encoding/json"
	"net/http"

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

	if len(req.Messages) == 0 {
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{
			"error": "No messages provided",
		})
		return
	}

	err := chatService.StreamChat(r.Context(), req.Messages, user, w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
