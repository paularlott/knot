package scriptling

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/paularlott/knot/apiclient"
	scriptlib "github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/object"
)

// CreateResponseRequest represents a request to create a response
type CreateResponseRequest struct {
	Model              string                 `json:"model,omitempty"`
	Input              interface{}            `json:"input"`
	Instructions       string                 `json:"instructions,omitempty"`
	PreviousResponseId string                 `json:"previous_response_id,omitempty"`
	Params             map[string]interface{} `json:"params,omitempty"`
	Background         bool                   `json:"background,omitempty"`
}

// CreateResponseResponse represents the full response from creating a response
type CreateResponseResponse struct {
	ID        string                 `json:"id"`
	Status    string                 `json:"status"`
	Output    []interface{}          `json:"output"`
	Error     map[string]interface{} `json:"error,omitempty"`
	CreatedAt int64                  `json:"created_at,omitempty"`
}

// GetResponseResponse represents the response from getting a response
type GetResponseResponse struct {
	ResponseId string                 `json:"response_id"`
	Status     string                 `json:"status"`
	Request    map[string]interface{} `json:"request,omitempty"`
	Response   map[string]interface{} `json:"response,omitempty"`
	Error      string                 `json:"error,omitempty"`
}

// GetAILibrary returns the AI helper library for scriptling (local/remote environments)
func GetAILibrary(client *apiclient.ApiClient, userId string) *object.Library {
	functions := map[string]*object.Builtin{
		"completion": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return aiCompletion(ctx, client, userId, kwargs, args...)
			},
			HelpText: "completion(messages) - Get AI completion from a list of messages. Each message should be a dict with 'role' and 'content' keys.",
		},
		"response_create": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return responseCreate(ctx, client, userId, kwargs, args...)
			},
			HelpText: "response_create(input, model=None, instructions=None, previous_response_id=None, background=False) - Create AI response. Returns response dict by default, or response_id if background=True.",
		},
		"response_get": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return responseGet(ctx, client, kwargs, args...)
			},
			HelpText: "response_get(id) - Get response by ID. Returns dict with status and result.",
		},
		"response_wait": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return responseWait(ctx, client, kwargs, args...)
			},
			HelpText: "response_wait(id, timeout) - Wait for response completion. timeout is in seconds (default 300). Returns response dict.",
		},
		"response_cancel": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return responseCancel(ctx, client, kwargs, args...)
			},
			HelpText: "response_cancel(id) - Cancel in-progress response.",
		},
		"response_delete": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return responseDelete(ctx, client, kwargs, args...)
			},
			HelpText: "response_delete(id) - Delete response.",
		},
	}

	return object.NewLibrary(functions, nil, "AI completion functions")
}

// aiCompletion gets AI completion via API
func aiCompletion(ctx context.Context, client *apiclient.ApiClient, userId string, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{Message: "completion() requires messages array"}
	}

	if client == nil {
		return &object.Error{Message: "AI completion not available - API client not configured"}
	}

	messagesList, ok := args[0].(*object.List)
	if !ok {
		return &object.Error{Message: "completion() first argument must be a list of messages"}
	}

	messages := make([]ChatMessage, 0, len(messagesList.Elements))
	for i, msgObj := range messagesList.Elements {
		msgDict, ok := msgObj.(*object.Dict)
		if !ok {
			return &object.Error{Message: fmt.Sprintf("message %d must be a dict with 'role' and 'content' keys", i)}
		}

		role, content := "", ""
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

		if role == "" || content == "" {
			return &object.Error{Message: fmt.Sprintf("message %d missing 'role' or 'content' key", i)}
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

	// Copy user from original context
	if user := ctx.Value("user"); user != nil {
		aiCtx = context.WithValue(aiCtx, "user", user)
	}

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

// responseCreate creates a new async response via API
func responseCreate(ctx context.Context, client *apiclient.ApiClient, userId string, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if client == nil {
		return &object.Error{Message: "Response creation not available - API client not configured"}
	}

	// Build request from args
	req := CreateResponseRequest{
		Background: false, // Default to synchronous (background=false)
	}

	// Get input (required)
	if len(args) < 1 {
		return &object.Error{Message: "response_create() requires input argument"}
	}
	req.Input = scriptlib.ToGo(args[0])

	// Get optional parameters from kwargs
	if model, ok := kwargs["model"]; ok {
		if modelStr, ok := model.(*object.String); ok {
			req.Model = modelStr.Value
		}
	}
	if instructions, ok := kwargs["instructions"]; ok {
		if instrStr, ok := instructions.(*object.String); ok {
			req.Instructions = instrStr.Value
		}
	}
	if prevResp, ok := kwargs["previous_response_id"]; ok {
		if prevStr, ok := prevResp.(*object.String); ok {
			req.PreviousResponseId = prevStr.Value
		}
	}
	if background, ok := kwargs["background"]; ok {
		if boolVal, ok := background.(*object.Boolean); ok {
			req.Background = boolVal.Value
		}
	}

	// Create independent context for response creation
	// Use longer timeout for synchronous processing
	timeout := 30 * time.Second
	if !req.Background {
		timeout = 5 * time.Minute // Allow more time for synchronous processing
	}
	aiCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Copy user from original context
	if user := ctx.Value("user"); user != nil {
		aiCtx = context.WithValue(aiCtx, "user", user)
	}

	// Call API to create response
	var resp CreateResponseResponse
	_, err := client.Do(aiCtx, "POST", "v1/responses", req, &resp)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("Failed to create response: %v", err)}
	}

	// If background=true, return just the response_id
	if req.Background {
		return &object.String{Value: resp.ID}
	}

	// Otherwise (background=false), return the full response
	result := map[string]interface{}{
		"response_id": resp.ID,
		"status":      resp.Status,
	}
	if len(resp.Output) > 0 {
		result["output"] = resp.Output
	}
	if resp.Error != nil {
		result["error"] = resp.Error["message"]
	}
	if resp.CreatedAt > 0 {
		result["created_at"] = resp.CreatedAt
	}

	return scriptlib.FromGo(result)
}

// responseGet retrieves a response by ID via API
func responseGet(ctx context.Context, client *apiclient.ApiClient, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if client == nil {
		return &object.Error{Message: "Response retrieval not available - API client not configured"}
	}

	if len(args) < 1 {
		return &object.Error{Message: "response_get() requires response_id"}
	}

	responseId, ok := args[0].(*object.String)
	if !ok {
		return &object.Error{Message: "response_get() first argument must be a string (response_id)"}
	}

	// Create independent context
	aiCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Copy user from original context
	if user := ctx.Value("user"); user != nil {
		aiCtx = context.WithValue(aiCtx, "user", user)
	}

	// Call API to get response
	var resp GetResponseResponse
	_, err := client.Do(aiCtx, "GET", "v1/responses/"+responseId.Value, nil, &resp)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("Failed to get response: %v", err)}
	}

	return scriptlib.FromGo(resp)
}

// responseWait waits for a response to complete via API
func responseWait(ctx context.Context, client *apiclient.ApiClient, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if client == nil {
		return &object.Error{Message: "Response wait not available - API client not configured"}
	}

	if len(args) < 1 {
		return &object.Error{Message: "response_wait() requires response_id"}
	}

	responseId, ok := args[0].(*object.String)
	if !ok {
		return &object.Error{Message: "response_wait() first argument must be a string (response_id)"}
	}

	// Get timeout (default 300 seconds)
	timeout := 300 * time.Second
	if len(args) >= 2 {
		if timeoutObj, ok := args[1].(*object.Integer); ok {
			timeout = time.Duration(timeoutObj.Value) * time.Second
		}
	}

	// Create independent context with timeout
	aiCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Copy user from original context
	if user := ctx.Value("user"); user != nil {
		aiCtx = context.WithValue(aiCtx, "user", user)
	}

	// Poll for completion
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-aiCtx.Done():
			return &object.Error{Message: "Timeout waiting for response to complete"}
		case <-ticker.C:
			var resp GetResponseResponse
			_, err := client.Do(aiCtx, "GET", "v1/responses/"+responseId.Value, nil, &resp)
			if err != nil {
				return &object.Error{Message: fmt.Sprintf("Failed to get response: %v", err)}
			}

			// Check if complete
			if resp.Status == "completed" || resp.Status == "failed" || resp.Status == "cancelled" {
				return scriptlib.FromGo(resp)
			}
		}
	}
}

// responseCancel cancels an in-progress response via API
func responseCancel(ctx context.Context, client *apiclient.ApiClient, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if client == nil {
		return &object.Error{Message: "Response cancellation not available - API client not configured"}
	}

	if len(args) < 1 {
		return &object.Error{Message: "response_cancel() requires response_id"}
	}

	responseId, ok := args[0].(*object.String)
	if !ok {
		return &object.Error{Message: "response_cancel() first argument must be a string (response_id)"}
	}

	// Create independent context
	aiCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Copy user from original context
	if user := ctx.Value("user"); user != nil {
		aiCtx = context.WithValue(aiCtx, "user", user)
	}

	// Call API to cancel response
	_, err := client.Do(aiCtx, "POST", "v1/responses/"+responseId.Value+"/cancel", nil, nil)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("Failed to cancel response: %v", err)}
	}

	return &object.Boolean{Value: true}
}

// responseDelete deletes a response via API
func responseDelete(ctx context.Context, client *apiclient.ApiClient, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if client == nil {
		return &object.Error{Message: "Response deletion not available - API client not configured"}
	}

	if len(args) < 1 {
		return &object.Error{Message: "response_delete() requires response_id"}
	}

	responseId, ok := args[0].(*object.String)
	if !ok {
		return &object.Error{Message: "response_delete() first argument must be a string (response_id)"}
	}

	// Create independent context
	aiCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Copy user from original context
	if user := ctx.Value("user"); user != nil {
		aiCtx = context.WithValue(aiCtx, "user", user)
	}

	// Call API to delete response
	_, err := client.Do(aiCtx, "DELETE", "v1/responses/"+responseId.Value, nil, nil)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("Failed to delete response: %v", err)}
	}

	return &object.Boolean{Value: true}
}
