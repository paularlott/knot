package openai

import (
	"context"

	"github.com/paularlott/mcp"
	mcpopenai "github.com/paularlott/mcp/openai"
)

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
	ListToolsWithContext(ctx context.Context) []mcp.MCPTool
	CallTool(ctx context.Context, name string, args map[string]any) (*mcp.ToolResponse, error)
}

// Client is now an alias to mcp/openai.Client
type Client = mcpopenai.Client

// Config holds configuration for the OpenAI client
type Config struct {
	APIKey  string
	BaseURL string
}

// New creates a new OpenAI client using mcp/openai.Client
// Maintains backward compatibility while using the shared HTTP pool
func New(config Config, mcpServer MCPServer) (*Client, error) {
	// Convert to new config format
	newConfig := mcpopenai.Config{
		APIKey:      config.APIKey,
		BaseURL:     config.BaseURL,
		LocalServer: mcpServer,
	}

	return mcpopenai.New(newConfig)
}
