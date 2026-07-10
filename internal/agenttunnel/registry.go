package agenttunnel

import (
	"sync"

	"github.com/paularlott/knot/internal/tunnel_server"
)

// Shared state for web tunnels owned by the agent daemon (started via the
// `knot tunnel <proto> --daemon` agentlink path). Tunnels live for the lifetime
// of the agent process; none are persisted.
//
// Keyed by tunnel name (the <name> in <user>--<name>.<domain>): the name is the
// tunnel's identity, matching the server-side tunnels map, the DELETE API and
// the web UI. A given name may only have one daemon tunnel at a time.
var (
	tunnelsMux sync.RWMutex
	tunnels    = make(map[string]*Entry)
)

// Entry holds information about an active web tunnel owned by the daemon.
type Entry struct {
	Name     string
	Port     uint16
	Protocol string
	URL      string
	Client   *tunnel_server.TunnelClient
}

// Start registers a running web tunnel. It returns false if the name already
// has a tunnel.
func Start(name string, port uint16, protocol, url string, client *tunnel_server.TunnelClient) (*Entry, bool) {
	tunnelsMux.Lock()
	defer tunnelsMux.Unlock()

	if _, exists := tunnels[name]; exists {
		return nil, false
	}

	entry := &Entry{
		Name:     name,
		Port:     port,
		Protocol: protocol,
		URL:      url,
		Client:   client,
	}
	tunnels[name] = entry
	return entry, true
}

// Stop shuts down and removes the tunnel with the given name. It returns false
// if no tunnel exists for the name.
func Stop(name string) bool {
	tunnelsMux.Lock()
	defer tunnelsMux.Unlock()

	entry, exists := tunnels[name]
	if !exists {
		return false
	}

	if entry.Client != nil {
		entry.Client.Shutdown()
	}
	delete(tunnels, name)
	return true
}

// Get returns the tunnel entry for a name, if any.
func Get(name string) (*Entry, bool) {
	tunnelsMux.RLock()
	defer tunnelsMux.RUnlock()

	entry, exists := tunnels[name]
	return entry, exists
}

// List returns all active daemon-owned tunnels.
func List() []*Entry {
	tunnelsMux.RLock()
	defer tunnelsMux.RUnlock()

	result := make([]*Entry, 0, len(tunnels))
	for _, entry := range tunnels {
		result = append(result, entry)
	}
	return result
}

// IsTunneled reports whether the name already has a daemon-owned tunnel.
func IsTunneled(name string) bool {
	tunnelsMux.RLock()
	defer tunnelsMux.RUnlock()

	_, exists := tunnels[name]
	return exists
}

// StopAll shuts down every daemon-owned tunnel. Called at agent shutdown.
func StopAll() {
	tunnelsMux.Lock()
	defer tunnelsMux.Unlock()

	for _, entry := range tunnels {
		if entry.Client != nil {
			entry.Client.Shutdown()
		}
	}
	tunnels = make(map[string]*Entry)
}
