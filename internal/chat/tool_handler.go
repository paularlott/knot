package chat

import (
	"github.com/paularlott/knot/internal/openai"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/rs/zerolog/log"
)

// WebChatToolHandler handles tool events for the web chat
type WebChatToolHandler struct {
	streamWriter *rest.StreamWriter
}

func NewWebChatToolHandler(streamWriter *rest.StreamWriter) *WebChatToolHandler {
	return &WebChatToolHandler{
		streamWriter: streamWriter,
	}
}

func (h *WebChatToolHandler) OnToolCall(toolCall openai.ToolCall) error {
	// Check if stream writer is still open before writing
	if h.streamWriter == nil {
		log.Debug().Str("tool_name", toolCall.Function.Name).Msg("Stream writer is nil, skipping tool call notification")
		return nil
	}

	// Try to write, but don't fail if stream is closed
	err := h.streamWriter.WriteChunk(SSEEvent{
		Type: "tool_calls",
		Data: []openai.ToolCall{toolCall},
	})

	if err != nil {
		log.Debug().Err(err).Str("tool_name", toolCall.Function.Name).Msg("Failed to write tool call to stream (stream likely closed)")
		return nil // Don't propagate the error - just log it
	}

	return nil
}

func (h *WebChatToolHandler) OnToolResult(toolCallID, toolName, result string) error {
	// Check if stream writer is still open before writing
	if h.streamWriter == nil {
		log.Debug().Str("tool_name", toolName).Msg("Stream writer is nil, skipping tool result notification")
		return nil
	}

	// Try to write, but don't fail if stream is closed
	err := h.streamWriter.WriteChunk(SSEEvent{
		Type: "tool_result",
		Data: map[string]interface{}{
			"tool_call_id": toolCallID,
			"tool_name":    toolName,
			"result":       result,
		},
	})

	if err != nil {
		log.Debug().Err(err).Str("tool_name", toolName).Msg("Failed to write tool result to stream (stream likely closed)")
		return nil // Don't propagate the error - just log it
	}

	return nil
}
