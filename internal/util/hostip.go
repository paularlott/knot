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

// HostIP returns the host's primary outbound IPv4 address. It prefers the
// address the routing table would use for an external destination (UDP dial
// sends no packets), then falls back to the first non-loopback interface
// address, then to loopback.
func HostIP() string {
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
