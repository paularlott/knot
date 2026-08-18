package util

import (
	"net"
	"strings"
	"text/template"
)

// HostIPToken is a template function usable in any address value (e.g.
// server.agent_endpoint): `${{ host_ip }}:3010` renders to the host's IP
// address at startup. Agents run inside containers and cannot reach the
// host via 127.0.0.1, so endpoints advertised to them need the real
// interface IP. The ${{ }} delimiters match knot's template variable style
// (used to avoid clashing with Nomad interpolation); any spacing inside
// them works.
const HostIPToken = "${{ host_ip }}"

// HostIP returns the host's primary outbound IPv4 address. Physical
// interfaces are preferred — virtual interfaces (VPN/tailscale tunnels,
// docker/veth bridges) and their address ranges (CGNAT 100.64/10,
// link-local) are skipped first so advertised endpoints stay on durable
// addresses. If no physical candidate exists (e.g. a host only reachable
// via VPN) it falls back to the address the routing table would use for an
// external destination (UDP dial sends no packets), then to any
// non-loopback IPv4, then to loopback.
func HostIP() string {
	if ip, ok := physicalInterfaceIP(); ok {
		return ip
	}

	if conn, err := net.Dial("udp", "8.8.8.8:80"); err == nil {
		defer conn.Close()
		if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok && addr.IP != nil && !addr.IP.IsLoopback() {
			return addr.IP.String()
		}
	}

	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil && !ipnet.IP.IsLoopback() {
					return ipnet.IP.String()
				}
			}
		}
	}

	return "127.0.0.1"
}

// virtualInterfacePrefixes lists interface-name prefixes that carry
// non-durable addresses (VPN tunnels, container bridges).
var virtualInterfacePrefixes = []string{
	"utun", "tun", "tap", "tailscale", "wg", "vpn",
	"docker", "veth", "br-", "bridge", "ziti", "llk",
}

// physicalInterfaceIP scans interfaces for an IPv4 address on a physical
// (non-virtual, non-loopback) interface, skipping the CGNAT and
// link-local ranges used by VPN overlays.
func physicalInterfaceIP() (string, bool) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", false
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		name := strings.ToLower(iface.Name)
		virtual := false
		for _, prefix := range virtualInterfacePrefixes {
			if strings.HasPrefix(name, prefix) {
				virtual = true
				break
			}
		}
		if virtual {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil || ipnet.IP.IsLoopback() {
				continue
			}
			if ipnet.IP.IsLinkLocalUnicast() || isCGNAT(ipnet.IP) {
				continue
			}
			return ipnet.IP.String(), true
		}
	}
	return "", false
}

// isCGNAT reports whether the address is inside the RFC 6598 carrier-grade
// NAT range used by VPN overlays such as Tailscale (100.64.0.0/10).
func isCGNAT(ip net.IP) bool {
	cgnat := net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}
	return cgnat.Contains(ip)
}

// ResolveHostIP renders any template functions in an address using the
// address funcs (currently host_ip). knot-style `${{ func }}` delimiters
// are normalised to Go template actions first so the $ is consumed.
// Strings that are not templates — or that fail to parse or execute — are
// returned unchanged, so plain addresses pass through untouched.
func ResolveHostIP(s string) string {
	if !strings.Contains(s, "{{") {
		return s
	}
	s = strings.ReplaceAll(s, "${{", "{{")

	tmpl, err := template.New("address").Funcs(template.FuncMap{
		"host_ip": HostIP,
	}).Parse(s)
	if err != nil {
		return s
	}

	var out strings.Builder
	if err := tmpl.Execute(&out, nil); err != nil {
		return s
	}
	return out.String()
}
