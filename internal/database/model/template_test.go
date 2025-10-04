package model

import (
	"testing"
)

func TestNewTemplate(t *testing.T) {
	groups := []string{"group1", "group2"}
	zones := []string{"zone1"}
	schedule := []TemplateScheduleDays{
		{Enabled: true, From: "9:00am", To: "5:00pm"},
	}
	customFields := []TemplateCustomField{
		{Name: "field1", Description: "desc1"},
	}

	template := NewTemplate(
		"test-template",
		"test description",
		"job-spec",
		"volumes-spec",
		"user-123",
		groups,
		PlatformDocker,
		true,
		false,
		true,
		true,
		false,
		100,
		200,
		true,
		&schedule,
		zones,
		true,
		true,
		8,
		"hour",
		"icon.png",
		customFields,
	)

	if template.Id == "" {
		t.Error("Template ID should not be empty")
	}
	if template.Name != "test-template" {
		t.Errorf("Expected name 'test-template', got '%s'", template.Name)
	}
	if template.Platform != PlatformDocker {
		t.Errorf("Expected platform '%s', got '%s'", PlatformDocker, template.Platform)
	}
	if !template.WithTerminal {
		t.Error("WithTerminal should be true")
	}
	if !template.WithCodeServer {
		t.Error("WithCodeServer should be true")
	}
	if !template.WithSSH {
		t.Error("WithSSH should be true")
	}
	if template.ComputeUnits != 100 {
		t.Errorf("Expected compute units 100, got %d", template.ComputeUnits)
	}
	if template.StorageUnits != 200 {
		t.Errorf("Expected storage units 200, got %d", template.StorageUnits)
	}
	if !template.ScheduleEnabled {
		t.Error("ScheduleEnabled should be true")
	}
	if !template.AutoStart {
		t.Error("AutoStart should be true")
	}
	if template.MaxUptime != 8 {
		t.Errorf("Expected max uptime 8, got %d", template.MaxUptime)
	}
	if template.MaxUptimeUnit != "hour" {
		t.Errorf("Expected max uptime unit 'hour', got '%s'", template.MaxUptimeUnit)
	}
	if template.Hash == "" {
		t.Error("Hash should be generated")
	}
	if len(template.CustomFields) != 1 {
		t.Errorf("Expected 1 custom field, got %d", len(template.CustomFields))
	}
}

func TestUpdateHash(t *testing.T) {
	template := &Template{
		Job:              "job-spec",
		Volumes:          "volumes-spec",
		Platform:         PlatformDocker,
		WithTerminal:     true,
		WithVSCodeTunnel: false,
		WithCodeServer:   true,
		WithSSH:          true,
		WithRunCommand:   false,
	}

	template.UpdateHash()
	hash1 := template.Hash

	if hash1 == "" {
		t.Error("Hash should be generated")
	}

	// Change something and verify hash changes
	template.Job = "different-job-spec"
	template.UpdateHash()
	hash2 := template.Hash

	if hash1 == hash2 {
		t.Error("Hash should change when template content changes")
	}
}

func TestIsManual(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		expected bool
	}{
		{"manual platform", PlatformManual, true},
		{"docker platform", PlatformDocker, false},
		{"nomad platform", PlatformNomad, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			template := &Template{Platform: tt.platform}
			result := template.IsManual()
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsLocalContainer(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		expected bool
	}{
		{"docker platform", PlatformDocker, true},
		{"podman platform", PlatformPodman, true},
		{"apple platform", PlatformApple, true},
		{"container platform", PlatformContainer, true},
		{"nomad platform", PlatformNomad, false},
		{"manual platform", PlatformManual, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			template := &Template{Platform: tt.platform}
			result := template.IsLocalContainer()
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsValidForZone(t *testing.T) {
	tests := []struct {
		name     string
		zones    []string
		testZone string
		expected bool
	}{
		{
			name:     "no zones specified - valid for all",
			zones:    []string{},
			testZone: "zone1",
			expected: true,
		},
		{
			name:     "zone explicitly allowed",
			zones:    []string{"zone1", "zone2"},
			testZone: "zone1",
			expected: true,
		},
		{
			name:     "zone not in list",
			zones:    []string{"zone1", "zone2"},
			testZone: "zone3",
			expected: false,
		},
		{
			name:     "zone explicitly excluded",
			zones:    []string{"!zone1", "zone2"},
			testZone: "zone1",
			expected: false,
		},
		{
			name:     "zone not excluded",
			zones:    []string{"!zone1", "zone2"},
			testZone: "zone2",
			expected: true,
		},
		{
			name:     "zone not in exclusion list but not explicitly allowed",
			zones:    []string{"!zone1"},
			testZone: "zone2",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			template := &Template{Zones: tt.zones}
			result := template.IsValidForZone(tt.testZone)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}
