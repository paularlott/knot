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
