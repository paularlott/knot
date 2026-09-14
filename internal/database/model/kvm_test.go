package model

import (
	"strings"
	"testing"
)

func kvmTemplate(cidr, start, end, gateway string) *Template {
	return &Template{
		Platform:        PlatformKvm,
		KvmNetworkCidr:  cidr,
		KvmIPRangeStart: start,
		KvmIPRangeEnd:   end,
		KvmGateway:      gateway,
	}
}

func TestParseKvmNetworkValid(t *testing.T) {
	network, err := ParseKvmNetwork(kvmTemplate("192.168.50.0/24", "192.168.50.10", "192.168.50.100", ""))
	if err != nil {
		t.Fatalf("expected valid network, got %v", err)
	}
	if network.Start.String() != "192.168.50.10" || network.End.String() != "192.168.50.100" {
		t.Fatalf("unexpected range %s - %s", network.Start, network.End)
	}
	if network.Gateway != nil {
		t.Fatalf("gateway should be unset when the field is empty")
	}
	if network.Broadcast().String() != "192.168.50.255" {
		t.Fatalf("unexpected broadcast %s", network.Broadcast())
	}
}

func TestParseKvmNetworkErrors(t *testing.T) {
	cases := []struct {
		name             string
		cidr, start, end string
		wantSubstring    string
	}{
		{"bad cidr", "192.168.50", "192.168.50.10", "192.168.50.20", "not a valid CIDR"},
		{"ipv6 cidr", "fd00::/8", "fd00::1", "fd00::2", "IPv4"},
		{"bad start", "192.168.50.0/24", "not-an-ip", "192.168.50.20", "not a valid IPv4"},
		{"start outside", "192.168.50.0/24", "10.0.0.1", "192.168.50.20", "outside the network"},
		{"end outside", "192.168.50.0/24", "192.168.50.10", "10.0.0.1", "outside the network"},
		{"inverted range", "192.168.50.0/24", "192.168.50.50", "192.168.50.10", "after the range end"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseKvmNetwork(kvmTemplate(tc.cidr, tc.start, tc.end, ""))
			if err == nil || !strings.Contains(err.Error(), tc.wantSubstring) {
				t.Fatalf("expected error containing %q, got %v", tc.wantSubstring, err)
			}
		})
	}
}

func TestOptionalIPRange(t *testing.T) {
	// No range: the whole usable subnet is pickable (gateway and broadcast
	// still excluded at pick time).
	network, err := ParseKvmNetwork(kvmTemplate("192.0.2.0/24", "", "", "192.0.2.1"))
	if err != nil {
		t.Fatalf("empty range should parse: %v", err)
	}
	start, end := network.Range()
	if start.String() != "192.0.2.1" || end.String() != "192.0.2.254" {
		t.Fatalf("default range = %s - %s, want 192.0.2.1 - 192.0.2.254", start, end)
	}
	if err := network.ValidateSpaceIP("192.0.2.200"); err != nil {
		t.Errorf("192.0.2.200 should be pickable with no range: %v", err)
	}
	if err := network.ValidateSpaceIP("192.0.2.255"); err == nil {
		t.Error("broadcast must still be rejected with no range")
	}

	// Half a range is a config error, not a silent default.
	if _, err := ParseKvmNetwork(kvmTemplate("192.0.2.0/24", "192.0.2.10", "", "")); err == nil {
		t.Error("start without end must fail")
	}
}

func TestValidateSpaceIP(t *testing.T) {
	network, err := ParseKvmNetwork(kvmTemplate("192.168.50.0/24", "192.168.50.10", "192.168.50.100", "192.168.50.1"))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	valid := []string{"192.168.50.10", "192.168.50.100", "192.168.50.42"}
	for _, ip := range valid {
		if err := network.ValidateSpaceIP(ip); err != nil {
			t.Errorf("expected %s valid, got %v", ip, err)
		}
	}

	invalid := map[string]string{
		"not-an-ip":      "not a valid IPv4",
		"192.168.51.42":  "outside the network",
		"192.168.50.5":   "outside the template's range",
		"192.168.50.0":   "network address",
		"192.168.50.255": "broadcast address",
		"192.168.50.1":   "gateway address",
	}
	for ip, want := range invalid {
		err := network.ValidateSpaceIP(ip)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("expected %s rejected with %q, got %v", ip, want, err)
		}
	}
}

// The allowlist gates what's offered; empty = all, "container" is offered
// when any container runtime is, manual always.
func TestBackendEnabled(t *testing.T) {
	all := []string{}
	if !BackendEnabled(PlatformDocker, all) || !BackendEnabled(PlatformKvm, all) || !BackendEnabled(PlatformNomad, all) {
		t.Error("empty allowlist must offer everything")
	}

	dockerOnly := []string{PlatformDocker}
	if !BackendEnabled(PlatformDocker, dockerOnly) {
		t.Error("docker must be offered")
	}
	if BackendEnabled(PlatformPodman, dockerOnly) || BackendEnabled(PlatformNomad, dockerOnly) || BackendEnabled(PlatformKvm, dockerOnly) {
		t.Error("unlisted platforms must not be offered")
	}
	if !BackendEnabled(PlatformContainer, dockerOnly) {
		t.Error("container (auto) must be offered when a container runtime is listed")
	}
	if BackendEnabled(PlatformManual, dockerOnly) {
		t.Error("manual must follow the allowlist — excluded when not listed")
	}
	if !BackendEnabled(PlatformManual, []string{PlatformDocker, PlatformManual}) {
		t.Error("manual must be offered when listed")
	}

	// An effectively empty list (blank entries from an empty env var or
	// config value) means all — never "nothing offered".
	if !BackendEnabled(PlatformDocker, []string{""}) || !BackendEnabled(PlatformManual, []string{""}) {
		t.Error("blank-only allowlist must offer everything")
	}

	kvmOnly := []string{PlatformKvm}
	if BackendEnabled(PlatformContainer, kvmOnly) {
		t.Error("container must not be offered on a KVM-only allowlist")
	}
}
