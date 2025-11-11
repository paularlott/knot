package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/mcp"
)

func TestSkillsWithoutPath(t *testing.T) {
	// Set up a test config without skills path
	cfg := &config.ServerConfig{
		SkillsPath: "",
	}
	config.SetServerConfig(cfg)

	// Create a mock request for listing skills
	req := &mcp.ToolRequest{}

	// Call the skills function
	ctx := context.Background()
	response, err := skills(ctx, req)
	if err != nil {
		t.Fatalf("skills returned error when it should return empty list: %v", err)
	}

	// Parse the response
	var result map[string]interface{}
	err = json.Unmarshal([]byte(response.Content[0].Text), &result)
	if err != nil {
		t.Fatalf("Failed to parse response JSON: %v", err)
	}

	// Verify the response structure
	if result["action"] != "list" {
		t.Errorf("Expected action to be 'list', got %v", result["action"])
	}

	if result["count"] != float64(2) {
		t.Errorf("Expected count to be 2, got %v", result["count"])
	}

	skills, ok := result["skills"].([]interface{})
	if !ok {
		t.Errorf("Expected skills to be an array, got %T", result["skills"])
	}

	if len(skills) != 2 {
		t.Errorf("Expected 2 internal specs, got %d items", len(skills))
	}

	if result["message"] != "Skills path not configured - showing built-in specs only" {
		t.Errorf("Expected message about path not configured, got %v", result["message"])
	}
}

// Note: Testing specific file requests would require mocking the mcp.ToolRequest
// which is complex. The main functionality (empty list when path not configured)
// is tested above.
