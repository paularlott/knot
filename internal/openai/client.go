package openai

import (
	mcpopenai "github.com/paularlott/mcp/ai/openai"
)

// Re-export types from mcp/openai for convenience within internal/openai package
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
	ResponseObject         = mcpopenai.ResponseObject
	CreateResponseRequest  = mcpopenai.CreateResponseRequest
	APIError               = mcpopenai.APIError
	Client                 = mcpopenai.Client
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
