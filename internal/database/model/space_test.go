package model

import (
	"testing"
	"time"
)

func TestNewSpace(t *testing.T) {
	altNames := []string{"alt1", "alt2"}
	customFields := []SpaceCustomField{
		{Name: "field1", Value: "value1"},
	}

	space := NewSpace("test-space", "test description", "user-123", "template-456", "/bin/bash", &altNames, "zone1", "icon.png", customFields)

	if space.Id == "" {
		t.Error("Space ID should not be empty")
	}
	if space.Name != "test-space" {
		t.Errorf("Expected name 'test-space', got '%s'", space.Name)
	}
	if space.UserId != "user-123" {
		t.Errorf("Expected userId 'user-123', got '%s'", space.UserId)
	}
	if space.TemplateId != "template-456" {
		t.Errorf("Expected templateId 'template-456', got '%s'", space.TemplateId)
	}
	if space.Shell != "/bin/bash" {
		t.Errorf("Expected shell '/bin/bash', got '%s'", space.Shell)
	}
	if len(space.AltNames) != 2 {
		t.Errorf("Expected 2 alt names, got %d", len(space.AltNames))
	}
	if space.Zone != "zone1" {
		t.Errorf("Expected zone 'zone1', got '%s'", space.Zone)
	}
	if space.IconURL != "icon.png" {
		t.Errorf("Expected iconURL 'icon.png', got '%s'", space.IconURL)
	}
	if len(space.CustomFields) != 1 {
		t.Errorf("Expected 1 custom field, got %d", len(space.CustomFields))
	}
	if space.IsDeployed {
		t.Error("New space should not be deployed")
	}
	if space.IsPending {
		t.Error("New space should not be pending")
	}
	if space.IsDeleting {
		t.Error("New space should not be deleting")
	}
	if space.SSHHostSigner == "" {
		t.Error("SSH host signer should be generated")
	}
	if space.VolumeData == nil {
		t.Error("VolumeData should be initialized")
	}
}

func TestMaxUptimeReached(t *testing.T) {
	tests := []struct {
		name           string
		maxUptime      uint32
		maxUptimeUnit  string
		startedAt      time.Time
		expectedResult bool
	}{
		{
			name:           "disabled uptime",
			maxUptime:      10,
			maxUptimeUnit:  "disabled",
			startedAt:      time.Now().UTC().Add(-1 * time.Hour),
			expectedResult: false,
		},
		{
			name:           "zero uptime",
			maxUptime:      0,
			maxUptimeUnit:  "hour",
			startedAt:      time.Now().UTC(),
			expectedResult: true,
		},
		{
			name:           "uptime not reached - minutes",
			maxUptime:      30,
			maxUptimeUnit:  "minute",
			startedAt:      time.Now().UTC().Add(-10 * time.Minute),
			expectedResult: false,
		},
		{
			name:           "uptime reached - minutes",
			maxUptime:      10,
			maxUptimeUnit:  "minute",
			startedAt:      time.Now().UTC().Add(-15 * time.Minute),
			expectedResult: true,
		},
		{
			name:           "uptime not reached - hours",
			maxUptime:      2,
			maxUptimeUnit:  "hour",
			startedAt:      time.Now().UTC().Add(-1 * time.Hour),
			expectedResult: false,
		},
		{
			name:           "uptime reached - hours",
			maxUptime:      1,
			maxUptimeUnit:  "hour",
			startedAt:      time.Now().UTC().Add(-2 * time.Hour),
			expectedResult: true,
		},
		{
			name:           "uptime not reached - days",
			maxUptime:      2,
			maxUptimeUnit:  "day",
			startedAt:      time.Now().UTC().Add(-24 * time.Hour),
			expectedResult: false,
		},
		{
			name:           "uptime reached - days",
			maxUptime:      1,
			maxUptimeUnit:  "day",
			startedAt:      time.Now().UTC().Add(-25 * time.Hour),
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			space := &Space{
				StartedAt: tt.startedAt,
			}
			template := &Template{
				MaxUptime:     tt.maxUptime,
				MaxUptimeUnit: tt.maxUptimeUnit,
			}

			result := space.MaxUptimeReached(template)
			if result != tt.expectedResult {
				t.Errorf("Expected %v, got %v", tt.expectedResult, result)
			}
		})
	}
}

func TestVolumeDataMapValueScan(t *testing.T) {
	volumeData := VolumeDataMap{
		"vol1": SpaceVolume{
			Id:        "volume-123",
			Namespace: "default",
			Type:      "csi",
		},
	}

	value, err := volumeData.Value()
	if err != nil {
		t.Fatalf("Value() failed: %v", err)
	}

	var scanned VolumeDataMap
	err = scanned.Scan(value)
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}

	if len(scanned) != 1 {
		t.Errorf("Expected 1 volume, got %d", len(scanned))
	}
	if scanned["vol1"].Id != "volume-123" {
		t.Errorf("Expected volume ID 'volume-123', got '%s'", scanned["vol1"].Id)
	}
}
