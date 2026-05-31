package specvalidate

import (
	"fmt"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

func TestValidateTemplateSpecLocalContainerValid(t *testing.T) {
	issues := ValidateTemplateSpec(model.PlatformContainer, `
image: registry-1.docker.io/library/nginx:latest
ports:
  - "8080:80/tcp"
environment:
  - FOO=bar
volumes:
  - /tmp/cache:/cache
memory: 512M
cpus: "1.5"
`, `
volumes:
  app-cache:
`)

	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
}

func TestValidateTemplateSpecLocalContainerPathsOnly(t *testing.T) {
	issues := ValidateTemplateSpec(model.PlatformContainer, `
image: registry-1.docker.io/library/nginx:latest
volumes:
  - "workspace:/workspace"
`, `
paths:
  - workspace
  - ~/knot-test
  - /storage/${{ .space.id }}/data
`)

	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
}

func TestValidateTemplateSpecLocalContainerInvalid(t *testing.T) {
	issues := ValidateTemplateSpec(model.PlatformDocker, `
ports:
  - "abc:80"
environment:
  - BAD
`, `
volumez:
  wrong:
`)

	if len(issues) < 3 {
		t.Fatalf("expected multiple issues, got %+v", issues)
	}
}

func TestValidateTemplateSpecNomadUsesParser(t *testing.T) {
	parseCalls := 0
	issues := ValidateTemplateSpecWithNomadParser(
		model.PlatformNomad,
		`job "example" {}`,
		"",
		func(job string) error {
			parseCalls++
			if !strings.Contains(job, `job "example"`) {
				t.Fatalf("unexpected job: %q", job)
			}
			return fmt.Errorf("nomad parse failed")
		},
	)

	if parseCalls != 1 {
		t.Fatalf("expected parser to be called once, got %d", parseCalls)
	}

	if len(issues) != 1 || issues[0].Field != "job" {
		t.Fatalf("expected job issue, got %+v", issues)
	}
}

func TestValidateTemplateSpecNomadAllowsVolumesAndPaths(t *testing.T) {
	issues := ValidateTemplateSpecWithNomadParser(
		model.PlatformNomad,
		`job "example" {}`,
		`
volumes:
  - name: data
    type: csi
    plugin_id: hostpath
paths:
  - /storage/${{ .space.id }}/data
`,
		func(job string) error {
			return nil
		},
	)

	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
}

func TestValidateTemplateSpecNomadAllowsPathsOnly(t *testing.T) {
	issues := ValidateTemplateSpecWithNomadParser(
		model.PlatformNomad,
		`job "example" {}`,
		`
paths:
  - /storage/${{ .space.id }}/data
`,
		func(job string) error {
			return nil
		},
	)

	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
}

func TestValidateVolumeSpecLocalRequiresSingleVolume(t *testing.T) {
	issues := ValidateVolumeSpec(model.PlatformContainer, `
volumes:
  one:
  two:
`)

	if len(issues) != 1 {
		t.Fatalf("expected one issue, got %+v", issues)
	}
}

func TestValidateVolumeSpecAppleWithSize(t *testing.T) {
	issues := ValidateVolumeSpec(model.PlatformApple, `
volumes:
  workspace:
    size: 20G
`)

	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
}

func TestValidateVolumeSpecLocalRejectsPaths(t *testing.T) {
	issues := ValidateVolumeSpec(model.PlatformContainer, `
paths:
  - workspace
`)

	if len(issues) == 0 {
		t.Fatal("expected paths to be rejected for standalone volume definitions")
	}
}

func TestValidateVolumeSpecNomadRequiresPluginID(t *testing.T) {
	issues := ValidateVolumeSpec(model.PlatformNomad, `
volumes:
  - name: data
    type: csi
`)

	if len(issues) == 0 {
		t.Fatal("expected at least one issue")
	}
}
