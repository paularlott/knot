package openai

import (
	"testing"
)

func TestCompletionAccumulator_AddChunk_Content(t *testing.T) {
	acc := CompletionAccumulator{}

	// Add content chunks
	acc.AddChunk(ChatCompletionResponse{
		Choices: []Choice{{Index: 0, Delta: Delta{Content: "Hello"}}},
	})
	acc.AddChunk(ChatCompletionResponse{
		Choices: []Choice{{Index: 0, Delta: Delta{Content: " world"}}},
	})
	acc.AddChunk(ChatCompletionResponse{
		Choices: []Choice{{Index: 0, FinishReason: "stop"}},
	})

	if got := acc.Choices[0].Message.GetContentAsString(); got != "Hello world" {
		t.Errorf("expected 'Hello world', got '%s'", got)
	}

	if content, ok := acc.FinishedContent(); !ok || content != "Hello world" {
		t.Errorf("FinishedContent() = %s, %v; want 'Hello world', true", content, ok)
	}
}

func TestCompletionAccumulator_AddChunk_ToolCall(t *testing.T) {
	acc := CompletionAccumulator{}

	// Add tool call chunks
	acc.AddChunk(ChatCompletionResponse{
		Choices: []Choice{{Index: 0, Delta: Delta{
			ToolCalls: []DeltaToolCall{{Index: 0, ID: "call_123", Type: "function", Function: DeltaFunction{Name: "test_func"}}},
		}}},
	})
	acc.AddChunk(ChatCompletionResponse{
		Choices: []Choice{{Index: 0, Delta: Delta{
			ToolCalls: []DeltaToolCall{{Index: 0, Function: DeltaFunction{Arguments: `{"arg1"`}}},
		}}},
	})
	acc.AddChunk(ChatCompletionResponse{
		Choices: []Choice{{Index: 0, Delta: Delta{
			ToolCalls: []DeltaToolCall{{Index: 0, Function: DeltaFunction{Arguments: `:"value"}`}}},
		}}},
	})
	acc.AddChunk(ChatCompletionResponse{
		Choices: []Choice{{Index: 0, FinishReason: "tool_calls"}},
	})

	toolCall, ok := acc.FinishedToolCall()
	if !ok {
		t.Fatal("FinishedToolCall() should return true")
	}

	if toolCall.ID != "call_123" || toolCall.Function.Name != "test_func" {
		t.Errorf("unexpected tool call: %+v", toolCall)
	}

	// Check accumulated arguments
	if rawArgs, ok := toolCall.Function.Arguments["_raw"].(string); !ok || rawArgs != `{"arg1":"value"}` {
		t.Errorf("expected raw arguments '{\"arg1\":\"value\"}', got %v", toolCall.Function.Arguments)
	}
}

func TestCompletionAccumulator_AddChunk_Refusal(t *testing.T) {
	acc := CompletionAccumulator{}

	acc.AddChunk(ChatCompletionResponse{
		Choices: []Choice{{Index: 0, FinishReason: "content_filter"}},
	})

	refusal, ok := acc.FinishedRefusal()
	if !ok || refusal != "Content filtered" {
		t.Errorf("FinishedRefusal() = %s, %v; want 'Content filtered', true", refusal, ok)
	}
}

func TestCompletionAccumulator_JustFinished_NotFinished(t *testing.T) {
	acc := CompletionAccumulator{}
	acc.AddChunk(ChatCompletionResponse{
		Choices: []Choice{{Index: 0, Delta: Delta{Content: "Hello"}}},
	})

	if _, ok := acc.FinishedContent(); ok {
		t.Error("FinishedContent() should return false when not finished")
	}

	if _, ok := acc.FinishedToolCall(); ok {
		t.Error("FinishedToolCall() should return false when not finished")
	}

	if _, ok := acc.FinishedRefusal(); ok {
		t.Error("FinishedRefusal() should return false when not finished")
	}
}
