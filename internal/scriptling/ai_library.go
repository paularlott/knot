package scriptling

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/openai"
	"github.com/paularlott/scriptling/object"
)

// ChatMessage represents a chat message
type ChatMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp,omitempty"`
}

// ChatCompletionRequest represents a chat completion request
type ChatCompletionRequest struct {
	Messages []ChatMessage `json:"messages"`
}

// ChatCompletionResponse represents a chat completion response
type ChatCompletionResponse struct {
	Content string `json:"content"`
}

// GetAILibrary returns the AI helper library for scriptling (local/remote environments)
func GetAILibrary(client *apiclient.ApiClient, userId string, openaiClient *openai.Client) *object.Library {
	functions := map[string]*object.Builtin{
		"completion": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return aiCompletion(ctx, client, userId, openaiClient, kwargs, args...)
			},
			HelpText: "completion(messages) - Get AI completion from a list of messages. Each message should be a dict with 'role' and 'content' keys.",
		},
	}

	return object.NewLibrary(functions, nil, "AI completion functions")
}

// GetAIMCPLibrary returns the AI helper library for MCP environment (uses MCP server directly)
func GetAIMCPLibrary(openaiClient *openai.Client) *object.Library {
	functions := map[string]*object.Builtin{
		"completion": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return aiCompletionMCP(ctx, openaiClient, kwargs, args...)
			},
			HelpText: "completion(messages) - Get AI completion from a list of messages. Each message should be a dict with 'role' and 'content' keys.",
		},
	}

	return object.NewLibrary(functions, nil, "AI completion functions")
}

func aiCompletion(ctx context.Context, client *apiclient.ApiClient, userId string, openaiClient *openai.Client, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{Message: "completion() requires messages array"}
	}

	// Check if API client is available
	if client == nil {
		return &object.Error{Message: "AI completion not available - API client not configured"}
	}

	// Get messages from first argument
	messagesList, ok := args[0].(*object.List)
	if !ok {
		return &object.Error{Message: "completion() first argument must be a list of messages"}
	}

	// Convert scriptling messages to API format with timestamps
	messages := make([]ChatMessage, 0, len(messagesList.Elements))
	for i, msgObj := range messagesList.Elements {
		msgDict, ok := msgObj.(*object.Dict)
		if !ok {
			return &object.Error{Message: fmt.Sprintf("message %d must be a dict with 'role' and 'content' keys", i)}
		}

		// Extract role
		role := ""
		content := ""
		for _, pair := range msgDict.Pairs {
			key := pair.Key.(*object.String).Value
			if key == "role" {
				if roleStr, ok := pair.Value.(*object.String); ok {
					role = roleStr.Value
				}
			} else if key == "content" {
				if contentStr, ok := pair.Value.(*object.String); ok {
					content = contentStr.Value
				}
			}
		}

		if role == "" {
			return &object.Error{Message: fmt.Sprintf("message %d missing 'role' key", i)}
		}
		if content == "" {
			return &object.Error{Message: fmt.Sprintf("message %d missing 'content' key", i)}
		}

		messages = append(messages, ChatMessage{
			Role:      role,
			Content:   content,
			Timestamp: time.Now().Unix(),
		})
	}

	// Create request
	req := ChatCompletionRequest{
		Messages: messages,
	}

	// Create independent context for AI completion to prevent script timeout from canceling AI operations
	aiCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Call API - the server will handle tool calling via MCP server integration
	var response ChatCompletionResponse
	_, err := client.Do(aiCtx, "POST", "api/chat/completion", req, &response)
	if err != nil {
		// Provide more helpful error message
		errMsg := fmt.Sprintf("AI completion failed: %v", err)
		if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline exceeded") {
			errMsg += "\nNote: Make sure the server has AI chat enabled with valid OpenAI credentials"
		} else if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			errMsg += "\nNote: The server may not have the chat completion endpoint enabled"
		}
		return &object.Error{Message: errMsg}
	}

	return &object.String{Value: response.Content}
}

// makeLocalChatCompletion makes a direct HTTP call to the local server for AI completion
// This is used by MCP tools running within the server context
func makeLocalChatCompletion(ctx context.Context, req ChatCompletionRequest, userID string) (*ChatCompletionResponse, error) {
	// Create JSON request body
	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request to local server
	httpReq, err := http.NewRequestWithContext(ctx, "POST", "http://localhost:8080/api/chat/completion", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	if userID != "" {
		httpReq.Header.Set("X-User-ID", userID)
	}

	// Make the request
	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse response
	var response ChatCompletionResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &response, nil
}

func aiCompletionMCP(ctx context.Context, openaiClient *openai.Client, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{Message: "completion() requires messages array"}
	}

	// Get messages from first argument
	messagesList, ok := args[0].(*object.List)
	if !ok {
		return &object.Error{Message: "completion() first argument must be a list of messages"}
	}

	// Convert to OpenAI message format
	openaiMessages := make([]openai.Message, 0, len(messagesList.Elements))
	for i, msgObj := range messagesList.Elements {
		msgDict, ok := msgObj.(*object.Dict)
		if !ok {
			return &object.Error{Message: fmt.Sprintf("message %d must be a dict with 'role' and 'content' keys", i)}
		}

		// Extract role and content
		role := ""
		content := ""
		for _, pair := range msgDict.Pairs {
			key := pair.Key.(*object.String).Value
			if key == "role" {
				if roleStr, ok := pair.Value.(*object.String); ok {
					role = roleStr.Value
				}
			} else if key == "content" {
				if contentStr, ok := pair.Value.(*object.String); ok {
					content = contentStr.Value
				}
			}
		}

		if role == "" {
			return &object.Error{Message: fmt.Sprintf("message %d missing 'role' key", i)}
		}
		if content == "" {
			return &object.Error{Message: fmt.Sprintf("message %d missing 'content' key", i)}
		}

		openaiMessages = append(openaiMessages, openai.Message{
			Role:    role,
			Content: content,
		})
	}

	// Check OpenAI client
	if openaiClient == nil {
		return &object.Error{Message: "AI completion not available - OpenAI client not configured"}
	}

	// Create independent context for AI completion to prevent script timeout from canceling AI operations
	aiCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Copy user from original context if it exists
	if user := ctx.Value("user"); user != nil {
		aiCtx = context.WithValue(aiCtx, "user", user)
	}

	// Create chat completion request
	req := openai.ChatCompletionRequest{
		Messages: openaiMessages,
	}

	// Call OpenAI with MCP server integration using the AI-specific context
	response, err := openaiClient.ChatCompletion(aiCtx, req)
	if err != nil {
		errMsg := fmt.Sprintf("AI completion failed: %v", err)
		if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "deadline exceeded") {
			errMsg += "\nNote: Make sure OpenAI API is properly configured"
		}
		return &object.Error{Message: errMsg}
	}

	if len(response.Choices) == 0 {
		return &object.String{Value: ""}
	}

	content := response.Choices[0].Message.GetContentAsString()
	return &object.String{Value: content}
}