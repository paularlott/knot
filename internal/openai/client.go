package openai

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/util/rest"

	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/mcp"
	mcpopenai "github.com/paularlott/mcp/openai"
)

const MAX_TOOL_CALL_ITERATIONS = 20

// Re-export types from mcp/openai for convenience
type (
	ChatCompletionRequest  = mcpopenai.ChatCompletionRequest
	ChatCompletionResponse = mcpopenai.ChatCompletionResponse
	Message                = mcpopenai.Message
	Choice                 = mcpopenai.Choice
	Delta                  = mcpopenai.Delta
	Tool                   = mcpopenai.Tool
	ToolFunction           = mcpopenai.ToolFunction
	ToolCall               = mcpopenai.ToolCall
	ToolCallFunction       = mcpopenai.ToolCallFunction
	DeltaToolCall          = mcpopenai.DeltaToolCall
	DeltaFunction          = mcpopenai.DeltaFunction
	ModelsResponse         = mcpopenai.ModelsResponse
	Model                  = mcpopenai.Model
	Usage                  = mcpopenai.Usage
	ChatStream             = mcpopenai.ChatStream
	ToolHandler            = mcpopenai.ToolHandler
	ToolFilter             = mcpopenai.ToolFilter
	ResponseObject         = mcpopenai.ResponseObject
	CreateResponseRequest  = mcpopenai.CreateResponseRequest
	APIError               = mcpopenai.APIError
)

// Re-export functions from mcp/openai
var (
	WithToolHandler                 = mcpopenai.WithToolHandler
	ToolHandlerFromContext          = mcpopenai.ToolHandlerFromContext
	NewChatStream                   = mcpopenai.NewChatStream
	BuildToolResultMessage          = mcpopenai.BuildToolResultMessage
	BuildAssistantToolCallMessage   = mcpopenai.BuildAssistantToolCallMessage
	NewStreamingToolCallAccumulator = mcpopenai.NewStreamingToolCallAccumulator
	NewMaxToolIterationsError       = mcpopenai.NewMaxToolIterationsError
	ExtractToolResult               = mcpopenai.ExtractToolResult
	MCPToolsToOpenAI                = mcpopenai.MCPToolsToOpenAI
	MCPToolsToOpenAIFiltered        = mcpopenai.MCPToolsToOpenAIFiltered
	ExecuteToolCalls                = mcpopenai.ExecuteToolCalls
	GenerateToolCallID              = mcpopenai.GenerateToolCallID
)

// MCPServer interface for MCP server operations
type MCPServer interface {
	ListTools() []mcp.MCPTool
	CallTool(ctx context.Context, name string, args map[string]any) (*mcp.ToolResponse, error)
}

// Client represents an OpenAI API client
type Client struct {
	restClient *rest.RESTClient
	mcpServer  MCPServer // Optional - if nil, tool calls won't be processed
}

// Config holds configuration for the OpenAI client
type Config struct {
	APIKey  string
	BaseURL string
	Timeout time.Duration
}

// New creates a new OpenAI client
func New(config Config, mcpServer MCPServer) (*Client, error) {
	if config.BaseURL == "" {
		config.BaseURL = "https://api.openai.com/v1/"
	}

	// Ensure BaseURL has a trailing slash for proper URL resolution
	if !strings.HasSuffix(config.BaseURL, "/") {
		config.BaseURL = config.BaseURL + "/"
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

// ChatCompletion performs a non-streaming chat completion with automatic tool processing
func (c *Client) ChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	currentMessages := req.Messages

	// Add tools if MCP server is available
	if c.mcpServer != nil {
		req.Tools = MCPToolsToOpenAI(c.mcpServer.ListTools())
	}

	toolHandler := ToolHandlerFromContext(ctx)

	// Multi-turn tool processing loop if MCP server is available
	for iteration := 0; iteration < MAX_TOOL_CALL_ITERATIONS; iteration++ {
		req.Messages = currentMessages

		response, err := c.nonStreamingChatCompletion(ctx, req)
		if err != nil {
			return nil, err
		}

		// If no MCP server, no tool calls, or no choices, we're done
		if c.mcpServer == nil || len(response.Choices) == 0 || len(response.Choices[0].Message.ToolCalls) == 0 {
			return response, nil
		}

		// Process tool calls
		message := response.Choices[0].Message
		toolCalls := message.ToolCalls

		// Notify handler of tool calls
		if toolHandler != nil {
			for _, toolCall := range toolCalls {
				if err := toolHandler.OnToolCall(toolCall); err != nil {
					return nil, fmt.Errorf("tool handler error: %w", err)
				}
			}
		}

		// Add assistant message to conversation
		currentMessages = append(currentMessages, BuildAssistantToolCallMessage(
			message.GetContentAsString(),
			toolCalls,
		))

		// Execute tools using shared helper
		toolResults, err := ExecuteToolCalls(toolCalls, func(name string, args map[string]any) (string, error) {
			response, err := c.mcpServer.CallTool(ctx, name, args)
			if err != nil {
				return "", err
			}
			result, _ := ExtractToolResult(response)
			return result, nil
		}, false)
		if err != nil {
			return nil, err
		}

		// Notify handler of tool results
		if toolHandler != nil {
			for i, toolCall := range toolCalls {
				if err := toolHandler.OnToolResult(toolCall.ID, toolCall.Function.Name, toolResults[i].Content.(string)); err != nil {
					return nil, fmt.Errorf("tool handler error: %w", err)
				}
			}
		}

		// Add tool results to conversation
		currentMessages = append(currentMessages, toolResults...)
	}

	return nil, NewMaxToolIterationsError(MAX_TOOL_CALL_ITERATIONS)
}

// StreamChatCompletion performs a streaming chat completion with automatic tool processing
// Returns a channel of pure OpenAI ChatCompletionResponse chunks
func (c *Client) StreamChatCompletion(ctx context.Context, req ChatCompletionRequest) *ChatStream {
	logger := log.WithGroup("openai")

	responseChan := make(chan ChatCompletionResponse, 50)
	errorChan := make(chan error, 1)

	go func() {
		defer close(responseChan)
		defer close(errorChan)

		// Add timeout context for the entire operation
		ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()

		currentMessages := req.Messages

		// Add tools if MCP server is available
		if c.mcpServer != nil {
			tools := MCPToolsToOpenAI(c.mcpServer.ListTools())
			if len(tools) > 0 {
				req.Tools = tools
			}
		}

		toolHandler := ToolHandlerFromContext(ctx)

		// Multi-turn tool processing loop if MCP server is available
		for iteration := 0; iteration < MAX_TOOL_CALL_ITERATIONS; iteration++ {
			req.Messages = currentMessages
			req.Stream = true

			// Stream single completion
			finalResponse, err := c.streamSingleCompletion(ctx, req, responseChan)
			if err != nil {
				logger.Error("stream single completion failed", "error", err, "iteration", iteration)
				errorChan <- err
				return
			}

			// If no MCP server, no tool calls, or no choices, we're done
			if c.mcpServer == nil || finalResponse == nil || len(finalResponse.Choices) == 0 || len(finalResponse.Choices[0].Message.ToolCalls) == 0 {
				return
			}

			// Process tool calls
			message := finalResponse.Choices[0].Message
			toolCalls := message.ToolCalls

			// Notify handler of tool calls
			if toolHandler != nil {
				for _, toolCall := range toolCalls {
					if err := toolHandler.OnToolCall(toolCall); err != nil {
						logger.Error("tool handler OnToolCall failed", "error", err, "tool_name", toolCall.Function.Name)
						errorChan <- fmt.Errorf("tool handler error: %w", err)
						return
					}
				}
			}

			// Add assistant message to conversation
			currentMessages = append(currentMessages, BuildAssistantToolCallMessage(
				message.GetContentAsString(),
				toolCalls,
			))

			// Execute tools using shared helper
			toolResults, err := ExecuteToolCalls(toolCalls, func(name string, args map[string]any) (string, error) {
				response, err := c.mcpServer.CallTool(ctx, name, args)
				if err != nil {
					return "", err
				}
				result, _ := ExtractToolResult(response)
				return result, nil
			}, false)
			if err != nil {
				logger.Error("tool execution failed", "error", err)
				errorChan <- err
				return
			}

			// Notify handler of tool results
			if toolHandler != nil {
				for i, toolCall := range toolCalls {
					if err := toolHandler.OnToolResult(toolCall.ID, toolCall.Function.Name, toolResults[i].Content.(string)); err != nil {
						logger.Error("tool handler OnToolResult failed", "error", err, "tool_name", toolCall.Function.Name)
						errorChan <- fmt.Errorf("tool handler error: %w", err)
						return
					}
				}
			}

			// Add tool results to conversation
			currentMessages = append(currentMessages, toolResults...)
		}

		errorChan <- NewMaxToolIterationsError(MAX_TOOL_CALL_ITERATIONS)
	}()

	return NewChatStream(ctx, responseChan, errorChan)
}

// nonStreamingChatCompletion handles non-streaming chat completion
func (c *Client) nonStreamingChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	var response ChatCompletionResponse

	_, err := c.restClient.Post(ctx, "chat/completions", req, &response, http.StatusOK)
	if err != nil {
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}

	// Ensure Choices is never nil for N8N compatibility
	if response.Choices == nil {
		response.Choices = []Choice{}
	}

	return &response, nil
}

// streamSingleCompletion handles a single streaming completion
func (c *Client) streamSingleCompletion(ctx context.Context, req ChatCompletionRequest, responseChan chan<- ChatCompletionResponse) (*ChatCompletionResponse, error) {
	var finalResponse *ChatCompletionResponse
	var assistantContent strings.Builder

	// Use streaming accumulator for tool calls
	toolAccumulator := NewStreamingToolCallAccumulator()

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

			// Process the chunk for internal state first
			shouldStop, err := c.processStreamChunk(response, toolAccumulator, &assistantContent)
			if err != nil {
				return true, err
			}

			// Only send response to client if:
			// 1. No MCP server (client handles tool calls), OR
			// 2. MCP server exists but this chunk has no tool calls (just content)
			shouldSendToClient := c.mcpServer == nil ||
				(len(response.Choices) > 0 && len(response.Choices[0].Delta.ToolCalls) == 0)

			if shouldSendToClient {
				// Send the response to the channel
				select {
				case responseChan <- *response:
				case <-ctx.Done():
					return true, ctx.Err()
				}
			}

			// Check if we should stop streaming
			if shouldStop {
				// Finalize tool calls using accumulator
				toolCalls := toolAccumulator.Finalize()

				// Create final response
				finishReason := ""
				if len(response.Choices) > 0 {
					finishReason = response.Choices[0].FinishReason
				}

				finalMessage := BuildAssistantToolCallMessage(assistantContent.String(), toolCalls)

				finalResponse = &ChatCompletionResponse{
					ID:      response.ID,
					Object:  response.Object,
					Created: response.Created,
					Model:   response.Model,
					Choices: []Choice{
						{
							Message:      finalMessage,
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

	return finalResponse, nil
}

// processStreamChunk processes a single streaming chunk
func (c *Client) processStreamChunk(response *ChatCompletionResponse, toolAccumulator *mcpopenai.StreamingToolCallAccumulator, assistantContent *strings.Builder) (bool, error) {
	if len(response.Choices) == 0 {
		return false, nil
	}

	choice := response.Choices[0]

	// Handle tool calls using the accumulator with ID callback
	if len(choice.Delta.ToolCalls) > 0 && c.mcpServer != nil {
		// Use callback to update response with generated IDs
		toolAccumulator.ProcessDeltaWithIDCallback(choice.Delta, func(index int, id string) {
			// Update the response chunk with the generated ID so it's forwarded to clients
			for i := range choice.Delta.ToolCalls {
				if choice.Delta.ToolCalls[i].Index == index {
					response.Choices[0].Delta.ToolCalls[i].ID = id
					break
				}
			}
		})
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

// GetMCPServer returns the MCP server instance
func (c *Client) GetMCPServer() MCPServer {
	return c.mcpServer
}


