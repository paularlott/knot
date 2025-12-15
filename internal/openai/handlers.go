package openai

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/paularlott/knot/internal/util/rest"

	"github.com/paularlott/knot/internal/log"
)

type Service struct {
	client       *Client
	systemPrompt string
}

func NewService(client *Client, systemPrompt string) *Service {
	return &Service{
		client:       client,
		systemPrompt: systemPrompt,
	}
}

// Handles GET /v1/models
func (s *Service) HandleGetModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	models, err := s.client.GetModels(ctx)
	if err != nil {
		log.WithError(err).Error("OpenAI: Failed to get models")
		rest.WriteResponse(http.StatusInternalServerError, w, r, map[string]string{
			"error": "Failed to get models",
		})
		return
	}

	rest.WriteResponse(http.StatusOK, w, r, models)
}

// Handles POST /v1/chat/completions
func (s *Service) HandleChatCompletions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.WithError(err).Error("OpenAI: Failed to decode chat completion request")
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{
			"error": "Invalid request body",
		})
		return
	}

	// Strip existing system messages and add our system prompt
	req.Messages = s.replaceSystemPrompt(req.Messages)

	if req.Stream {
		s.handleStreamingChatCompletion(ctx, w, r, req)
	} else {
		s.handleNonStreamingChatCompletion(ctx, w, r, req)
	}
}

// handleNonStreamingChatCompletion handles non-streaming chat completions
func (s *Service) handleNonStreamingChatCompletion(ctx context.Context, w http.ResponseWriter, r *http.Request, req ChatCompletionRequest) {
	response, err := s.client.ChatCompletion(ctx, req)
	if err != nil {
		log.WithError(err).Error("OpenAI: Chat completion failed")
		rest.WriteResponse(http.StatusInternalServerError, w, r, map[string]string{
			"error": "Chat completion failed",
		})
		return
	}

	rest.WriteResponse(http.StatusOK, w, r, response)
}

// handleStreamingChatCompletion handles streaming chat completions
func (s *Service) handleStreamingChatCompletion(ctx context.Context, w http.ResponseWriter, r *http.Request, req ChatCompletionRequest) {
	streamWriter := rest.NewStreamWriter(w, r)
	defer streamWriter.Close()

	stream := s.client.StreamChatCompletion(ctx, req)
	for stream.Next() {
		response := stream.Current()
		if err := streamWriter.WriteChunk(response); err != nil {
			log.WithError(err).Error("OpenAI: Failed to write streaming response")
			return
		}
	}
	streamWriter.WriteEnd()
}

// replaceSystemPrompt checks for existing system messages and injects system prompt only if none present
func (s *Service) replaceSystemPrompt(messages []Message) []Message {
	if s.systemPrompt == "" || (len(messages) > 0 && messages[0].Role == "system") {
		return messages
	}

	systemMessage := Message{Role: "system"}
	systemMessage.SetContentAsString(s.systemPrompt)

	result := make([]Message, 0, len(messages)+1)
	result = append(result, systemMessage)
	result = append(result, messages...)
	return result
}
