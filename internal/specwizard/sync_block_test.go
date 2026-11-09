package specwizard

import (
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
)

const syncVolumes = `sync:
  - local: /home/user/src
    remote: src:projects/foo/
`

func TestWizardKeepsTheSyncBlock(t *testing.T) {
	spec := &apiclient.UnifiedSpec{Name: "x", Image: "ubuntu"}
	for _, platform := range []string{"docker", "nomad", "kvm"} {
		_, volumes, err := Build(platform, spec, "", "volumes:\n  data:\n"+syncVolumes)
		if err != nil {
			t.Fatalf("%s: %v", platform, err)
		}
		if !strings.Contains(volumes, "remote: src:projects/foo/") {
			t.Errorf("%s: the sync block was lost:\n%s", platform, volumes)
		}
	}
	if got := withSyncBlock("volumes:\n  a: {}\n", ""); got != "volumes:\n  a: {}\n" {
		t.Errorf("volumes without a sync block changed: %q", got)
	}
	if got := withSyncBlock(syncVolumes, syncVolumes); strings.Count(got, "sync:") != 1 {
		t.Errorf("a sync block added twice: %q", got)
	}
}
