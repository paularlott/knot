package specwizard

import (
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/specvalidate"
)

// The wizard's output has to satisfy knot's own spec validator — otherwise the
// template form rejects a spec the wizard just wrote. These tests close that
// loop for the container platform (the Nomad validator needs a live Nomad
// endpoint to parse HCL, so it isn't exercised here).
func TestContainerYAMLFromWizardPassesValidator(t *testing.T) {
	spec := &apiclient.UnifiedSpec{
		Image:      "${{ .server.base_image_registry }}/knot-ubuntu:26.04",
		Hostname:   "${{ .space.name }}",
		Memory:     "2G",
		CPUs:       "2",
		Privileged: true,
		Network:    "bridge",
		Command:    []string{"sleep", "infinity"},
		Environment: []apiclient.KeyValue{
			{Key: "TZ", Value: "${{ .user.timezone }}"},
		},
		Ports: []apiclient.PortMapping{
			{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"},
			{HostPort: 53, ContainerPort: 53, Protocol: "tcp+udp"},
		},
		Storage: []apiclient.StorageEntry{
			{Kind: "bind", HostPath: "workspace", ContainerPath: "/workspace"},
			{Kind: "bind", HostPath: "/etc/hosts", ContainerPath: "/etc/hosts", ReadOnly: true},
		},
		DNS:       []string{"1.1.1.1"},
		DNSSearch: []string{"internal.example"},
		// Deliberately mixed input forms: the emitter must canonicalise them.
		CapAdd:  []string{"net_admin", "CAP_SYS_PTRACE"},
		CapDrop: []string{"mknod"},
	}

	job, _, err := BuildContainerYAML(spec, "", "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}

	issues := specvalidate.ValidateTemplateSpec(model.PlatformDocker, job, "")
	if len(issues) > 0 {
		t.Fatalf("wizard output failed validation: %+v\n--- spec ---\n%s", issues, job)
	}
}

func TestContainerYAMLPatchedByWizardPassesValidator(t *testing.T) {
	original := `# hand written spec
image: registry.example.com/app:1
container_name: fixed-name
memory: 1G
cap_add:
  - CAP_AUDIT_WRITE
`
	spec, wizardable, reason := ParseContainerYAML(original, "")
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}
	spec.CapAdd = append(spec.CapAdd, "net_admin")
	spec.CapDrop = []string{"mknod"}

	job, _, err := BuildContainerYAML(spec, original, "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}
	if issues := specvalidate.ValidateTemplateSpec(model.PlatformDocker, job, ""); len(issues) > 0 {
		t.Fatalf("patched output failed validation: %+v\n--- spec ---\n%s", issues, job)
	}
	if !strings.Contains(job, "container_name: fixed-name") {
		t.Errorf("container_name lost:\n%s", job)
	}
	if !strings.Contains(job, "- CAP_AUDIT_WRITE") || !strings.Contains(job, "- CAP_NET_ADMIN") {
		t.Errorf("capabilities not merged:\n%s", job)
	}
	if !strings.Contains(job, "- CAP_MKNOD") {
		t.Errorf("cap_drop missing:\n%s", job)
	}
}

// specvalidateIssues is a small shim so other tests in this package can assert
// their generated container spec passes knot's validator.
func specvalidateIssues(job string) []specvalidate.Issue {
	return specvalidate.ValidateTemplateSpec(model.PlatformDocker, job, "")
}
