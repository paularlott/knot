package agenttunnel

import (
	"fmt"
	"strings"

	"github.com/paularlott/knot/internal/tunnel_server"
)

// CreateWebTunnel creates a web tunnel client, connects it, and registers it
// under the given name. It returns the public tunnel URL.
//
// serverURL is normalised (scheme added if missing, trailing slash trimmed) and
// the WS URL is derived from it. This is shared by the agentlink (in-space CLI)
// and the server-relay (remote desktop) control paths so both populate the same
// registry identically.
func CreateWebTunnel(name, protocol string, port uint16, tlsName string, tlsSkipVerify bool, serverURL, token string, skipTLSVerify bool) (string, error) {
	if protocol != "http" && protocol != "https" {
		return "", fmt.Errorf("invalid protocol, must be http or https")
	}
	if name == "" {
		return "", fmt.Errorf("tunnel name is required")
	}
	if port < 1 {
		return "", fmt.Errorf("invalid port")
	}
	if IsTunneled(name) {
		return "", fmt.Errorf("a tunnel with this name already exists")
	}

	httpServer := serverURL
	if !strings.HasPrefix(httpServer, "http://") && !strings.HasPrefix(httpServer, "https://") {
		httpServer = "https://" + httpServer
	}
	httpServer = strings.TrimSuffix(httpServer, "/")
	wsServer := "ws" + httpServer[4:]

	client := tunnel_server.NewTunnelClient(wsServer, httpServer, token, skipTLSVerify, &tunnel_server.TunnelOpts{
		Type:          tunnel_server.WebTunnel,
		Protocol:      protocol,
		LocalPort:     port,
		TunnelName:    name,
		TlsName:       tlsName,
		TlsSkipVerify: tlsSkipVerify,
	})
	if err := client.ConnectAndServe(); err != nil {
		return "", fmt.Errorf("failed to create tunnel: %w", err)
	}

	if _, ok := Start(name, port, protocol, client.URL(), client); !ok {
		client.Shutdown()
		return "", fmt.Errorf("a tunnel with this name already exists")
	}
	return client.URL(), nil
}
