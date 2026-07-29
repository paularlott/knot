package specwizard

import (
	"strings"
	"testing"
)

func TestNormaliseCapability(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"CAP_NET_ADMIN", "CAP_NET_ADMIN"},
		{"cap_net_admin", "CAP_NET_ADMIN"},
		{"net_admin", "CAP_NET_ADMIN"},
		{"NET_ADMIN", "CAP_NET_ADMIN"},
		{"  net_raw  ", "CAP_NET_RAW"},
		{"Cap_Sys_Time", "CAP_SYS_TIME"},
		{"CAP_BLOCK_SUSPEND", "CAP_BLOCK_SUSPEND"},
		// Unknown-but-well-formed names are kept: the catalog is a subset.
		{"CAP_FUTURE_THING", "CAP_FUTURE_THING"},
		// Malformed / unusable input.
		{"", ""},
		{"   ", ""},
		{"CAP_", ""},
		{"cap_", ""},
		{"net admin", ""},
		{"net-admin", ""},
		{"net.admin", ""},
		{`net"admin`, ""},
		{"1NET", ""},
		{"CAP_1NET", ""},
	}
	for _, c := range cases {
		if got := NormaliseCapability(c.in); got != c.want {
			t.Errorf("NormaliseCapability(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormaliseCapabilities_dedupeAndOrder(t *testing.T) {
	in := []string{"net_admin", "CAP_NET_ADMIN", "", "sys_time", "bogus name", "CAP_SYS_TIME", "mknod"}
	got := NormaliseCapabilities(in)
	want := []string{"CAP_NET_ADMIN", "CAP_SYS_TIME", "CAP_MKNOD"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

func TestNormaliseCapabilities_emptyResultIsNil(t *testing.T) {
	for _, in := range [][]string{nil, {}, {""}, {"  "}, {"not valid", "also-bad"}} {
		if got := NormaliseCapabilities(in); got != nil {
			t.Errorf("NormaliseCapabilities(%v) = %v, want nil", in, got)
		}
	}
}

func TestCapabilityBareName(t *testing.T) {
	if got := capabilityBareName("CAP_NET_ADMIN"); got != "net_admin" {
		t.Errorf("got %q, want net_admin", got)
	}
	if got := capabilityBareName("sys_ptrace"); got != "sys_ptrace" {
		t.Errorf("got %q, want sys_ptrace", got)
	}
	if got := capabilityBareName("nope!"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// TestCapabilityCatalog guards the invariants the wizard UI depends on: every
// entry is canonical, described, and listed once.
func TestCapabilityCatalog(t *testing.T) {
	caps := Capabilities()
	if len(caps) < 20 {
		t.Fatalf("catalog looks too small: %d entries", len(caps))
	}
	seen := map[string]bool{}
	for _, c := range caps {
		if c.Name != NormaliseCapability(c.Name) {
			t.Errorf("catalog entry %q is not in canonical form", c.Name)
		}
		if !strings.HasPrefix(c.Name, "CAP_") {
			t.Errorf("catalog entry %q missing CAP_ prefix", c.Name)
		}
		if strings.TrimSpace(c.Description) == "" {
			t.Errorf("catalog entry %q has no description (search matches on it)", c.Name)
		}
		if seen[c.Name] {
			t.Errorf("catalog entry %q listed twice", c.Name)
		}
		seen[c.Name] = true
	}
	// A few capabilities the wizard is expected to offer.
	for _, want := range []string{"CAP_NET_ADMIN", "CAP_SYS_PTRACE", "CAP_AUDIT_WRITE", "CAP_MKNOD"} {
		if !seen[want] {
			t.Errorf("catalog missing %s", want)
		}
	}
}

func TestCapabilities_returnsCopy(t *testing.T) {
	first := Capabilities()
	if len(first) == 0 {
		t.Fatal("empty catalog")
	}
	original := first[0].Name
	first[0].Name = "CAP_MUTATED"
	if Capabilities()[0].Name != original {
		t.Error("Capabilities() handed out a reference to the package catalog")
	}
}
