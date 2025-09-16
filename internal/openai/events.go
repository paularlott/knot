package openai

import "context"

// ToolHandler receives tool-related events during streaming
type ToolHandler interface {
	OnToolCall(toolCall ToolCall) error
	OnToolResult(toolCallID, toolName, result string) error
}

// private key type to avoid collisions
type toolHandlerKey struct{}

// WithToolHandler attaches a per-request ToolHandler to the context.
func WithToolHandler(ctx context.Context, h ToolHandler) context.Context {
	// ToolHandler is assumed to be defined in this package
	return context.WithValue(ctx, toolHandlerKey{}, h)
}

// toolHandlerFromContext retrieves a per-request ToolHandler (or nil).
func toolHandlerFromContext(ctx context.Context) ToolHandler {
	if v := ctx.Value(toolHandlerKey{}); v != nil {
		if th, ok := v.(ToolHandler); ok {
			return th
		}
	}
	return nil
}
