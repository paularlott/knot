package mcp

import (
	"github.com/paularlott/mcp"
)

// mcpServer holds a reference to the application's MCP server so that other
// packages (e.g. the script CRUD handlers) can trigger listChanged
// notifications without the server being threaded through every call. It is
// set once via SetServer during startup.
var mcpServer *mcp.Server

// SetServer records the application's MCP server so its notification helpers
// are reachable from anywhere. Safe to call once at startup.
func SetServer(s *mcp.Server) {
	mcpServer = s
}

// NotifyToolsChanged emits a notifications/tools/listChanged to every connected
// MCP client, signalling that its cached tool list is stale. It is a no-op (and
// cheap) when no server is set or no clients are connected. Call this when the
// data behind a tool provider changes (e.g. a user's scripts are created,
// updated, or deleted).
func NotifyToolsChanged() {
	if mcpServer == nil {
		return
	}
	mcpServer.NotifyToolsChanged()
}

// NotifyResourcesChanged emits a notifications/resources/listChanged.
func NotifyResourcesChanged() {
	if mcpServer == nil {
		return
	}
	mcpServer.NotifyResourcesChanged()
}

// NotifyPromptsChanged emits a notifications/prompts/listChanged.
func NotifyPromptsChanged() {
	if mcpServer == nil {
		return
	}
	mcpServer.NotifyPromptsChanged()
}
