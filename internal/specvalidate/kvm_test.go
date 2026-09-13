package specvalidate

import (
	"strings"
	"testing"
)

func TestValidateKvmJob(t *testing.T) {
	issues := ValidateTemplateSpec("kvm", `
image: https://cloud.example.com/base.qcow2
cpus: 2
memory: 4G
disk: 20G
network:
  cidr: 192.168.50.0/24
  bridge: br0
  ip_range_start: 192.168.50.10
  ip_range_end: 192.168.50.100
  gateway: 192.168.50.1
environment:
  - KEY=value
`, "")
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
}

func TestValidateKvmJobErrors(t *testing.T) {
	// image + network required
	issues := ValidateTemplateSpec("kvm", "cpus: 2\n", "")
	if !containsIssue(issues, 0, "image must be set") {
		t.Fatalf("expected image-required issue, got %+v", issues)
	}
	if !containsIssue(issues, 0, "network must be configured") {
		t.Fatalf("expected network-required issue, got %+v", issues)
	}

	// inconsistent network values are reported with the block's line
	issues = ValidateTemplateSpec("kvm", `image: /tmp/base.qcow2
network:
  cidr: 192.168.50.0/24
  ip_range_start: 10.0.0.10
  ip_range_end: 192.168.50.100
`, "")
	if !containsIssue(issues, 3, "outside the network") {
		t.Fatalf("expected network range issue on line 3 (the mapping body), got %+v", issues)
	}

	// unknown network sub-fields surface
	issues = ValidateTemplateSpec("kvm", `image: /tmp/base.qcow2
network:
  cidr: 192.168.50.0/24
  ip_range_start: 192.168.50.10
  ip_range_end: 192.168.50.100
  dns: 1.1.1.1
`, "")
	if !containsIssue(issues, 6, "unknown network field", "dns") {
		t.Fatalf("expected unknown network field issue, got %+v", issues)
	}

	// empty spec
	issues = ValidateTemplateSpec("kvm", "  \n", "")
	if !containsIssue(issues, 0, "virtual machine specification is required") {
		t.Fatalf("expected required issue, got %+v", issues)
	}

	// bad values and unknown fields carry lines
	issues = ValidateTemplateSpec("kvm", `image: /tmp/base.qcow2
memory: lots
cpus: -1
disk: big
ports:
  - "80:80"
network:
  cidr: 192.168.50.0/24
  ip_range_start: 192.168.50.10
  ip_range_end: 192.168.50.100
`, "")
	if !containsIssue(issues, 2, "invalid memory") {
		t.Fatalf("expected memory issue on line 2, got %+v", issues)
	}
	if !containsIssue(issues, 3, "invalid cpus") {
		t.Fatalf("expected cpus issue on line 3, got %+v", issues)
	}
	if !containsIssue(issues, 4, "invalid disk") {
		t.Fatalf("expected disk issue on line 4, got %+v", issues)
	}
	if !containsIssue(issues, 5, "unknown field", "ports") {
		t.Fatalf("expected unknown-field issue for ports, got %+v", issues)
	}
}

func TestValidateKvmNetworkModes(t *testing.T) {
	// NAT: no static addressing fields allowed, nothing else required.
	issues := ValidateTemplateSpec("kvm", `image: base
network:
  mode: nat
  bridge: default
`, "")
	if len(issues) != 0 {
		t.Fatalf("expected nat spec valid, got %+v", issues)
	}

	// NAT with static fields rejected.
	issues = ValidateTemplateSpec("kvm", `image: base
network:
  mode: nat
  cidr: 192.168.50.0/24
  ip_range_start: 192.168.50.10
  ip_range_end: 192.168.50.100
`, "")
	if !containsIssue(issues, 3, "must not be set in nat mode") {
		t.Fatalf("expected nat cidr rejection, got %+v", issues)
	}

	// Unknown mode rejected.
	issues = ValidateTemplateSpec("kvm", `image: base
network:
  mode: switched
  cidr: 192.168.50.0/24
  ip_range_start: 192.168.50.10
  ip_range_end: 192.168.50.100
`, "")
	if !containsIssue(issues, 3, "invalid network mode") {
		t.Fatalf("expected mode rejection, got %+v", issues)
	}
}

func TestValidateKvmDevices(t *testing.T) {
	valid := `image: base
devices:
  - pci_0000_01_00_0
  - usb_002_003
  - 0x8086:0x1234
  - 10de:2684
network:
  mode: nat
`
	if issues := ValidateTemplateSpec("kvm", valid, ""); len(issues) != 0 {
		t.Fatalf("expected valid device spec, got %+v", issues)
	}

	invalid := `image: base
devices:
  - pci_0000_01
  - usb_002
  - not-a-device
network:
  mode: nat
`
	issues := ValidateTemplateSpec("kvm", invalid, "")
	if !containsIssue(issues, 3, "PCI address") {
		t.Fatalf("expected PCI format issue, got %+v", issues)
	}
	if !containsIssue(issues, 4, "USB bus.device") {
		t.Fatalf("expected USB format issue, got %+v", issues)
	}
	if !containsIssue(issues, 5, "vendor:product") {
		t.Fatalf("expected vendor:product issue, got %+v", issues)
	}
}

func TestValidateKvmOptionalIPRange(t *testing.T) {
	issues := ValidateTemplateSpec("kvm", `image: base
network:
  mode: bridged
  cidr: 192.0.2.0/24
  bridge: br0
`, "")
	if len(issues) != 0 {
		t.Fatalf("bridged network without an IP range should be valid (whole-subnet default), got %+v", issues)
	}

	// Half a range is still rejected.
	issues = ValidateTemplateSpec("kvm", `image: base
network:
  mode: bridged
  cidr: 192.0.2.0/24
  ip_range_start: 192.0.2.10
`, "")
	if !containsIssue(issues, 3, "needs both start and end") {
		t.Fatalf("expected half-range rejection, got %+v", issues)
	}
}

func TestValidateKvmVolumesRejected(t *testing.T) {
	issues := ValidateTemplateSpec("kvm", "image: /tmp/base.qcow2\nnetwork:\n  cidr: 192.168.50.0/24\n  ip_range_start: 192.168.50.10\n  ip_range_end: 192.168.50.100\n", "volumes:\n  data: {}\n")
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "cannot define volumes") {
		t.Fatalf("expected volumes rejection, got %+v", issues)
	}
}
