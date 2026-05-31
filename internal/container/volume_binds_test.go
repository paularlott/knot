package container

import (
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

func TestValidateManagedVolumeBinds(t *testing.T) {
	volumeData := model.VolumeDataMap{
		"volume-space": model.SpaceVolume{Id: "volume-space", Namespace: "_docker"},
	}

	tests := []struct {
		name    string
		binds   []string
		wantErr string
	}{
		{
			name:  "host path bind",
			binds: []string{"/Users/paul/t:/myhome"},
		},
		{
			name:  "declared named volume",
			binds: []string{"volume-space:/home"},
		},
		{
			name:    "undeclared named volume",
			binds:   []string{"volume-paul:/home"},
			wantErr: `undeclared named volume "volume-paul"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateManagedVolumeBinds(tt.binds, volumeData)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateManagedVolumeBinds() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateManagedVolumeBinds() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
