package scriptling

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/paularlott/knot/apiclient"
	scriptlib "github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/errors"
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
	builder := object.NewLibraryBuilder("ai", "AI completion functions")

	builder.FunctionWithHelp("completion", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return aiCompletion(ctx, client, userId, args...)
	}, "completion(messages) - Get AI completion from a list of messages. Each message should be a dict with 'role' and 'content' keys.")

	builder.FunctionWithHelp("response_create", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return responseCreate(ctx, client, userId, kwargs, args...)
	}, "response_create(input, model=None, instructions=None, previous_response_id=None, background=False) - Create AI response. Returns response dict by default, or response_id if background=True.")

	builder.FunctionWithHelp("response_get", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return responseGet(ctx, client, args...)
	}, "response_get(id) - Get response by ID. Returns dict with status and result.")

	builder.FunctionWithHelp("response_wait", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return responseWait(ctx, client, args...)
	}, "response_wait(id, timeout) - Wait for response completion. timeout is in seconds (default 300). Returns response dict.")

	builder.FunctionWithHelp("response_cancel", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return responseCancel(ctx, client, args...)
	}, "response_cancel(id) - Cancel in-progress response.")

	builder.FunctionWithHelp("response_delete", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return responseDelete(ctx, client, args...)
	}, "response_delete(id) - Delete response.")

	return builder.Build()
}

// aiCompletion gets AI completion via API
func aiCompletion(ctx context.Context, client *apiclient.ApiClient, userId string, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	if client == nil {
		return &object.Error{Message: "AI completion not available - API client not configured"}
	}

	messagesList, err := args[0].AsList()
	if err != nil {
		return errors.ParameterError("messages", err)
	}

	messages := make([]ChatMessage, 0, len(messagesList))
	for i, msgObj := range messagesList {
		msgDict, err := msgObj.AsDict()
		if err != nil {
			return errors.NewError("message %d must be a dict with 'role' and 'content' keys", i)
		}

		role, content := "", ""
		for key, val := range msgDict {
			if key == "role" {
				if roleStr, err := val.AsString(); err == nil {
					role = roleStr
				}
			} else if key == "content" {
				if contentStr, err := val.AsString(); err == nil {
					content = contentStr
				}
			}
		}

		if role == "" || content == "" {
			return errors.NewError("message %d missing 'role' or 'content' key", i)
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
	_, apiErr := client.Do(aiCtx, "POST", "api/chat/completion", req, &response)
	if apiErr != nil {
		// Provide more helpful error message
		errMsg := fmt.Sprintf("AI completion failed: %v", apiErr)
		if strings.Contains(apiErr.Error(), "timeout") || strings.Contains(apiErr.Error(), "deadline exceeded") {
			errMsg += "\nNote: Make sure the server has AI chat enabled with valid OpenAI credentials"
		} else if strings.Contains(apiErr.Error(), "404") || strings.Contains(apiErr.Error(), "not found") {
			errMsg += "\nNote: The server may not have the chat completion endpoint enabled"
		}
		return errors.NewError("%s", errMsg)
	}

	return &object.String{Value: response.Content}
}

// responseCreate creates a new async response via API
func responseCreate(ctx context.Context, client *apiclient.ApiClient, userId string, kwargs object.Kwargs, args ...object.Object) object.Object {
	if client == nil {
		return &object.Error{Message: "Response creation not available - API client not configured"}
	}

	// Build request from args
	req := CreateResponseRequest{
		Background: false, // Default to synchronous (background=false)
	}

	// Get input (required)
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}
	req.Input = scriptlib.ToGo(args[0])

	// Get optional parameters from kwargs
	if model, err := kwargs.GetString("model", ""); err != nil {
		return err
	} else if model != "" {
		req.Model = model
	}

	if instructions, err := kwargs.GetString("instructions", ""); err != nil {
		return err
	} else if instructions != "" {
		req.Instructions = instructions
	}

	if prevResp, err := kwargs.GetString("previous_response_id", ""); err != nil {
		return err
	} else if prevResp != "" {
		req.PreviousResponseId = prevResp
	}

	if background, err := kwargs.GetBool("background", false); err != nil {
		return err
	} else {
		req.Background = background
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
	_, apiErr := client.Do(aiCtx, "POST", "v1/responses", req, &resp)
	if apiErr != nil {
		return errors.NewError("Failed to create response: %v", apiErr)
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
func responseGet(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if client == nil {
		return &object.Error{Message: "Response retrieval not available - API client not configured"}
	}

	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	responseId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("response_id", err)
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
	_, apiErr := client.Do(aiCtx, "GET", "v1/responses/"+responseId, nil, &resp)
	if apiErr != nil {
		return errors.NewError("Failed to get response: %v", apiErr)
	}

	return scriptlib.FromGo(resp)
}

// responseWait waits for a response to complete via API
func responseWait(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if client == nil {
		return &object.Error{Message: "Response wait not available - API client not configured"}
	}

	if err := errors.MinArgs(args, 1); err != nil {
		return err
	}

	responseId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("response_id", err)
	}

	// Get timeout (default 300 seconds)
	timeout := 300 * time.Second
	if len(args) >= 2 {
		if timeoutInt, err := args[1].AsInt(); err == nil {
			timeout = time.Duration(timeoutInt) * time.Second
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
			return errors.NewError("Timeout waiting for response to complete")
		case <-ticker.C:
			var resp GetResponseResponse
			_, apiErr := client.Do(aiCtx, "GET", "v1/responses/"+responseId, nil, &resp)
			if apiErr != nil {
				return errors.NewError("Failed to get response: %v", apiErr)
			}

			// Check if complete
			if resp.Status == "completed" || resp.Status == "failed" || resp.Status == "cancelled" {
				return scriptlib.FromGo(resp)
			}
		}
	}
}

// responseCancel cancels an in-progress response via API
func responseCancel(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if client == nil {
		return &object.Error{Message: "Response cancellation not available - API client not configured"}
	}

	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	responseId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("response_id", err)
	}

	// Create independent context
	aiCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Copy user from original context
	if user := ctx.Value("user"); user != nil {
		aiCtx = context.WithValue(aiCtx, "user", user)
	}

	// Call API to cancel response
	_, apiErr := client.Do(aiCtx, "POST", "v1/responses/"+responseId+"/cancel", nil, nil)
	if apiErr != nil {
		return errors.NewError("Failed to cancel response: %v", apiErr)
	}

	return &object.Boolean{Value: true}
}

// responseDelete deletes a response via API
func responseDelete(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if client == nil {
		return &object.Error{Message: "Response deletion not available - API client not configured"}
	}

	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	responseId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("response_id", err)
	}

	// Create independent context
	aiCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Copy user from original context
	if user := ctx.Value("user"); user != nil {
		aiCtx = context.WithValue(aiCtx, "user", user)
	}

	// Call API to delete response
	_, apiErr := client.Do(aiCtx, "DELETE", "v1/responses/"+responseId, nil, nil)
	if apiErr != nil {
		return errors.NewError("Failed to delete response: %v", apiErr)
	}

	return &object.Boolean{Value: true}
}
