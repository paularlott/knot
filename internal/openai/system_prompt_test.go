package openai

import (
	"testing"
)

func TestReplaceSystemPrompt(t *testing.T) {
	t.Run("no system prompt", func(t *testing.T) {
		service := NewService(nil, "")
		userMsg := Message{Role: "user"}
		userMsg.SetContentAsString("Hello")
		
		messages := []Message{userMsg}
		result := service.replaceSystemPrompt(messages)
		
		if len(result) != 1 {
			t.Errorf("Expected 1 message, got %d", len(result))
		}
		
		if result[0].Role != "user" {
			t.Errorf("Expected user message, got %s", result[0].Role)
		}
	})

	t.Run("add system prompt", func(t *testing.T) {
		service := NewService(nil, "You are a helpful assistant")
		userMsg := Message{Role: "user"}
		userMsg.SetContentAsString("Hello")
		
		messages := []Message{userMsg}
		result := service.replaceSystemPrompt(messages)
		
		if len(result) != 2 {
			t.Errorf("Expected 2 messages, got %d", len(result))
		}
		
		if result[0].Role != "system" {
			t.Errorf("Expected system message first, got %s", result[0].Role)
		}
		
		if result[0].GetContentAsString() != "You are a helpful assistant" {
			t.Errorf("Expected system prompt, got %s", result[0].GetContentAsString())
		}
		
		if result[1].Role != "user" {
			t.Errorf("Expected user message second, got %s", result[1].Role)
		}
	})

	t.Run("replace existing system prompt", func(t *testing.T) {
		service := NewService(nil, "New system prompt")
		systemMsg := Message{Role: "system"}
		systemMsg.SetContentAsString("Old system prompt")
		
		userMsg := Message{Role: "user"}
		userMsg.SetContentAsString("Hello")
		
		messages := []Message{systemMsg, userMsg}
		result := service.replaceSystemPrompt(messages)
		
		if len(result) != 2 {
			t.Errorf("Expected 2 messages, got %d", len(result))
		}
		
		if result[0].Role != "system" {
			t.Errorf("Expected system message first, got %s", result[0].Role)
		}
		
		if result[0].GetContentAsString() != "New system prompt" {
			t.Errorf("Expected new system prompt, got %s", result[0].GetContentAsString())
		}
		
		if result[1].Role != "user" {
			t.Errorf("Expected user message second, got %s", result[1].Role)
		}
	})

	t.Run("strip multiple system prompts", func(t *testing.T) {
		service := NewService(nil, "Our system prompt")
		systemMsg1 := Message{Role: "system"}
		systemMsg1.SetContentAsString("First system prompt")
		
		systemMsg2 := Message{Role: "system"}
		systemMsg2.SetContentAsString("Second system prompt")
		
		userMsg := Message{Role: "user"}
		userMsg.SetContentAsString("Hello")
		
		messages := []Message{systemMsg1, systemMsg2, userMsg}
		result := service.replaceSystemPrompt(messages)
		
		if len(result) != 2 {
			t.Errorf("Expected 2 messages, got %d", len(result))
		}
		
		if result[0].Role != "system" {
			t.Errorf("Expected system message first, got %s", result[0].Role)
		}
		
		if result[0].GetContentAsString() != "Our system prompt" {
			t.Errorf("Expected our system prompt, got %s", result[0].GetContentAsString())
		}
	})
}