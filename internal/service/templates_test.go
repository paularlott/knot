package service

import (
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

func TestGetTemplateService(t *testing.T) {
	service := GetTemplateService()
	if service == nil {
		t.Fatal("GetTemplateService returned nil")
	}

	service2 := GetTemplateService()
	if service != service2 {
		t.Error("GetTemplateService should return singleton")
	}
}

func TestValidateTemplateInput(t *testing.T) {
	service := GetTemplateService()

	tests := []struct {
		name          string
		templateName  string
		platform      string
		job           string
		volumes       string
		computeUnits  int
		storageUnits  int
		maxUptime     int
		maxUptimeUnit string
		expectError   bool
	}{
		{
			name:          "valid docker template",
			templateName:  "test-template",
			platform:      model.PlatformDocker,
			job:           "docker run test",
			volumes:       "",
			computeUnits:  100,
			storageUnits:  200,
			maxUptime:     8,
			maxUptimeUnit: "hour",
			expectError:   false,
		},
		{
			name:          "empty name",
			templateName:  "",
			platform:      model.PlatformDocker,
			job:           "docker run test",
			volumes:       "",
			computeUnits:  100,
			storageUnits:  200,
			maxUptime:     8,
			maxUptimeUnit: "hour",
			expectError:   true,
		},
		{
			name:          "invalid platform",
			templateName:  "test",
			platform:      "invalid",
			job:           "docker run test",
			volumes:       "",
			computeUnits:  100,
			storageUnits:  200,
			maxUptime:     8,
			maxUptimeUnit: "hour",
			expectError:   true,
		},
		{
			name:          "manual platform without job",
			templateName:  "test",
			platform:      model.PlatformManual,
			job:           "",
			volumes:       "",
			computeUnits:  100,
			storageUnits:  200,
			maxUptime:     8,
			maxUptimeUnit: "hour",
			expectError:   false,
		},
		{
			name:          "non-manual platform without job",
			templateName:  "test",
			platform:      model.PlatformDocker,
			job:           "",
			volumes:       "",
			computeUnits:  100,
			storageUnits:  200,
			maxUptime:     8,
			maxUptimeUnit: "hour",
			expectError:   true,
		},
		{
			name:          "negative compute units",
			templateName:  "test",
			platform:      model.PlatformDocker,
			job:           "docker run test",
			volumes:       "",
			computeUnits:  -1,
			storageUnits:  200,
			maxUptime:     8,
			maxUptimeUnit: "hour",
			expectError:   true,
		},
		{
			name:          "invalid uptime unit",
			templateName:  "test",
			platform:      model.PlatformDocker,
			job:           "docker run test",
			volumes:       "",
			computeUnits:  100,
			storageUnits:  200,
			maxUptime:     8,
			maxUptimeUnit: "invalid",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.validateTemplateInput(
				tt.templateName,
				tt.platform,
				tt.job,
				tt.volumes,
				tt.computeUnits,
				tt.storageUnits,
				tt.maxUptime,
				tt.maxUptimeUnit,
				false,
				nil,
				nil,
			)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestValidateTemplateInputWithSchedule(t *testing.T) {
	service := GetTemplateService()

	validSchedule := []model.TemplateScheduleDays{
		{Enabled: true, From: "9:00am", To: "5:00pm"},
		{Enabled: true, From: "9:00am", To: "5:00pm"},
		{Enabled: true, From: "9:00am", To: "5:00pm"},
		{Enabled: true, From: "9:00am", To: "5:00pm"},
		{Enabled: true, From: "9:00am", To: "5:00pm"},
		{Enabled: false, From: "9:00am", To: "5:00pm"},
		{Enabled: false, From: "9:00am", To: "5:00pm"},
	}

	err := service.validateTemplateInput(
		"test",
		model.PlatformDocker,
		"docker run test",
		"",
		100,
		200,
		8,
		"hour",
		true,
		&validSchedule,
		nil,
	)

	if err != nil {
		t.Errorf("Valid schedule should not error: %v", err)
	}

	invalidSchedule := []model.TemplateScheduleDays{
		{Enabled: true, From: "9:00am", To: "5:00pm"},
	}

	err = service.validateTemplateInput(
		"test",
		model.PlatformDocker,
		"docker run test",
		"",
		100,
		200,
		8,
		"hour",
		true,
		&invalidSchedule,
		nil,
	)

	if err == nil {
		t.Error("Invalid schedule (not 7 days) should error")
	}
}

func TestValidateTemplateInputWithCustomFields(t *testing.T) {
	service := GetTemplateService()

	validFields := []model.TemplateCustomField{
		{Name: "field1", Description: "Test field"},
		{Name: "field2", Description: "Another field"},
	}

	err := service.validateTemplateInput(
		"test",
		model.PlatformDocker,
		"docker run test",
		"",
		100,
		200,
		8,
		"hour",
		false,
		nil,
		validFields,
	)

	if err != nil {
		t.Errorf("Valid custom fields should not error: %v", err)
	}

	invalidFields := []model.TemplateCustomField{
		{Name: "1invalid", Description: "Test"},
	}

	err = service.validateTemplateInput(
		"test",
		model.PlatformDocker,
		"docker run test",
		"",
		100,
		200,
		8,
		"hour",
		false,
		nil,
		invalidFields,
	)

	if err == nil {
		t.Error("Invalid custom field name should error")
	}
}
