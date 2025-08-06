package chat

import (
	"strings"
	"testing"
)

func TestPseudoBufferToolCallDetection(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string // Expected content chunks sent
	}{
		{
			name:     "simple content",
			input:    "Hello world",
			expected: []string{"Hello world"},
		},
		{
			name:     "tool call at start",
			input:    "```tool_call\n{\"name\":\"test\"}\n```",
			expected: []string{}, // Tool call should be processed, not sent as content
		},
		{
			name:     "content then tool call",
			input:    "Hello ```tool_call\n{\"name\":\"test\"}\n```",
			expected: []string{"Hello "},
		},
		{
			name:     "tool call then content",
			input:    "```tool_call\n{\"name\":\"test\"}\n``` world",
			expected: []string{" world"},
		},
		{
			name:     "partial marker",
			input:    "Hello ```too",
			expected: []string{"Hello ```too"}, // Should buffer partial marker
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffer := newPseudoBuffer()
			var contentChunks []string
			
			// Simulate processing the input character by character
			for _, char := range tt.input {
				buffer.buffer.WriteString(string(char))
				bufferContent := buffer.buffer.String()
				
				if !buffer.inToolCall {
					if idx := strings.Index(bufferContent, toolCallMarker); idx != -1 {
						beforeToolCall := bufferContent[:idx]
						if beforeToolCall != "" {
							contentChunks = append(contentChunks, beforeToolCall)
						}
						buffer.inToolCall = true
						buffer.toolCallData.Reset()
						buffer.buffer.Reset()
						buffer.buffer.WriteString(bufferContent[idx+len(toolCallMarker):])
					} else if len(bufferContent) > buffer.windowSize {
						toSend := bufferContent[:len(bufferContent)-len(toolCallMarker)]
						if toSend != "" {
							contentChunks = append(contentChunks, toSend)
						}
						buffer.buffer.Reset()
						buffer.buffer.WriteString(bufferContent[len(bufferContent)-len(toolCallMarker):])
					}
				} else {
					if idx := strings.Index(bufferContent, toolCallEnd); idx != -1 {
						buffer.toolCallData.WriteString(bufferContent[:idx])
						// Tool call would be processed here
						buffer.inToolCall = false
						buffer.toolCallData.Reset()
						buffer.buffer.Reset()
						
						remaining := bufferContent[idx+len(toolCallEnd):]
						if remaining != "" {
							buffer.buffer.WriteString(remaining)
							contentChunks = append(contentChunks, remaining)
						}
					} else {
						buffer.toolCallData.WriteString(string(char))
					}
				}
			}
			
			// Flush remaining buffer
			if buffer.buffer.Len() > 0 {
				contentChunks = append(contentChunks, buffer.buffer.String())
			}
			
			// Join all content chunks
			result := strings.Join(contentChunks, "")
			expectedResult := strings.Join(tt.expected, "")
			
			if result != expectedResult {
				t.Errorf("Expected content %q, got %q", expectedResult, result)
			}
		})
	}
}