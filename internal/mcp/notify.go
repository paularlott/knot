package mcp

import (
	"sync"

	"github.com/paularlott/mcp"
)

// notifyServers holds the application's MCP servers so that other packages
// (e.g. the script CRUD handlers) can trigger list_changed notifications
// without the servers being threaded through every call. InitializeMCPServer
// records both of knot's servers: the public /mcp endpoint server, which
// external MCP clients connect to, and the internal one behind knot's own AI
// surfaces. Each is notified, as either may have connected clients.
var (
	notifyMu      sync.RWMutex
	notifyServers []*mcp.Server
)

// SetServer records the MCP servers the notification helpers reach,
// replacing any recorded before. nil entries are ignored.
func SetServer(servers ...*mcp.Server) {
	var list []*mcp.Server
	for _, s := range servers {
		if s != nil {
			list = append(list, s)
		}
	}
	notifyMu.Lock()
	notifyServers = list
	notifyMu.Unlock()
}

func eachServer(fn func(*mcp.Server)) {
	notifyMu.RLock()
	servers := notifyServers
	notifyMu.RUnlock()
	for _, s := range servers {
		fn(s)
	}
}

// NotifyToolsChanged emits a notifications/tools/list_changed to every
// connected MCP client of every recorded server, signalling that its cached
// tool list is stale. It is a no-op (and cheap) when no server is set or no
// clients are connected. Call this when the data behind a tool provider
// changes (e.g. a user's scripts are created, updated, or deleted).
func NotifyToolsChanged() {
	eachServer((*mcp.Server).NotifyToolsChanged)
}

// NotifyResourcesChanged emits a notifications/resources/list_changed.
func NotifyResourcesChanged() {
	eachServer((*mcp.Server).NotifyResourcesChanged)
}

// NotifyPromptsChanged emits a notifications/prompts/list_changed.
func NotifyPromptsChanged() {
	eachServer((*mcp.Server).NotifyPromptsChanged)
}
