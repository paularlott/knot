package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/mcp"
)

func TestRecipesWithoutPath(t *testing.T) {
	// Set up a test config without recipes path
	cfg := &config.ServerConfig{
		RecipesPath: "",
	}
	config.SetServerConfig(cfg)

	// Create a mock request for listing recipes
	req := &mcp.ToolRequest{}

	// Call the recipes function
	ctx := context.Background()
	response, err := recipes(ctx, req)
	if err != nil {
		t.Fatalf("recipes returned error when it should return empty list: %v", err)
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

	if result["count"] != float64(3) {
		t.Errorf("Expected count to be 3, got %v", result["count"])
	}

	recipes, ok := result["recipes"].([]interface{})
	if !ok {
		t.Errorf("Expected recipes to be an array, got %T", result["recipes"])
	}

	if len(recipes) != 3 {
		t.Errorf("Expected 3 internal specs, got %d items", len(recipes))
	}

	if result["message"] != "Recipes path not configured - showing built-in specs only" {
		t.Errorf("Expected message about path not configured, got %v", result["message"])
	}
}

// Note: Testing specific file requests would require mocking the mcp.ToolRequest
// which is complex. The main functionality (empty list when path not configured)
// is tested above.
