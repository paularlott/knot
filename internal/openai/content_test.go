package openai

import (
	"encoding/json"
	"testing"
)

func TestMessage_ContentHandling(t *testing.T) {
	// Test string content
	t.Run("string content", func(t *testing.T) {
		msg := Message{Role: "user"}
		msg.SetContentAsString("Hello world")
		
		if content := msg.GetContentAsString(); content != "Hello world" {
			t.Errorf("Expected 'Hello world', got '%s'", content)
		}
	})

	// Test array content unmarshaling
	t.Run("array content unmarshaling", func(t *testing.T) {
		jsonData := `{
			"role": "user",
			"content": [
				{"type": "text", "text": "Hello "},
				{"type": "text", "text": "world"}
			]
		}`
		
		var msg Message
		if err := json.Unmarshal([]byte(jsonData), &msg); err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}
		
		if content := msg.GetContentAsString(); content != "Hello world" {
			t.Errorf("Expected 'Hello world', got '%s'", content)
		}
	})

	// Test string content unmarshaling
	t.Run("string content unmarshaling", func(t *testing.T) {
		jsonData := `{
			"role": "user",
			"content": "Simple string content"
		}`
		
		var msg Message
		if err := json.Unmarshal([]byte(jsonData), &msg); err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}
		
		if content := msg.GetContentAsString(); content != "Simple string content" {
			t.Errorf("Expected 'Simple string content', got '%s'", content)
		}
	})

	// Test empty content
	t.Run("empty content", func(t *testing.T) {
		msg := Message{Role: "user"}
		
		if content := msg.GetContentAsString(); content != "" {
			t.Errorf("Expected empty string, got '%s'", content)
		}
	})
}