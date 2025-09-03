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

const MAX_TOOL_CALL_ITERATIONS = 20

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

type ToolFilter func(toolName string) bool

// New creates a new OpenAI client
func New(config Config, mcpServer MCPServer) (*Client, error) {
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

// ChatCompletion performs a non-streaming chat completion with automatic tool processing
func (c *Client) ChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	currentMessages := req.Messages

	// Add tools if MCP server is available
	if c.mcpServer != nil {
		tools, err := c.getAvailableTools(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get available tools: %w", err)
		}
		req.Tools = tools
	}

	toolHandler := toolHandlerFromContext(ctx)

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
		currentMessages = append(currentMessages, message)

		// Execute tools and add results
		for _, toolCall := range toolCalls {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			result, err := c.executeToolCall(ctx, toolCall)
			if err != nil {
				result = fmt.Sprintf("Error: %s", err.Error())
			}

			// Notify handler of tool result
			if toolHandler != nil {
				if err := toolHandler.OnToolResult(toolCall.ID, toolCall.Function.Name, result); err != nil {
					return nil, fmt.Errorf("tool handler error: %w", err)
				}
			}

			// Add tool result to conversation
			toolResultMessage := Message{
				Role:       "tool",
				ToolCallID: toolCall.ID,
			}
			toolResultMessage.SetContentAsString(result)
			currentMessages = append(currentMessages, toolResultMessage)
		}
	}

	return nil, fmt.Errorf("maximum tool call iterations (%d) reached", MAX_TOOL_CALL_ITERATIONS)
}

// StreamChatCompletion performs a streaming chat completion with automatic tool processing
// Returns a channel of pure OpenAI ChatCompletionResponse chunks
func (c *Client) StreamChatCompletion(ctx context.Context, req ChatCompletionRequest) *ChatStream {
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
			tools, err := c.getAvailableTools(nil)
			if err != nil {
				errorChan <- fmt.Errorf("failed to get available tools: %w", err)
				return
			}
			if len(tools) > 0 {
				req.Tools = tools
				log.Debug().Int("tools_count", len(tools)).Msg("Added tools to streaming request")
			}
		}

		toolHandler := toolHandlerFromContext(ctx)

		// Multi-turn tool processing loop if MCP server is available
		for iteration := 0; iteration < MAX_TOOL_CALL_ITERATIONS; iteration++ {
			log.Debug().Int("iteration", iteration).Msg("Starting tool processing iteration")

			req.Messages = currentMessages
			req.Stream = true

			// Stream single completion
			finalResponse, err := c.streamSingleCompletion(ctx, req, responseChan)
			if err != nil {
				log.Error().Err(err).Int("iteration", iteration).Msg("Stream single completion failed")
				errorChan <- err
				return
			}

			// If no MCP server, no tool calls, or no choices, we're done
			if c.mcpServer == nil || len(finalResponse.Choices) == 0 || len(finalResponse.Choices[0].Message.ToolCalls) == 0 {
				log.Debug().
					Bool("has_mcp_server", c.mcpServer != nil).
					Int("choices_count", len(finalResponse.Choices)).
					Int("tool_calls_count", func() int {
						if len(finalResponse.Choices) > 0 {
							return len(finalResponse.Choices[0].Message.ToolCalls)
						}
						return 0
					}()).
					Msg("No tool calls to process, ending stream")
				return
			}

			// Process tool calls
			message := finalResponse.Choices[0].Message
			toolCalls := message.ToolCalls

			log.Debug().Int("tool_calls_count", len(toolCalls)).Int("iteration", iteration).Msg("Processing tool calls")

			// Notify handler of tool calls
			if toolHandler != nil {
				for _, toolCall := range toolCalls {
					if err := toolHandler.OnToolCall(toolCall); err != nil {
						log.Error().Err(err).Str("tool_name", toolCall.Function.Name).Msg("Tool handler OnToolCall failed")
						errorChan <- fmt.Errorf("tool handler error: %w", err)
						return
					}
				}
			}

			// Add assistant message to conversation
			currentMessages = append(currentMessages, message)

			// Execute tools and add results
			log.Debug().Int("tool_calls_to_execute", len(toolCalls)).Msg("Starting tool execution loop")

			for i, toolCall := range toolCalls {
				select {
				case <-ctx.Done():
					log.Debug().
						Err(ctx.Err()).
						Str("context_error", ctx.Err().Error()).
						Msg("Context cancelled during tool execution")
					errorChan <- ctx.Err()
					return
				default:
				}

				log.Debug().
					Int("tool_index", i).
					Str("tool_name", toolCall.Function.Name).
					Str("tool_id", toolCall.ID).
					Msg("About to execute tool call")

				result, err := c.executeToolCall(ctx, toolCall)
				if err != nil {
					log.Error().Err(err).Str("tool_name", toolCall.Function.Name).Msg("Tool execution failed")
					result = fmt.Sprintf("Error: %s", err.Error())
				} else {
					log.Debug().Str("tool_name", toolCall.Function.Name).Str("result", result).Msg("Tool execution succeeded")
				}

				log.Debug().
					Int("tool_index", i).
					Str("tool_name", toolCall.Function.Name).
					Msg("About to notify tool handler of result")

				// Notify handler of tool result
				if toolHandler != nil {
					if err := toolHandler.OnToolResult(toolCall.ID, toolCall.Function.Name, result); err != nil {
						log.Error().Err(err).Str("tool_name", toolCall.Function.Name).Msg("Tool handler OnToolResult failed")
						errorChan <- fmt.Errorf("tool handler error: %w", err)
						return
					}
				}

				log.Debug().
					Int("tool_index", i).
					Str("tool_name", toolCall.Function.Name).
					Msg("About to add tool result to conversation")

				// Add tool result to conversation
				toolResultMessage := Message{
					Role:       "tool",
					ToolCallID: toolCall.ID,
				}
				toolResultMessage.SetContentAsString(result)
				currentMessages = append(currentMessages, toolResultMessage)

				log.Debug().
					Int("tool_index", i).
					Str("tool_name", toolCall.Function.Name).
					Msg("Completed processing tool call")
			}

			log.Debug().Int("iteration", iteration).Int("messages_count", len(currentMessages)).Msg("Completed tool execution, continuing to next iteration")
		}

		errorChan <- fmt.Errorf("maximum tool call iterations (%d) reached", MAX_TOOL_CALL_ITERATIONS)
	}()

	return &ChatStream{
		responseChan: responseChan,
		errorChan:    errorChan,
		ctx:          ctx,
		current:      nil,
		err:          nil,
		done:         false,
	}
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

// streamSingleCompletion handles a single streaming completion
func (c *Client) streamSingleCompletion(ctx context.Context, req ChatCompletionRequest, responseChan chan<- ChatCompletionResponse) (*ChatCompletionResponse, error) {
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

			// Send the raw OpenAI response to the channel
			select {
			case responseChan <- *response:
			case <-ctx.Done():
				return true, ctx.Err()
			}

			// Process the chunk for internal state
			shouldStop, err := c.processStreamChunk(response, toolCallBuffer, argumentsBuffer, &assistantContent)
			if err != nil {
				return true, err
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

				finalMessage := Message{
					Role:      "assistant",
					ToolCalls: toolCalls,
				}
				finalMessage.SetContentAsString(assistantContent.String())

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
func (c *Client) processStreamChunk(response *ChatCompletionResponse, toolCallBuffer map[int]*ToolCall, argumentsBuffer map[int]string, assistantContent *strings.Builder) (bool, error) {
	if len(response.Choices) == 0 {
		return false, nil
	}

	choice := response.Choices[0]

	// Handle tool calls only if MCP server is available
	if c.mcpServer != nil && len(choice.Delta.ToolCalls) > 0 {
		log.Debug().Int("tool_calls_count", len(choice.Delta.ToolCalls)).Msg("Processing tool calls in stream chunk")

		for _, deltaCall := range choice.Delta.ToolCalls {
			index := deltaCall.Index
			if toolCallBuffer[index] == nil {
				toolCallBuffer[index] = &ToolCall{
					Index:    index,
					Function: ToolCallFunction{Arguments: make(map[string]any)},
				}
				argumentsBuffer[index] = ""
				log.Debug().Int("index", index).Msg("Created new tool call buffer entry")
			}
			if deltaCall.ID != "" {
				toolCallBuffer[index].ID = deltaCall.ID
				log.Debug().Int("index", index).Str("id", deltaCall.ID).Msg("Set tool call ID")
			}
			if deltaCall.Type != "" {
				toolCallBuffer[index].Type = deltaCall.Type
				log.Debug().Int("index", index).Str("type", deltaCall.Type).Msg("Set tool call type")
			}
			if deltaCall.Function.Name != "" {
				toolCallBuffer[index].Function.Name = deltaCall.Function.Name
				log.Debug().Int("index", index).Str("name", deltaCall.Function.Name).Msg("Set tool call function name")
			}
			if deltaCall.Function.Arguments != "" {
				argumentsBuffer[index] += deltaCall.Function.Arguments
				log.Debug().Int("index", index).Str("args_chunk", deltaCall.Function.Arguments).Msg("Added arguments chunk")
			}
		}
	}

	// Handle content
	if choice.Delta.Content != "" {
		assistantContent.WriteString(choice.Delta.Content)
	}

	// Check for finish reason
	if choice.FinishReason != "" {
		log.Debug().Str("finish_reason", choice.FinishReason).Msg("Stream chunk finished")
		return true, nil
	}

	return false, nil
}

// finalizeToolCalls converts buffered tool calls to final format
func (c *Client) finalizeToolCalls(toolCallBuffer map[int]*ToolCall, argumentsBuffer map[int]string) []ToolCall {
	if c.mcpServer == nil || len(toolCallBuffer) == 0 {
		log.Debug().Bool("has_mcp_server", c.mcpServer != nil).Int("buffer_size", len(toolCallBuffer)).Msg("No tool calls to finalize")
		return nil
	}

	log.Debug().Int("buffer_size", len(toolCallBuffer)).Msg("Finalizing tool calls")

	toolCalls := make([]ToolCall, 0, len(toolCallBuffer))

	for index, toolCall := range toolCallBuffer {
		if toolCall == nil || toolCall.Function.Name == "" {
			continue
		}

		log.Debug().
			Int("index", index).
			Str("name", toolCall.Function.Name).
			Str("id", toolCall.ID).
			Str("raw_args", argumentsBuffer[index]).
			Msg("Processing tool call")

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

// executeToolCall executes a single tool call using the MCP server
func (c *Client) executeToolCall(ctx context.Context, toolCall ToolCall) (string, error) {
	if c.mcpServer == nil {
		return "", fmt.Errorf("MCP server not configured")
	}

	log.Debug().
		Str("tool_name", toolCall.Function.Name).
		Str("tool_id", toolCall.ID).
		Interface("arguments", toolCall.Function.Arguments).
		Msg("Executing tool call")

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
