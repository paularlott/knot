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

	start := net.ParseIP(template.KvmIPRangeStart)
	if start == nil || start.To4() == nil {
		return nil, fmt.Errorf("KVM IP range start %q is not a valid IPv4 address", template.KvmIPRangeStart)
	}
	if !ipNet.Contains(start) {
		return nil, fmt.Errorf("KVM IP range start %s is outside the network %s", start, ipNet)
	}

	end := net.ParseIP(template.KvmIPRangeEnd)
	if end == nil || end.To4() == nil {
		return nil, fmt.Errorf("KVM IP range end %q is not a valid IPv4 address", template.KvmIPRangeEnd)
	}
	if !ipNet.Contains(end) {
		return nil, fmt.Errorf("KVM IP range end %s is outside the network %s", end, ipNet)
	}

	if bytes.Compare(start.To4(), end.To4()) > 0 {
		return nil, fmt.Errorf("KVM IP range start %s is after the range end %s", start, end)
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

	ip4 := ip.To4()
	if bytes.Compare(ip4, n.Start.To4()) < 0 || bytes.Compare(ip4, n.End.To4()) > 0 {
		return fmt.Errorf("IP address %s is outside the template's range %s - %s", ip, n.Start, n.End)
	}

	return nil
}
