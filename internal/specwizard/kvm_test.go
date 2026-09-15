package specwizard

import (
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
)

func kvmTestSpec() *apiclient.UnifiedSpec {
	return &apiclient.UnifiedSpec{
		Name:     "${{ .user.username }}-${{ .space.name }}",
		Hostname: "${{ .space.name }}",
		Image:    "https://cloud.example.com/jammy.qcow2",
		Memory:   "4G",
		CPUs:     "2",
		Disk:     "20G",
		KvmNetwork: &apiclient.SpecKvmNetwork{
			Cidr:         "192.168.50.0/24",
			Bridge:       "br0",
			IPRangeStart: "192.168.50.10",
			IPRangeEnd:   "192.168.50.100",
		},
		Environment: []apiclient.KeyValue{
			{Key: "EDITOR", Value: "vim"},
		},
	}
}

func TestKvmYAMLRoundTrip(t *testing.T) {
	job, volumes, err := BuildKvmYAML(kvmTestSpec(), "", "")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if volumes != "" {
		t.Fatalf("KVM build must not emit volumes, got %q", volumes)
	}
	if !strings.Contains(job, "image: https://cloud.example.com/jammy.qcow2") {
		t.Fatalf("built job missing image: %s", job)
	}
	if !strings.Contains(job, "cpus: 2") {
		t.Fatalf("built job must emit unquoted numeric cpus: %s", job)
	}
	if !strings.Contains(job, "cidr: 192.168.50.0/24") || !strings.Contains(job, "ip_range_start: 192.168.50.10") {
		t.Fatalf("built job must emit the network block: %s", job)
	}

	spec, wizardable, reason := ParseKvmYAML(job, "")
	if !wizardable {
		t.Fatalf("round-tripped spec must be wizardable: %s", reason)
	}
	if spec.Image != "https://cloud.example.com/jammy.qcow2" ||
		spec.Memory != "4G" || spec.Disk != "20G" ||
		spec.CPUs != "2" || spec.Hostname != "${{ .space.name }}" {
		t.Fatalf("round-trip mismatch: %+v", spec)
	}
	if len(spec.Environment) != 1 || spec.Environment[0].Key != "EDITOR" {
		t.Fatalf("environment round-trip mismatch: %+v", spec.Environment)
	}
	if spec.KvmNetwork == nil || spec.KvmNetwork.Cidr != "192.168.50.0/24" || spec.KvmNetwork.IPRangeEnd != "192.168.50.100" {
		t.Fatalf("network round-trip mismatch: %+v", spec.KvmNetwork)
	}
}

func TestBuildKvmYAMLPatchesOriginal(t *testing.T) {
	original := `# my vm
image: /var/lib/images/base.qcow2  # local base
memory: 2G
extra_field: keep-me
`
	spec := kvmTestSpec()
	spec.Image = "/var/lib/images/new.qcow2"
	spec.Memory = "8G"

	job, _, err := BuildKvmYAML(spec, original, "")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(job, "# my vm") {
		t.Fatalf("patched job lost leading comment: %s", job)
	}
	if !strings.Contains(job, "extra_field: keep-me") {
		t.Fatalf("patched job lost unknown field: %s", job)
	}
	if !strings.Contains(job, "/var/lib/images/new.qcow2") {
		t.Fatalf("patched job did not update image: %s", job)
	}
	if strings.Contains(job, "memory: 2G") {
		t.Fatalf("patched job did not update memory: %s", job)
	}
}

// Regression: patching a spec whose network block was hand-edited must keep
// the block a mapping (a node-wrapper bug once replaced it with the scalar
// "cidr"), and numeric cpus must stay unquoted.
func TestBuildKvmYAMLPatchesNetworkAndNumericCPUs(t *testing.T) {
	original := `name: ${{ .user.username }}-${{ .space.name }}
hostname: ${{ .space.name }}
image: resolute-server-cloudimg-amd64.img
memory: 2G
cpus: 4
disk: 20G
network:
    cidr: 192.168.8.0/23
    bridge: br0
    ip_range_start: 192.168.9.10
    ip_range_end: 192.168.9.20
    gateway: 192.168.8.1
`
	spec := kvmTestSpec()
	spec.CPUs = "4"
	spec.KvmNetwork = &apiclient.SpecKvmNetwork{
		Cidr:         "10.10.0.0/16",
		Bridge:       "br1",
		IPRangeStart: "10.10.0.10",
		IPRangeEnd:   "10.10.0.100",
	}

	job, _, err := BuildKvmYAML(spec, original, "")
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if !strings.Contains(job, "cidr: 10.10.0.0/16") || !strings.Contains(job, "bridge: br1") {
		t.Fatalf("patched job did not update the network block: %s", job)
	}
	if strings.Contains(job, "192.168.8.0/23") {
		t.Fatalf("patched job kept the old network values: %s", job)
	}
	if strings.Contains(job, `network: cidr`) || !strings.Contains(job, "network:") {
		t.Fatalf("network block lost its mapping shape: %s", job)
	}
	if !strings.Contains(job, "cpus: 4") {
		t.Fatalf("patched job must keep cpus unquoted: %s", job)
	}
	if strings.Contains(job, `cpus: "4"`) {
		t.Fatalf("patched job quoted cpus: %s", job)
	}

	// The patched output must still parse with the network intact.
	parsed, wizardable, reason := ParseKvmYAML(job, "")
	if !wizardable {
		t.Fatalf("patched job must remain wizardable: %s", reason)
	}
	if parsed.KvmNetwork == nil || parsed.KvmNetwork.Cidr != "10.10.0.0/16" {
		t.Fatalf("patched network did not round-trip: %+v", parsed.KvmNetwork)
	}
}

func TestKvmNATRoundTrip(t *testing.T) {
	spec := &apiclient.UnifiedSpec{
		Image: "base",
		KvmNetwork: &apiclient.SpecKvmNetwork{
			Mode:   "nat",
			Bridge: "default",
		},
	}
	job, _, err := BuildKvmYAML(spec, "", "")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(job, "mode: nat") || strings.Contains(job, "cidr:") {
		t.Fatalf("nat build must carry mode and no static fields: %s", job)
	}

	parsed, wizardable, reason := ParseKvmYAML(job, "")
	if !wizardable {
		t.Fatalf("nat spec must be wizardable: %s", reason)
	}
	if parsed.KvmNetwork == nil || parsed.KvmNetwork.Mode != "nat" || parsed.KvmNetwork.Bridge != "default" {
		t.Fatalf("nat round-trip mismatch: %+v", parsed.KvmNetwork)
	}
}

// Regression: switching a spec from bridged to nat in the wizard leaves the
// static fields in the spec object — the build must drop them.
func TestKvmNATBuildDropsStaleStaticFields(t *testing.T) {
	spec := &apiclient.UnifiedSpec{
		Image: "base",
		KvmNetwork: &apiclient.SpecKvmNetwork{
			Mode:         "nat",
			Bridge:       "default",
			Cidr:         "192.168.8.0/23",
			IPRangeStart: "192.168.9.10",
			IPRangeEnd:   "192.168.9.20",
			Gateway:      "192.168.8.1",
		},
	}
	job, _, err := BuildKvmYAML(spec, "image: base\nnetwork:\n  mode: bridged\n  cidr: 192.168.8.0/23\n  ip_range_start: 192.168.9.10\n  ip_range_end: 192.168.9.20\n  gateway: 192.168.8.1\n", "")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	for _, stale := range []string{"cidr:", "ip_range_start:", "ip_range_end:", "gateway:"} {
		if strings.Contains(job, stale) {
			t.Fatalf("nat build must drop %s: %s", stale, job)
		}
	}
	if !strings.Contains(job, "mode: nat") {
		t.Fatalf("nat build must carry mode: %s", job)
	}
}

func TestKvmDevicesRoundTrip(t *testing.T) {
	spec := &apiclient.UnifiedSpec{
		Image:       "base",
		HostDevices: []string{"pci_0000_01_00_0", "0x8086:0x1234"},
		KvmNetwork:  &apiclient.SpecKvmNetwork{Mode: "nat"},
	}
	job, _, err := BuildKvmYAML(spec, "", "")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(job, "- pci_0000_01_00_0") || !strings.Contains(job, "- 0x8086:0x1234") {
		t.Fatalf("build must emit devices: %s", job)
	}

	parsed, wizardable, reason := ParseKvmYAML(job, "")
	if !wizardable {
		t.Fatalf("devices spec must be wizardable: %s", reason)
	}
	if len(parsed.HostDevices) != 2 || parsed.HostDevices[0] != "pci_0000_01_00_0" {
		t.Fatalf("devices round-trip mismatch: %+v", parsed.HostDevices)
	}

	// Patching preserves hand-written specs' devices when the wizard list
	// still carries them, and clears them when emptied.
	spec.HostDevices = nil
	job, _, err = BuildKvmYAML(spec, job, "")
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if strings.Contains(job, "pci_0000_01_00_0") {
		t.Fatalf("emptied devices must be removed: %s", job)
	}
}

func TestParseKvmYAMLRejectsVolumes(t *testing.T) {
	_, wizardable, reason := ParseKvmYAML("image: x\n", "volumes:\n  data: {}\n")
	if wizardable || !strings.Contains(reason, "cannot define volumes") {
		t.Fatalf("expected volumes rejection, got wizardable=%v reason=%q", wizardable, reason)
	}
}

func TestKvmFullyRepresentable(t *testing.T) {
	spec, _, _ := ParseKvmYAML("image: x\ncpus: 2\n", "")
	fully, reason := CheckFullyRepresentable("kvm", "image: x\ncpus: 2\n", "", spec)
	if !fully || reason != "" {
		t.Fatalf("expected fully representable, got %v %q", fully, reason)
	}

	fully, reason = CheckFullyRepresentable("kvm", "image: x\nbogus: 1\n", "", spec)
	if fully || !strings.Contains(reason, "bogus") {
		t.Fatalf("expected not fully representable, got %v %q", fully, reason)
	}
}
