package apiclient

import (
	"testing"

	"github.com/paularlott/knot/internal/database/model"
	"gopkg.in/yaml.v3"
)

func TestTemplateExportRoundTrip(t *testing.T) {
	exp := &TemplateExport{
		Name:         "Ubuntu Desktop",
		Description:  "Base Ubuntu 26.04",
		Platform:     "nomad",
		IconURL:      "/icons/ubuntu.svg",
		Active:       true,
		ComputeUnits: 1,
		Features: TemplateExportFeatures{
			WithTerminal: true,
			WithSSH:      true,
		},
		Groups: []string{"developers"},
		Zones:  []string{"zone1", "!zone2"},
		CustomFields: []TemplateExportCustomField{
			{Name: "branch", Description: "Git branch"},
		},
		StartupScript: "install-tools.sh",
		Job: `job "${{.space.name}}" {
  group "app" {
    task "app" {
      driver = "docker"
      config {
        image = "${{ .server.base_image_registry }}/knot-ubuntu:26.04"
      }
    }
  }
}
`,
		Volumes: "volumes:\n  - name: data\n    type: csi\n",
	}

	// Marshal to YAML.
	yamlText, err := exp.String()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Verify template variables survive.
	if !contains(yamlText, "${{.space.name}}") {
		t.Errorf("template variable lost in job:\n%s", yamlText)
	}
	if !contains(yamlText, "${{ .server.base_image_registry }}") {
		t.Errorf("template variable lost in image:\n%s", yamlText)
	}

	// Verify script name (not ID).
	if !contains(yamlText, "install-tools.sh") {
		t.Errorf("startup script name missing:\n%s", yamlText)
	}

	// Parse back.
	parsed, err := ParseTemplateExport([]byte(yamlText))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Name != exp.Name {
		t.Errorf("name mismatch: %s != %s", parsed.Name, exp.Name)
	}
	if parsed.Platform != exp.Platform {
		t.Errorf("platform mismatch: %s != %s", parsed.Platform, exp.Platform)
	}
	if parsed.Job != exp.Job {
		t.Errorf("job mismatch")
	}
	if parsed.Volumes != exp.Volumes {
		t.Errorf("volumes mismatch")
	}
	if parsed.StartupScript != exp.StartupScript {
		t.Errorf("startup script mismatch: %s != %s", parsed.StartupScript, exp.StartupScript)
	}
	if len(parsed.CustomFields) != 1 || parsed.CustomFields[0].Name != "branch" {
		t.Errorf("custom fields mismatch: %+v", parsed.CustomFields)
	}
	if len(parsed.Groups) != 1 || parsed.Groups[0] != "developers" {
		t.Errorf("groups mismatch: %+v", parsed.Groups)
	}
	if !parsed.Features.WithTerminal || !parsed.Features.WithSSH {
		t.Errorf("features mismatch: %+v", parsed.Features)
	}
}

func TestExportFromDetails(t *testing.T) {
	details := &TemplateDetails{
		Name:         "Test",
		Platform:     "docker",
		Job:          "image: x:1",
		ComputeUnits: 2,
		WithTerminal: true,
		CustomFields: []CustomFieldDef{
			{Name: "key", Description: "desc"},
		},
		Schedule: []TemplateDetailsDay{
			{Enabled: true, From: "9:00am", To: "5:00pm"},
		},
		Ports: []model.TemplatePort{
			{Name: "web", Port: 80, Protocol: "http"},
		},
	}

	exp := ExportFromDetails(details)
	if exp.Name != "Test" {
		t.Errorf("name: %s", exp.Name)
	}
	if exp.Platform != "docker" {
		t.Errorf("platform: %s", exp.Platform)
	}
	if exp.Job != "image: x:1" {
		t.Errorf("job: %s", exp.Job)
	}
	if !exp.Features.WithTerminal {
		t.Error("with_terminal not set")
	}
	if len(exp.CustomFields) != 1 || exp.CustomFields[0].Name != "key" {
		t.Errorf("custom fields: %+v", exp.CustomFields)
	}
	if len(exp.Schedule) != 1 || !exp.Schedule[0].Enabled {
		t.Errorf("schedule: %+v", exp.Schedule)
	}
	if len(exp.Ports) != 1 || exp.Ports[0].Name != "web" {
		t.Errorf("ports: %+v", exp.Ports)
	}
}

func TestToCreateRequest(t *testing.T) {
	exp := &TemplateExport{
		Name:         "My Template",
		Platform:     "nomad",
		ComputeUnits: 1,
		Features: TemplateExportFeatures{
			WithSSH: true,
		},
		StartupScript: "script-uuid-here",
		CustomFields: []TemplateExportCustomField{
			{Name: "branch"},
		},
		Schedule: []TemplateExportScheduleDay{
			{Enabled: true, From: "9:00am", To: "5:00pm"},
		},
	}

	req := exp.ToCreateRequest()
	if req.Name != "My Template" {
		t.Errorf("name: %s", req.Name)
	}
	if req.Platform != "nomad" {
		t.Errorf("platform: %s", req.Platform)
	}
	if !req.WithSSH {
		t.Error("with_ssh not set")
	}
	if req.StartupScriptId != "script-uuid-here" {
		t.Errorf("startup script id: %s", req.StartupScriptId)
	}
	if len(req.CustomFields) != 1 || req.CustomFields[0].Name != "branch" {
		t.Errorf("custom fields: %+v", req.CustomFields)
	}
	if len(req.Schedule) != 1 || !req.Schedule[0].Enabled {
		t.Errorf("schedule: %+v", req.Schedule)
	}
}

func TestTemplateExportYAMLFormat(t *testing.T) {
	// Verify the YAML output uses block scalars for job/volumes and is
	// human-readable (no JSON escaping of newlines).
	exp := &TemplateExport{
		Name:     "Test",
		Platform: "nomad",
		Job:      "line1\nline2\nline3\n",
	}

	data, err := yaml.Marshal(exp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)

	// Block scalar indicator should be present.
	if !contains(s, "job: |") {
		t.Errorf("job should use block scalar:\n%s", s)
	}
	// Lines should be on separate lines, not escaped.
	if !contains(s, "line1\n") || !contains(s, "line2\n") {
		t.Errorf("job content should be multi-line:\n%s", s)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
