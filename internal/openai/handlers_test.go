package openai

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandlers_GetModels(t *testing.T) {
	// Create mock upstream server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"object": "list",
				"data": [
					{
						"id": "test-model",
						"object": "model",
						"created": 1677652288,
						"owned_by": "test"
					}
				]
			}`))
		}
	}))
	defer upstream.Close()

	// Create OpenAI client pointing to mock upstream
	config := Config{
		APIKey:  "test-key",
		BaseURL: upstream.URL + "/",
		Timeout: 10 * time.Second,
	}

	client, err := New(config, nil)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create service and test the models endpoint directly
	service := NewService(client, "")
	req := httptest.NewRequest("GET", "/v1/models", nil)
	w := httptest.NewRecorder()

	service.HandleGetModels(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response ModelsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(response.Data) != 1 {
		t.Errorf("Expected 1 model, got %d", len(response.Data))
	}

	if response.Data[0].ID != "test-model" {
		t.Errorf("Expected model ID 'test-model', got %s", response.Data[0].ID)
	}
}

func TestHandlers_ChatCompletions_NonStreaming(t *testing.T) {
	// Create mock upstream server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"id": "chatcmpl-123",
				"object": "chat.completion",
				"created": 1677652288,
				"model": "test-model",
				"choices": [{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "Hello! How can I help you?"
					},
					"finish_reason": "stop"
				}]
			}`))
		}
	}))
	defer upstream.Close()

	// Create OpenAI client pointing to mock upstream
	config := Config{
		APIKey:  "test-key",
		BaseURL: upstream.URL + "/",
		Timeout: 10 * time.Second,
	}

	client, err := New(config, nil)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create service and test chat completions directly
	service := NewService(client, "")

	// Create request body
	message := Message{Role: "user"}
	message.SetContentAsString("Hello")
	reqBody := ChatCompletionRequest{
		Model: "test-model",
		Messages: []Message{message},
		Stream: false,
	}

	jsonBody, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	service.HandleChatCompletions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response ChatCompletionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(response.Choices) != 1 {
		t.Errorf("Expected 1 choice, got %d", len(response.Choices))
	}

	if response.Choices[0].Message.GetContentAsString() != "Hello! How can I help you?" {
		t.Errorf("Unexpected response content: %s", response.Choices[0].Message.GetContentAsString())
	}
}