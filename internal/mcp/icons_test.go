package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/mcp"
)

func TestListIcons(t *testing.T) {
	// Set up a test config
	cfg := &config.ServerConfig{
		UI: config.UIConfig{
			EnableBuiltinIcons: true,
			Icons:              []string{},
		},
	}
	config.SetServerConfig(cfg)

	// Create a mock request
	req := &mcp.ToolRequest{}

	// Call the listIcons function
	ctx := context.Background()
	response, err := listIcons(ctx, req)
	if err != nil {
		t.Fatalf("listIcons returned error: %v", err)
	}

	// Parse the response
	var icons []service.Icon
	err = json.Unmarshal([]byte(response.Content[0].Text), &icons)
	if err != nil {
		t.Fatalf("Failed to parse response JSON: %v", err)
	}

	// Verify we got icons
	if len(icons) == 0 {
		t.Error("Expected to get icons, but got none")
	}

	// Verify structure of first icon
	if len(icons) > 0 {
		icon := icons[0]
		if icon.Description == "" {
			t.Error("Expected icon to have a description")
		}
		if icon.URL == "" {
			t.Error("Expected icon to have a URL")
		}
		if icon.Source == "" {
			t.Error("Expected icon to have a source")
		}
	}
}
