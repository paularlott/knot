package agentlink

import (
	"net"
	"sort"

	"github.com/paularlott/knot/internal/agenttunnel"
	"github.com/paularlott/knot/internal/log"
)

func handleListTunnels(conn net.Conn, msg *CommandMsg) {
	entries := agenttunnel.List()

	response := ListTunnelsResponse{
		Tunnels: make([]TunnelInfo, 0, len(entries)),
	}
	for _, entry := range entries {
		response.Tunnels = append(response.Tunnels, TunnelInfo{
			Port:     entry.Port,
			Protocol: entry.Protocol,
			Name:     entry.Name,
			URL:      entry.URL,
		})
	}

	// Stable order by name for consistent CLI output.
	sort.Slice(response.Tunnels, func(i, j int) bool {
		return response.Tunnels[i].Name < response.Tunnels[j].Name
	})

	if err := sendMsg(conn, CommandNil, response); err != nil {
		log.WithError(err).Error("Failed to send tunnel list response")
	}
}
