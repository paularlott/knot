package service

import (
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

// TemplateRuntimeAvailableIn is a pure membership check — manual/Nomad
// always true, "container" matches any container runtime but never KVM
// alone, named platforms need exact membership.
func TestTemplateRuntimeAvailableIn(t *testing.T) {
	available := map[string]bool{"docker": true, model.PlatformKvm: true}

	cases := []struct {
		platform string
		want     bool
	}{
		{model.PlatformManual, true},
		{model.PlatformNomad, true},
		{model.PlatformDocker, true},
		{model.PlatformKvm, true},
		{model.PlatformPodman, false},
		{model.PlatformContainer, true}, // docker present
	}
	for _, tc := range cases {
		template := &model.Template{Platform: tc.platform}
		if got := TemplateRuntimeAvailableIn(template, available); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.platform, got, tc.want)
		}
	}

	// KVM alone does not satisfy "container".
	kvmOnly := map[string]bool{model.PlatformKvm: true}
	if TemplateRuntimeAvailableIn(&model.Template{Platform: model.PlatformContainer}, kvmOnly) {
		t.Error("container platform must not be satisfied by KVM alone")
	}
}
