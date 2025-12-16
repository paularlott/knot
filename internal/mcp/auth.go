package mcp

import (
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/mcp"
)

// CreateAuthProvider creates an MCP authentication provider from configuration
func CreateAuthProvider(remoteServer config.MCPRemoteServerConfig) mcp.AuthProvider {
	if remoteServer.Token == "" {
		log.Error("No token provided for remote MCP server", "namespace", remoteServer.Namespace)
		return nil
	}

	return mcp.NewBearerTokenAuth(remoteServer.Token)
}