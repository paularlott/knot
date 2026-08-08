package agentcmd

import (
	"os"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/dns"
	"github.com/paularlott/knot/internal/log"
)

// startResidentResolver starts the in-container DNS resolver on 127.0.0.1:53.
//
// It is the container's sole nameserver, so it always binds :53 when enabled.
// Every query is forwarded to the knot server's own DNS (KNOT_SERVER_DNS),
// which is the single DNS point for the space: it serves the wildcard zone from
// its dns-records and forwards the rest upstream. That includes the agent's own
// server endpoint — the agent resolves it through the server DNS like any other
// name, rather than trusting a server-resolved IP baked in at space start.
//
// KNOT_SERVER_DNS is an "<ip>:<port>" address, so the resolver connects directly
// — no DNS lookup, no loop back to this resolver.
//
// The returned cleanup function stops the resolver and must be called on
// shutdown. If the resolver cannot start, nil cleanup is returned and the agent
// continues normally.
func startResidentResolver() (*dns.DNSServer, func()) {
	logger := log.WithGroup("agent-dns")

	serverDNS := strings.TrimSpace(os.Getenv("KNOT_SERVER_DNS"))
	if serverDNS == "" {
		logger.Warn("KNOT_SERVER_DNS not set; resident resolver has nowhere to forward, not starting")
		return nil, nil
	}

	upstream := dns.NewDNSResolver(dns.ResolverConfig{
		QueryTimeout: 2 * time.Second,
		EnableCache:  true,
		MaxCacheTTL:  30,
		// Forward over TCP: the knot DNS often runs on the same host as the
		// container (e.g. macOS Apple Containers), where UDP to the host's own
		// address is refused (hairpin) but TCP works.
		UseTCP: true,
	})
	upstream.UpdateNameservers([]string{serverDNS})

	// Pure forwarder: no local records.
	server, err := dns.NewDNSServer(dns.DNSServerConfig{
		ListenAddr: "127.0.0.1:53",
		DefaultTTL: 30,
		Resolver:   upstream,
	})
	if err != nil {
		logger.WithError(err).Error("failed to create resident DNS resolver")
		return nil, nil
	}

	if err := server.Start(); err != nil {
		logger.WithError(err).Error("failed to start resident DNS resolver")
		return nil, nil
	}

	logger.Info("resident DNS resolver listening, forwarding to knot server DNS", "address", "127.0.0.1:53", "upstream", serverDNS)
	return server, func() {
		if err := server.Stop(); err != nil {
			logger.WithError(err).Warn("error stopping resident DNS resolver")
		}
	}
}
