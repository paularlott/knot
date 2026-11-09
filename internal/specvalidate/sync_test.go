package specvalidate

import (
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

const syncJob = "image: ubuntu\n"

func TestSyncBlockInVolumes(t *testing.T) {
	ok := `
sync:
  - local: /home/user/src
    remote: src:projects/${{ .space.name }}/
    snapshot: src:templates/go/
`
	for _, platform := range []string{model.PlatformDocker, model.PlatformPodman, model.PlatformApple} {
		if issues := ValidateTemplateSpec(platform, syncJob, ok); len(issues) != 0 {
			t.Errorf("%s: a sync-only volumes refused: %v", platform, issues)
		}
		if issues := ValidateTemplateSpec(platform, syncJob, "volumes:\n  data:\n"+ok); len(issues) != 0 {
			t.Errorf("%s: volumes with sync refused: %v", platform, issues)
		}
	}
	if issues := validateNomadVolumes("volumes", ok, false); len(issues) != 0 {
		t.Errorf("nomad: a sync-only volumes refused: %v", issues)
	}
	if issues := validateKvmVolumes("volumes", ok); len(issues) != 0 {
		t.Errorf("kvm: a sync-only volumes refused: %v", issues)
	}
	if issues := validateKvmVolumes("volumes", "volumes:\n  data:\n"+ok); len(issues) == 0 {
		t.Error("kvm: volumes accepted")
	}

	for name, bad := range map[string]string{
		"unknown setting": "sync:\n  - local: /a\n    remote: b1:x\n    wach: true\n",
		"no remote":       "sync:\n  - local: /a\n",
		"bad mode":        "sync:\n  - local: /a\n    remote: b1:x\n    mode: both\n",
		"not a list":      "sync:\n  local: /a\n",
	} {
		issues := ValidateTemplateSpec(model.PlatformDocker, syncJob, bad)
		if len(issues) == 0 || !strings.Contains(issues[0].Message, "sync") {
			t.Errorf("%s: %v", name, issues)
		}
	}
	if issues := ValidateVolumeSpec(model.PlatformDocker, "volumes:\n  data:\n"+ok); len(issues) == 0 {
		t.Error("a volume definition with a sync block was accepted")
	}
}
