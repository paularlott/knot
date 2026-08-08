package container

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/dns"
)

const (
	// AgentDNSEnvVar gates the in-container agent's resident resolver.
	AgentDNSEnvVar = "KNOT_AGENT_DNS"
	// AgentServerResolveEnv carries a curl "--resolve" value
	// ("<host>:<port>:<ip>") for the URL the entrypoint fetches the agent from.
	// The container's nameserver is 127.0.0.1 (not up until the agent starts),
	// so the fetch can't resolve KNOT_SERVER; curl --resolve connects straight
	// to the IP while keeping the hostname for SNI/Host.
	AgentServerResolveEnv = "KNOT_SERVER_RESOLVE"
	// AgentServerDNSEnv is the address ("<ip>:<port>") of the knot server's own
	// DNS server, which lives on the same host as the agent endpoint. The agent
	// forwards every query here; the server answers the wildcard zone from its
	// dns-records and forwards the rest upstream.
	AgentServerDNSEnv = "KNOT_SERVER_DNS"
	// AgentDNSListener is the address the agent resolver listens on; the
	// container's sole nameserver is pointed here.
	AgentDNSListener = "127.0.0.1"
)

// AgentDNSInjection is the set of container-level additions applied to a space
// when the server's DNS server is enabled (the signal that knot manages DNS for
// the wildcard domain, both for external clients and inside spaces).
type AgentDNSInjection struct {
	Env []string
	DNS []string // 127.0.0.1 (the agent resolver) — single entry
}

// BuildAgentDNSInjection computes the injection for a space. It returns a zero
// value when the server's DNS server is disabled.
//
// The container's only nameserver is 127.0.0.1 (the agent resolver), which
// forwards every query to the knot server's own DNS (KNOT_SERVER_DNS). That
// makes the server the single DNS point for the space: it serves the wildcard
// zone from its dns-records and forwards the rest upstream (using the server's
// configured nameservers, or the system default). The agent needs no upstream
// list of its own.
//
// Two different server addresses are involved:
//   - KNOT_SERVER_RESOLVE uses the URL host (the address the entrypoint fetches
//     the agent from — typically the ingress).
//   - KNOT_SERVER_DNS uses the agent-endpoint host (where the server process,
//     and thus its DNS listener, actually runs) — often a different host.
//
// If either host can't be resolved, an error is returned so the caller refuses
// to start the space.
func BuildAgentDNSInjection() (AgentDNSInjection, error) {
	cfg := config.GetServerConfig()
	if cfg == nil || !cfg.DNSEnabled {
		return AgentDNSInjection{}, nil
	}

	urlHost, urlPort := hostPortFromURL(cfg.URL)
	if urlHost == "" {
		return AgentDNSInjection{}, fmt.Errorf("DNS server enabled but server URL has no hostname: %q", cfg.URL)
	}
	urlIPs, err := dns.LookupIP(urlHost)
	if err != nil || len(urlIPs) == 0 {
		return AgentDNSInjection{}, fmt.Errorf("DNS server enabled but could not resolve server URL hostname %q for agent fetch: %w", urlHost, err)
	}

	epHost, _ := hostPortFromURL(cfg.AgentEndpoint)
	if epHost == "" {
		return AgentDNSInjection{}, fmt.Errorf("DNS server enabled but agent endpoint is not set")
	}
	epIPs, err := dns.LookupIP(epHost)
	if err != nil || len(epIPs) == 0 {
		return AgentDNSInjection{}, fmt.Errorf("DNS server enabled but could not resolve agent endpoint hostname %q for in-space DNS: %w", epHost, err)
	}

	dnsPort := portFromListen(cfg.DNSListen)
	return AgentDNSInjection{
		Env: []string{
			AgentDNSEnvVar + "=1",
			AgentServerResolveEnv + "=" + urlHost + ":" + urlPort + ":" + urlIPs[0],
			AgentServerDNSEnv + "=" + epIPs[0] + ":" + dnsPort,
		},
		DNS: []string{AgentDNSListener},
	}, nil
}

// hostPortFromURL extracts the hostname and port from a URL or host:port,
// defaulting the port from the scheme (443 for https, 80 otherwise) when absent.
func hostPortFromURL(raw string) (string, string) {
	if !strings.Contains(raw, "://") {
		raw = "//" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", ""
	}
	port := u.Port()
	if port == "" {
		if strings.EqualFold(u.Scheme, "https") {
			port = "443"
		} else {
			port = "80"
		}
	}
	return u.Hostname(), port
}

// portFromListen extracts the port from a DNS listen address like "0.0.0.0:3053"
// or ":3053", defaulting to "3053" if it cannot be parsed.
func portFromListen(listen string) string {
	if listen == "" {
		return "3053"
	}
	if _, port, err := net.SplitHostPort(listen); err == nil && port != "" {
		return port
	}
	return "3053"
}
