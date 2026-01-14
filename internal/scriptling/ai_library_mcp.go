package scriptling

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/openai"
	scriptlib "github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/object"
)

// GetAIMCPLibrary returns the AI helper library for MCP environment (uses MCP server directly)
func GetAIMCPLibrary(openaiClient *openai.Client) *object.Library {
	functions := map[string]*object.Builtin{
		"completion": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return aiCompletionMCP(ctx, openaiClient, kwargs.Kwargs, args...)
			},
			HelpText: "completion(messages) - Get AI completion from a list of messages. Each message should be a dict with 'role' and 'content' keys.",
		},
		"response_create": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return responseCreateMCP(ctx, openaiClient, kwargs.Kwargs, args...)
			},
			HelpText: "response_create(input, model=None, instructions=None, previous_response_id=None, background=False) - Create AI response. Returns response dict by default, or response_id if background=True.",
		},
		"response_get": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return responseGetMCP(kwargs.Kwargs, args...)
			},
			HelpText: "response_get(id) - Get response by ID. Returns dict with status and result.",
		},
		"response_wait": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return responseWaitMCP(kwargs.Kwargs, args...)
			},
			HelpText: "response_wait(id, timeout) - Wait for response completion. timeout is in seconds (default 300). Returns response dict.",
		},
		"response_cancel": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return responseCancelMCP(kwargs.Kwargs, args...)
			},
			HelpText: "response_cancel(id) - Cancel in-progress response.",
		},
		"response_delete": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return responseDeleteMCP(kwargs.Kwargs, args...)
			},
			HelpText: "response_delete(id) - Delete response.",
		},
	}

	return object.NewLibrary(functions, nil, "Knot AI completion functions")
}

// aiCompletionMCP gets AI completion using direct OpenAI client
func aiCompletionMCP(ctx context.Context, openaiClient *openai.Client, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{Message: "completion() requires messages array"}
	}

	if openaiClient == nil {
		return &object.Error{Message: "AI completion not available - OpenAI client not configured"}
	}

	messagesList, err := args[0].AsList()
	if err != nil {
		return &object.Error{Message: "completion() first argument must be a list of messages"}
	}

	openaiMessages := make([]openai.Message, 0, len(messagesList))
	for i, msgObj := range messagesList {
		msgDict, err := msgObj.AsDict()
		if err != nil {
			return &object.Error{Message: fmt.Sprintf("message %d must be a dict with 'role' and 'content' keys", i)}
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
			return &object.Error{Message: fmt.Sprintf("message %d missing 'role' or 'content' key", i)}
		}

		openaiMessages = append(openaiMessages, openai.Message{
			Role:    role,
			Content: content,
		})
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
	response, openaiErr := openaiClient.ChatCompletion(aiCtx, req)
	if openaiErr != nil {
		errMsg := fmt.Sprintf("AI completion failed: %v", openaiErr)
		if containsTimeOrDeadline(openaiErr.Error()) {
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

// responseCreateMCP creates a new async response using direct database access
func responseCreateMCP(ctx context.Context, openaiClient *openai.Client, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if openaiClient == nil {
		return &object.Error{Message: "Response creation not available - OpenAI client not configured"}
	}

	// Get user from context
	userVal := ctx.Value("user")
	if userVal == nil {
		return &object.Error{Message: "User context required"}
	}
	userId, ok := userVal.(string)
	if !ok {
		return &object.Error{Message: "Invalid user context"}
	}

	// Get input (required)
	if len(args) < 1 {
		return &object.Error{Message: "response_create() requires input argument"}
	}

	// Build request
	rawInput := scriptlib.ToGo(args[0])

	// Convert input to []any format expected by CreateResponseRequest
	var input []any
	switch v := rawInput.(type) {
	case string:
		input = []any{v}
	case []any:
		input = v
	case map[string]interface{}:
		input = []any{v}
	default:
		return &object.Error{Message: fmt.Sprintf("Unsupported input type: %T", rawInput)}
	}

	req := openai.CreateResponseRequest{
		Input: input,
	}

	// Get optional parameters from kwargs
	background := false // Default to synchronous
	if model, ok := kwargs["model"]; ok {
		if modelStr, err := model.AsString(); err == nil {
			req.Model = modelStr
		}
	}
	if instructions, ok := kwargs["instructions"]; ok {
		if instrStr, err := instructions.AsString(); err == nil {
			req.Instructions = instrStr
		}
	}
	if prevResp, ok := kwargs["previous_response_id"]; ok {
		if prevStr, err := prevResp.AsString(); err == nil {
			req.PreviousResponseID = prevStr
		}
	}
	if bg, ok := kwargs["background"]; ok {
		if boolVal, err := bg.AsBool(); err == nil {
			background = boolVal
		}
	}

	// Create response object
	response := model.NewResponse(userId, "", 30*24*time.Hour) // 30 day TTL
	response.SetRequest(req)

	// Save to database
	db := database.GetInstance()
	if err := db.SaveResponse(response); err != nil {
		return &object.Error{Message: fmt.Sprintf("Failed to save response: %v", err)}
	}

	if background {
		// Async: enqueue for processing and return just the response_id
		openai.EnqueueResponse(response)
		return &object.String{Value: response.Id}
	}

	// Sync: process immediately and return the full response
	// Create independent context for AI completion to prevent script timeout from canceling AI operations
	aiCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Copy user from original context if it exists
	if user := ctx.Value("user"); user != nil {
		aiCtx = context.WithValue(aiCtx, "user", user)
	}

	result, err := openai.ProcessResponseSynchronously(aiCtx, openaiClient, response)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("Failed to process response: %v", err)}
	}

	// Return the full response
	responseData := map[string]interface{}{
		"response_id": result.Id,
		"status":      string(result.Status),
	}
	if result.Status == model.StatusCompleted {
		var respData map[string]interface{}
		if err := result.GetResponse(&respData); err == nil {
			responseData["response"] = respData
		}
	}
	if result.Error != "" {
		responseData["error"] = result.Error
	}

	return scriptlib.FromGo(responseData)
}

// responseGetMCP retrieves a response by ID using direct database access
func responseGetMCP(kwargs map[string]object.Object, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{Message: "response_get() requires response_id"}
	}

	responseId, err := args[0].AsString()
	if err != nil {
		return &object.Error{Message: "response_get() first argument must be a string (response_id)"}
	}

	// Get from database
	db := database.GetInstance()
	response, dbErr := db.GetResponse(responseId)
	if dbErr != nil {
		return &object.Error{Message: fmt.Sprintf("Failed to get response: %v", dbErr)}
	}
	if response == nil || response.IsDeleted {
		return &object.Error{Message: "Response not found"}
	}

	// Convert to scriptling object
	result := map[string]interface{}{
		"response_id": response.Id,
		"status":      string(response.Status),
		"request":     response.Request,
		"response":    response.Response,
	}
	if response.Error != "" {
		result["error"] = response.Error
	}

	return scriptlib.FromGo(result)
}

// responseWaitMCP waits for a response to complete using direct database access
func responseWaitMCP(kwargs map[string]object.Object, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{Message: "response_wait() requires response_id"}
	}

	responseId, err := args[0].AsString()
	if err != nil {
		return &object.Error{Message: "response_wait() first argument must be a string (response_id)"}
	}

	// Get timeout (default 300 seconds)
	timeout := 300 * time.Second
	if len(args) >= 2 {
		if timeoutInt, err := args[1].AsInt(); err == nil {
			timeout = time.Duration(timeoutInt) * time.Second
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	db := database.GetInstance()

	// Poll for completion
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return &object.Error{Message: "Timeout waiting for response to complete"}
		case <-ticker.C:
			response, err := db.GetResponse(responseId)
			if err != nil {
				continue
			}
			if response == nil || response.IsDeleted {
				return &object.Error{Message: "Response not found"}
			}

			// Check if complete
			if response.Status == model.StatusCompleted || response.Status == model.StatusFailed || response.Status == model.StatusCancelled {
				result := map[string]interface{}{
					"response_id": response.Id,
					"status":      string(response.Status),
					"request":     response.Request,
					"response":    response.Response,
				}
				if response.Error != "" {
					result["error"] = response.Error
				}
				return scriptlib.FromGo(result)
			}
		}
	}
}

// responseCancelMCP cancels an in-progress response using direct database access
func responseCancelMCP(kwargs map[string]object.Object, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{Message: "response_cancel() requires response_id"}
	}

	responseId, err := args[0].AsString()
	if err != nil {
		return &object.Error{Message: "response_cancel() first argument must be a string (response_id)"}
	}

	// Get from database
	db := database.GetInstance()
	response, dbErr := db.GetResponse(responseId)
	if dbErr != nil {
		return &object.Error{Message: fmt.Sprintf("Failed to get response: %v", dbErr)}
	}
	if response == nil || response.IsDeleted {
		return &object.Error{Message: "Response not found"}
	}

	// Check if can be cancelled
	if response.Status != model.StatusPending && response.Status != model.StatusInProgress {
		return &object.Error{Message: fmt.Sprintf("Cannot cancel response with status: %s", response.Status)}
	}

	// Update status
	response.Status = model.StatusCancelled
	response.Error = "Cancelled by user"

	if err := db.SaveResponse(response); err != nil {
		return &object.Error{Message: fmt.Sprintf("Failed to cancel response: %v", err)}
	}

	// Cancel in worker pool and gossip the cancellation
	openai.CancelResponse(response)

	return &object.Boolean{Value: true}
}

// responseDeleteMCP deletes a response using direct database access
func responseDeleteMCP(kwargs map[string]object.Object, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{Message: "response_delete() requires response_id"}
	}

	responseId, err := args[0].AsString()
	if err != nil {
		return &object.Error{Message: "response_delete() first argument must be a string (response_id)"}
	}

	// Get from database
	db := database.GetInstance()
	response, dbErr := db.GetResponse(responseId)
	if dbErr != nil {
		return &object.Error{Message: fmt.Sprintf("Failed to get response: %v", dbErr)}
	}
	if response == nil || response.IsDeleted {
		// Already deleted or doesn't exist
		return &object.Boolean{Value: true}
	}

	// Delete
	if err := db.DeleteResponse(response); err != nil {
		return &object.Error{Message: fmt.Sprintf("Failed to delete response: %v", err)}
	}

	// Gossip the deletion so all cluster nodes are aware
	openai.CancelResponse(response)

	return &object.Boolean{Value: true}
}

// Helper function to check for timeout-related errors
func containsTimeOrDeadline(s string) bool {
	return strings.Contains(s, "timeout") || strings.Contains(s, "deadline exceeded")
}
