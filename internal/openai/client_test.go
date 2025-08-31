package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				APIKey:  "test-key",
				BaseURL: "https://api.openai.com/v1/",
				Timeout: 30 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "empty base URL uses default",
			config: Config{
				APIKey: "test-key",
			},
			wantErr: false,
		},
		{
			name: "zero timeout uses default",
			config: Config{
				APIKey:  "test-key",
				BaseURL: "https://api.openai.com/v1/",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := New(tt.config, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && client == nil {
				t.Error("New() returned nil client")
			}
		})
	}
}

func TestClient_GetModels(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"object": "list",
				"data": [
					{
						"id": "gpt-4",
						"object": "model",
						"created": 1687882411,
						"owned_by": "openai"
					},
					{
						"id": "gpt-3.5-turbo",
						"object": "model", 
						"created": 1677610602,
						"owned_by": "openai"
					}
				]
			}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	config := Config{
		APIKey:  "test-key",
		BaseURL: server.URL + "/",
		Timeout: 10 * time.Second,
	}

	client, err := New(config, nil)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx := context.Background()
	models, err := client.GetModels(ctx)
	if err != nil {
		t.Fatalf("GetModels() error = %v", err)
	}

	if models == nil {
		t.Fatal("GetModels() returned nil")
	}

	if len(models.Data) != 2 {
		t.Errorf("Expected 2 models, got %d", len(models.Data))
	}

	if models.Data[0].ID != "gpt-4" {
		t.Errorf("Expected first model to be gpt-4, got %s", models.Data[0].ID)
	}
}

func TestClient_ChatCompletion_NonStreaming(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"id": "chatcmpl-123",
				"object": "chat.completion",
				"created": 1677652288,
				"model": "gpt-4",
				"choices": [{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "Hello! How can I help you today?"
					},
					"finish_reason": "stop"
				}],
				"usage": {
					"prompt_tokens": 9,
					"completion_tokens": 12,
					"total_tokens": 21
				}
			}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	config := Config{
		APIKey:  "test-key",
		BaseURL: server.URL + "/",
		Timeout: 10 * time.Second,
	}

	client, err := New(config, nil)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	req := ChatCompletionRequest{
		Model: "gpt-4",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
		MaxTokens:   100,
		Temperature: 0.7,
	}

	ctx := context.Background()
	req.Stream = false
	response, err := client.ChatCompletion(ctx, req, nil)
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	if response == nil {
		t.Fatal("ChatCompletion() returned nil")
	}

	if len(response.Choices) != 1 {
		t.Errorf("Expected 1 choice, got %d", len(response.Choices))
	}

	if response.Choices[0].Message.Content != "Hello! How can I help you today?" {
		t.Errorf("Unexpected response content: %s", response.Choices[0].Message.Content)
	}
}

func TestClient_ChatCompletion_Streaming(t *testing.T) {
	// Create mock server for streaming
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")

			// Send streaming chunks
			chunks := []string{
				`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}` + "\n\n",
				`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}` + "\n\n",
				`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":null}]}` + "\n\n",
				`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n",
				`data: [DONE]` + "\n\n",
			}

			for _, chunk := range chunks {
				w.Write([]byte(chunk))
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	config := Config{
		APIKey:  "test-key",
		BaseURL: server.URL + "/",
		Timeout: 10 * time.Second,
	}

	client, err := New(config, nil)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	req := ChatCompletionRequest{
		Model: "gpt-4",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
		MaxTokens:   100,
		Temperature: 0.7,
	}

	var chunks []string
	streamCallback := func(content string, eventType string, data interface{}) error {
		if eventType == "content" {
			chunks = append(chunks, content)
		}
		return nil
	}

	ctx := context.Background()
	req.Stream = true
	response, err := client.ChatCompletion(ctx, req, streamCallback)
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	if response == nil {
		t.Fatal("ChatCompletion() returned nil")
	}

	if len(chunks) == 0 {
		t.Error("Expected streaming chunks, got none")
	}

	// Check that we received some content chunks
	totalContent := strings.Join(chunks, "")
	if !strings.Contains(totalContent, "Hello") {
		t.Errorf("Expected streamed content to contain 'Hello', got: %s", totalContent)
	}
}

func TestClient_FinalizeToolCalls(t *testing.T) {
	client := &Client{}

	toolCallBuffer := map[int]*ToolCall{
		0: {
			Index: 0,
			ID:    "call_1",
			Type:  "function",
			Function: ToolCallFunction{
				Name:      "get_weather",
				Arguments: make(map[string]any),
			},
		},
	}

	argumentsBuffer := map[int]string{
		0: `{"location": "New York"}`,
	}

	toolCalls := client.finalizeToolCalls(toolCallBuffer, argumentsBuffer)

	if len(toolCalls) != 1 {
		t.Errorf("Expected 1 tool call, got %d", len(toolCalls))
	}

	if toolCalls[0].Function.Name != "get_weather" {
		t.Errorf("Expected function name 'get_weather', got %s", toolCalls[0].Function.Name)
	}

	if location, ok := toolCalls[0].Function.Arguments["location"]; !ok || location != "New York" {
		t.Errorf("Expected location argument 'New York', got %v", location)
	}
}
