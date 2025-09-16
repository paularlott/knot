package service

import (
	"testing"

	"github.com/paularlott/knot/internal/config"
)

func TestIconService(t *testing.T) {
	// Set up a test config
	cfg := &config.ServerConfig{
		UI: config.UIConfig{
			EnableBuiltinIcons: true,
			Icons:              []string{},
		},
	}
	config.SetServerConfig(cfg)

	// Get the icon service
	iconService := GetIconService()

	// Test that we get icons
	icons := iconService.GetIcons()
	if len(icons) == 0 {
		t.Error("Expected to get built-in icons, but got none")
	}

	// Test that we have some expected icons
	foundDocker := false
	foundGo := false
	for _, icon := range icons {
		if icon.Description == "Docker" {
			foundDocker = true
			if icon.URL != "/icons/docker.svg" {
				t.Errorf("Expected Docker icon URL to be '/icons/docker.svg', got '%s'", icon.URL)
			}
			if icon.Source != "built-in" {
				t.Errorf("Expected Docker icon source to be 'built-in', got '%s'", icon.Source)
			}
		}
		if icon.Description == "Go" {
			foundGo = true
		}
	}

	if !foundDocker {
		t.Error("Expected to find Docker icon in built-in icons")
	}
	if !foundGo {
		t.Error("Expected to find Go icon in built-in icons")
	}

	// Test that icons are sorted
	for i := 1; i < len(icons); i++ {
		if icons[i-1].Description > icons[i].Description {
			t.Errorf("Icons are not sorted: '%s' comes after '%s'", icons[i-1].Description, icons[i].Description)
			break
		}
	}
}

func TestIconServiceDisabledBuiltins(t *testing.T) {
	// Set up a test config with built-ins disabled
	cfg := &config.ServerConfig{
		UI: config.UIConfig{
			EnableBuiltinIcons: false,
			Icons:              []string{},
		},
	}
	config.SetServerConfig(cfg)

	// Create a new icon service instance for this test
	iconService := &IconService{}
	iconService.loadIcons()

	// Test that we get no icons when built-ins are disabled
	icons := iconService.GetIcons()
	if len(icons) != 0 {
		t.Errorf("Expected no icons when built-ins are disabled, but got %d", len(icons))
	}
}
