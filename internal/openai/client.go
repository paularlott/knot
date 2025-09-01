package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/util/rest"

	"github.com/paularlott/mcp"
	"github.com/rs/zerolog/log"
)

// Client represents an OpenAI API client
type Client struct {
	restClient *rest.RESTClient
	mcpServer  *mcp.Server // Optional - if nil, tool calls won't be processed
}

// Config holds configuration for the OpenAI client
type Config struct {
	APIKey  string
	BaseURL string
	Timeout time.Duration
}

type ToolFilter func(toolName string) bool

// New creates a new OpenAI client
func New(config Config, mcpServer *mcp.Server) (*Client, error) {
	if config.BaseURL == "" {
		config.BaseURL = "https://api.openai.com/v1/"
	}

	if config.Timeout == 0 {
		config.Timeout = 5 * time.Minute
	}

	restClient, err := rest.NewClient(config.BaseURL, config.APIKey, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create REST client: %w", err)
	}

	restClient.SetTimeout(config.Timeout)
	restClient.SetTokenFormat("Bearer %s")
	restClient.SetContentType(rest.ContentTypeJSON)
	restClient.SetAccept(rest.ContentTypeJSON)

	return &Client{
		restClient: restClient,
		mcpServer:  mcpServer,
	}, nil
}

// GetModels retrieves the list of available models from OpenAI
func (c *Client) GetModels(ctx context.Context) (*ModelsResponse, error) {
	var response ModelsResponse

	_, err := c.restClient.Get(ctx, "models", &response)
	if err != nil {
		return nil, fmt.Errorf("failed to get models: %w", err)
	}

	return &response, nil
}

// nonStreamingChatCompletion handles non-streaming chat completion
func (c *Client) nonStreamingChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	var response ChatCompletionResponse

	_, err := c.restClient.Post(ctx, "chat/completions", req, &response, http.StatusOK)
	if err != nil {
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}

	return &response, nil
}

// streamChatCompletion handles streaming chat completion
func (c *Client) streamChatCompletion(ctx context.Context, req ChatCompletionRequest, callback func(ChatCompletionResponse) (bool, error)) (*ChatCompletionResponse, error) {
	var finalResponse *ChatCompletionResponse
	var toolCalls []ToolCall
	var assistantContent strings.Builder

	// Stream state for accumulating tool calls
	toolCallBuffer := make(map[int]*ToolCall)
	argumentsBuffer := make(map[int]string)

	err := rest.StreamData(
		c.restClient,
		ctx,
		"POST",
		"chat/completions",
		req,
		func(response *ChatCompletionResponse) (bool, error) {
			if response == nil {
				return false, fmt.Errorf("received nil response from OpenAI")
			}

			// Process the chunk
			shouldStop, err := c.processStreamChunk(response, &toolCallBuffer, &argumentsBuffer, &assistantContent)
			if err != nil {
				return true, err
			}

			// Call the user's callback
			if callback != nil {
				stop, callbackErr := callback(*response)
				if callbackErr != nil {
					return true, callbackErr
				}
				if stop {
					return true, nil
				}
			}

			// Check if we should stop streaming
			if shouldStop {
				// Finalize tool calls
				toolCalls = c.finalizeToolCalls(toolCallBuffer, argumentsBuffer)

				// Create final response
				finishReason := ""
				if len(response.Choices) > 0 {
					finishReason = response.Choices[0].FinishReason
				}

				finalResponse = &ChatCompletionResponse{
					Choices: []Choice{
						{
							Message: Message{
								Role:      "assistant",
								Content:   assistantContent.String(),
								ToolCalls: toolCalls,
							},
							FinishReason: finishReason,
						},
					},
				}

				return true, nil
			}

			return false, nil
		},
	)

	if err != nil {
		return nil, fmt.Errorf("streaming failed: %w", err)
	}

	// Tool calls are not automatically processed - that's the caller's responsibility

	return finalResponse, nil
}

// processStreamChunk processes a single streaming chunk
func (c *Client) processStreamChunk(response *ChatCompletionResponse, toolCallBuffer *map[int]*ToolCall, argumentsBuffer *map[int]string, assistantContent *strings.Builder) (bool, error) {
	if len(response.Choices) == 0 {
		return false, nil
	}

	choice := response.Choices[0]

	// Handle tool calls
	if len(choice.Delta.ToolCalls) > 0 {
		for _, deltaCall := range choice.Delta.ToolCalls {
			index := deltaCall.Index
			if (*toolCallBuffer)[index] == nil {
				(*toolCallBuffer)[index] = &ToolCall{
					Index:    index,
					Function: ToolCallFunction{Arguments: make(map[string]any)},
				}
				(*argumentsBuffer)[index] = ""
			}
			if deltaCall.ID != "" {
				(*toolCallBuffer)[index].ID = deltaCall.ID
			}
			if deltaCall.Type != "" {
				(*toolCallBuffer)[index].Type = deltaCall.Type
			}
			if deltaCall.Function.Name != "" {
				(*toolCallBuffer)[index].Function.Name = deltaCall.Function.Name
			}
			if deltaCall.Function.Arguments != "" {
				(*argumentsBuffer)[index] += deltaCall.Function.Arguments
			}
		}
	}

	// Handle content
	if choice.Delta.Content != "" {
		assistantContent.WriteString(choice.Delta.Content)
	}

	// Check for finish reason
	if choice.FinishReason != "" {
		return true, nil
	}

	return false, nil
}

// finalizeToolCalls converts buffered tool calls to final format
func (c *Client) finalizeToolCalls(toolCallBuffer map[int]*ToolCall, argumentsBuffer map[int]string) []ToolCall {
	if len(toolCallBuffer) == 0 {
		return nil
	}

	toolCalls := make([]ToolCall, 0, len(toolCallBuffer))

	for index, toolCall := range toolCallBuffer {
		if toolCall == nil || toolCall.Function.Name == "" {
			continue
		}

		// Parse arguments if present
		if argsStr := argumentsBuffer[index]; argsStr != "" && argsStr != "null" {
			if err := json.Unmarshal([]byte(argsStr), &toolCall.Function.Arguments); err != nil {
				log.Error().Err(err).Str("tool_name", toolCall.Function.Name).Msg("Failed to parse tool arguments")
				toolCall.Function.Arguments = make(map[string]any)
			}
		} else {
			toolCall.Function.Arguments = make(map[string]any)
		}

		// Ensure ID is set
		if toolCall.ID == "" {
			toolCall.ID = fmt.Sprintf("call_%d", index)
		}

		toolCalls = append(toolCalls, *toolCall)
	}

	return toolCalls
}

// getAvailableTools returns the list of available tools from MCP server
func (c *Client) getAvailableTools(filter ToolFilter) ([]Tool, error) {
	if c.mcpServer == nil {
		return []Tool{}, nil
	}

	// Get tools directly from MCP server
	tools := c.mcpServer.ListTools()
	var openAITools []Tool

	for _, tool := range tools {
		// Apply filter if provided
		if filter != nil && !filter(tool.Name) {
			continue // Skip this tool if filter rejects it
		}

		// Safely convert InputSchema to map[string]any
		var parameters map[string]any
		if tool.InputSchema != nil {
			if params, ok := tool.InputSchema.(map[string]any); ok {
				parameters = params
			} else {
				log.Warn().Str("tool_name", tool.Name).Msg("Tool InputSchema is not a map, using empty parameters")
				parameters = make(map[string]any)
			}
		} else {
			parameters = make(map[string]any)
		}

		openAITools = append(openAITools, Tool{
			Type: "function",
			Function: ToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  parameters,
			},
		})
	}

	return openAITools, nil
}

// StreamCallback defines the callback function for streaming events
type StreamCallback func(event StreamEvent) error

const MAX_TOOL_CALL_ITERATIONS = 20

// ChatCompletion performs a chat completion request with automatic tool handling
// Automatically detects if MCP server is available and adds tools if so
// Uses req.Stream to determine streaming vs non-streaming mode
// If streaming and streamCallback is provided, calls streamCallback for each event
func (c *Client) ChatCompletion(ctx context.Context, req ChatCompletionRequest, streamCallback StreamCallback, toolFilter ToolFilter) (*ChatCompletionResponse, error) {
	currentMessages := req.Messages

	// Get available tools
	tools, err := c.getAvailableTools(toolFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to get available tools: %w", err)
	}

	// Add tools to request
	req.Tools = tools

	// Iterative tool call loop
	for iteration := 0; iteration < MAX_TOOL_CALL_ITERATIONS; iteration++ {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Update messages for this iteration
		req.Messages = currentMessages

		// Call OpenAI API with streaming and retry logic
		response, err := c.chatCompletionWithRetry(ctx, req, streamCallback)

		if err != nil {
			return nil, fmt.Errorf("chat completion failed on iteration %d: %w", iteration, err)
		}

		if response == nil {
			return nil, fmt.Errorf("received nil response on iteration %d", iteration)
		}

		// If no tool calls, no tools or no mcp server, we're done
		if len(tools) == 0 || len(response.Choices) == 0 || len(response.Choices[0].Message.ToolCalls) == 0 {
			// Send completion event
			if streamCallback != nil {
				streamCallback(DoneEvent{})
			}
			return response, nil
		}

		message := response.Choices[0].Message
		toolCalls := message.ToolCalls
		assistantContent := message.Content

		// Send tool calls to stream
		if streamCallback != nil {
			if err := streamCallback(ToolCallsEvent{ToolCalls: toolCalls}); err != nil {
				return nil, err
			}
		}

		// Execute tools and collect results
		var toolResults []Message
		for _, toolCall := range toolCalls {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			result, err := c.executeToolCall(ctx, toolCall)
			if err != nil {
				log.Error().Err(err).Str("tool_name", toolCall.Function.Name).Msg("Tool execution failed")
				result = fmt.Sprintf("Error executing tool %s: %s", toolCall.Function.Name, err.Error())
			}

			toolResults = append(toolResults, Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: toolCall.ID,
			})

			// Send tool result to stream
			if streamCallback != nil {
				if err := streamCallback(ToolResultEvent{
					ToolName:   toolCall.Function.Name,
					Result:     result,
					ToolCallID: toolCall.ID,
				}); err != nil {
					return nil, err
				}
			}
		}

		// Add assistant message with tool calls to conversation
		currentMessages = append(currentMessages, Message{
			Role:      "assistant",
			Content:   assistantContent,
			ToolCalls: toolCalls,
		})

		// Add tool results to conversation
		currentMessages = append(currentMessages, toolResults...)
	}

	// If we reach here, we've hit the iteration limit
	if streamCallback != nil {
		streamCallback(ErrorEvent{
			Error: fmt.Sprintf("Maximum tool call limit (%d) reached. The conversation has been stopped to prevent infinite loops.", MAX_TOOL_CALL_ITERATIONS),
		})
	}

	return nil, fmt.Errorf("maximum tool call iterations (%d) reached", MAX_TOOL_CALL_ITERATIONS)
}

// executeToolCall executes a single tool call using the MCP server
func (c *Client) executeToolCall(ctx context.Context, toolCall ToolCall) (string, error) {
	if c.mcpServer == nil {
		return "", fmt.Errorf("MCP server not configured")
	}

	response, err := c.mcpServer.CallTool(ctx, toolCall.Function.Name, toolCall.Function.Arguments)
	if err != nil {
		return "", fmt.Errorf("MCP tool call failed: %w", err)
	}

	// Priority 1: Structured content
	if response.StructuredContent != nil {
		jsonBytes, err := json.Marshal(response.StructuredContent)
		if err != nil {
			return "", fmt.Errorf("failed to serialize structured tool response: %w", err)
		}
		return string(jsonBytes), nil
	}

	// Priority 2: Text content
	for _, content := range response.Content {
		if content.Type == "text" {
			return content.Text, nil
		}
	}

	return "Tool executed successfully", nil
}

func (c *Client) chatCompletionWithRetry(ctx context.Context, req ChatCompletionRequest, streamCallback StreamCallback) (*ChatCompletionResponse, error) {
	maxRetries := 2

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Warn().Int("attempt", attempt+1).Msg("Retrying OpenAI API call")
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}

		req.Stream = streamCallback != nil
		var response *ChatCompletionResponse
		var err error

		if req.Stream {
			response, err = c.streamChatCompletion(ctx, req, c.internalCallback(streamCallback))
		} else {
			response, err = c.nonStreamingChatCompletion(ctx, req)
		}

		if err == nil || !c.isRetryableError(err) {
			return response, err
		}

		log.Warn().Err(err).Int("attempt", attempt+1).Msg("Retryable error occurred")
	}

	return nil, fmt.Errorf("max retries exceeded")
}

func (c *Client) internalCallback(streamCallback StreamCallback) func(ChatCompletionResponse) (bool, error) {
	if streamCallback == nil {
		return nil
	}

	return func(resp ChatCompletionResponse) (bool, error) {
		if len(resp.Choices) > 0 {
			choice := resp.Choices[0]

			// Handle regular content
			if choice.Delta.Content != "" {
				if err := streamCallback(ContentEvent{Content: choice.Delta.Content}); err != nil {
					return false, err
				}
			}

			// Handle reasoning content (thinking blocks)
			if choice.Delta.ReasoningContent != "" {
				if err := streamCallback(ReasoningEvent{Content: choice.Delta.ReasoningContent}); err != nil {
					return false, err
				}
			}
		}
		return false, nil
	}
}

// isRetryableError determines if an error is worth retrying
func (c *Client) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Retry on timeout, connection issues, and 5xx errors
	retryablePatterns := []string{
		"context deadline exceeded",
		"timeout",
		"connection refused",
		"connection reset",
		"temporary failure",
		"500",
		"502",
		"503",
		"504",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}
