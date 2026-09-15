package model

import (
	"bytes"
	"fmt"
	"net"
)

// KvmNetwork is the parsed and validated form of a template's KVM network
// fields: the subnet VMs are attached to and the address range spaces pick
// their IP from.
type KvmNetwork struct {
	IPNet   *net.IPNet
	Start   net.IP
	End     net.IP
	Gateway net.IP
}

// ParseKvmNetwork validates a template's KVM network configuration. All
// fields must be present and consistent: CIDR parseable, the range inside
// the subnet with start <= end, and the optional gateway inside the subnet
// (it defaults to the network's first usable address).
func ParseKvmNetwork(template *Template) (*KvmNetwork, error) {
	_, ipNet, err := net.ParseCIDR(template.KvmNetworkCidr)
	if err != nil {
		return nil, fmt.Errorf("KVM network CIDR %q is not a valid CIDR (expected e.g. 192.168.50.0/24)", template.KvmNetworkCidr)
	}
	if ipNet.IP.To4() == nil {
		return nil, fmt.Errorf("KVM network CIDR %q must be an IPv4 network", template.KvmNetworkCidr)
	}

	// The range is optional: empty means the network's whole usable
	// address space (network+1 through broadcast-1), with the gateway still
	// excluded at pick time — the right default for a subnet dedicated to
	// VMs. Admins carving a slice out of a shared subnet set it explicitly.
	var start, end net.IP
	if template.KvmIPRangeStart != "" || template.KvmIPRangeEnd != "" {
		if template.KvmIPRangeStart == "" || template.KvmIPRangeEnd == "" {
			return nil, fmt.Errorf("KVM IP range needs both start and end (or neither)")
		}

		start = net.ParseIP(template.KvmIPRangeStart)
		if start == nil || start.To4() == nil {
			return nil, fmt.Errorf("KVM IP range start %q is not a valid IPv4 address", template.KvmIPRangeStart)
		}
		if !ipNet.Contains(start) {
			return nil, fmt.Errorf("KVM IP range start %s is outside the network %s", start, ipNet)
		}

		end = net.ParseIP(template.KvmIPRangeEnd)
		if end == nil || end.To4() == nil {
			return nil, fmt.Errorf("KVM IP range end %q is not a valid IPv4 address", template.KvmIPRangeEnd)
		}
		if !ipNet.Contains(end) {
			return nil, fmt.Errorf("KVM IP range end %s is outside the network %s", end, ipNet)
		}

		if bytes.Compare(start.To4(), end.To4()) > 0 {
			return nil, fmt.Errorf("KVM IP range start %s is after the range end %s", start, end)
		}
	}

	var gateway net.IP
	if template.KvmGateway != "" {
		gateway = net.ParseIP(template.KvmGateway)
		if gateway == nil || gateway.To4() == nil {
			return nil, fmt.Errorf("KVM gateway %q is not a valid IPv4 address", template.KvmGateway)
		}
		if !ipNet.Contains(gateway) {
			return nil, fmt.Errorf("KVM gateway %s is outside the network %s", gateway, ipNet)
		}
	}

	return &KvmNetwork{IPNet: ipNet, Start: start, End: end, Gateway: gateway}, nil
}

// Range returns the effective pick range: the configured start/end, or the
// network's whole usable address space when no range is set.
func (n *KvmNetwork) Range() (net.IP, net.IP) {
	if n.Start != nil && n.End != nil {
		return n.Start, n.End
	}
	return nextIP(n.IPNet.IP), prevIP(n.Broadcast())
}

// nextIP returns the address one above ip (the network base's next is the
// first usable address).
func nextIP(ip net.IP) net.IP {
	out := make(net.IP, len(ip))
	copy(out, ip)
	for i := len(out) - 1; i >= 0; i-- {
		out[i]++
		if out[i] != 0 {
			break
		}
	}
	return out
}

// prevIP returns the address one below ip (nil-safe enough for our use:
// only applied to a network's broadcast address).
func prevIP(ip net.IP) net.IP {
	out := make(net.IP, len(ip))
	copy(out, ip)
	for i := len(out) - 1; i >= 0; i-- {
		out[i]--
		if out[i] != 255 {
			break
		}
	}
	return out
}

// Broadcast returns the subnet's broadcast address.
func (n *KvmNetwork) Broadcast() net.IP {
	masked := n.IPNet.IP.To4()
	mask := n.IPNet.Mask
	broadcast := make(net.IP, len(masked))
	for i := range masked {
		broadcast[i] = masked[i] | ^mask[i]
	}
	return broadcast
}

// ValidateSpaceIP checks that ip is a usable address for a space's VM:
// valid IPv4, inside the configured range, and not the network, broadcast
// or gateway address.
func (n *KvmNetwork) ValidateSpaceIP(ipStr string) error {
	ip := net.ParseIP(ipStr)
	if ip == nil || ip.To4() == nil {
		return fmt.Errorf("IP address %q is not a valid IPv4 address", ipStr)
	}

	if !n.IPNet.Contains(ip) {
		return fmt.Errorf("IP address %s is outside the network %s", ip, n.IPNet)
	}

	if ip.Equal(n.IPNet.IP) {
		return fmt.Errorf("IP address %s is the network address", ip)
	}
	if ip.Equal(n.Broadcast()) {
		return fmt.Errorf("IP address %s is the broadcast address", ip)
	}
	if n.Gateway != nil && ip.Equal(n.Gateway) {
		return fmt.Errorf("IP address %s is the gateway address", ip)
	}

	start, end := n.Range()
	ip4 := ip.To4()
	if bytes.Compare(ip4, start.To4()) < 0 || bytes.Compare(ip4, end.To4()) > 0 {
		return fmt.Errorf("IP address %s is outside the template's range %s - %s", ip, start, end)
	}

	return nil
}
