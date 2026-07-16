package mcp

import (
	"context"
	"net/http"
	"strings"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/middleware"

	"github.com/paularlott/mcp"
)

func InitializeMCPServer(routes *http.ServeMux, enableWebEndpoint bool, mcpConfig *config.MCPConfig) *mcp.Server {
	// Debug: Log what we actually received
	if mcpConfig != nil && len(mcpConfig.RemoteServers) > 0 {
		for i, rs := range mcpConfig.RemoteServers {
			log.WithGroup("mcp").Info("RemoteServer config", "index", i, "namespace", rs.Namespace, "url", rs.URL, "tool_visibility", rs.ToolVisibility)
		}
	}

	// Create the main unified MCP server
	server := mcp.NewServer("knot-mcp-server", build.Version)
	server.SetInstructions(`These tools manage spaces, templates, and other resources.

All tools are directly callable on the /mcp endpoint.
Use tool_search to discover tools by keyword or description.`)

	if enableWebEndpoint {
		// Create unified handler for /mcp endpoint
		// Mode is determined from X-MCP-Show-All header or show_all query parameter
		unifiedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// The authentication middleware has already run and set the user in the context
			user := r.Context().Value("user").(*model.User)

			// Add request-scoped tool providers (order preserved: scripts then methods)
			var providers []mcp.ToolProvider
			if user != nil && (user.HasPermission(model.PermissionExecuteScripts) || user.HasPermission(model.PermissionExecuteOwnScripts)) {
				providers = append(providers, NewScriptToolsProvider(user))
			}
			if user != nil {
				providers = append(providers, NewMethodToolsProvider(user))
				providers = append(providers, NewRemoteServerProvider(user))
			}

			// Attach providers and apply show-all mode (X-MCP-Show-All / ?show_all) in one step
			ctx := mcp.WithShowAllFromRequest(r.Context(), r, providers...)

			// Handle the MCP request
			server.HandleRequest(w, r.WithContext(ctx))
		})

		// Apply authentication middleware - unified endpoint
		// Use X-MCP-Show-All: true header for show-all mode
		routes.HandleFunc("POST /mcp", middleware.ApiAuth(middleware.ApiPermissionUseMCPServer(unifiedHandler.ServeHTTP)))
	}

	// =========================================================================
	// Register Go-based tools (most tools are now scripted via mcptools)
	// =========================================================================

	// Skills tool is now dynamically registered via provider

	// =========================================================================
	// Register remote MCP servers if configured
	// =========================================================================
	if mcpConfig != nil && len(mcpConfig.RemoteServers) > 0 {
		for _, remoteServer := range mcpConfig.RemoteServers {
			// Build the client for the remote server: stdio (Command set) or HTTP (URL set).
			var client *mcp.Client
		if remoteServer.Command != "" {
			c, err := mcp.NewStdioClient(remoteServer.Command, remoteServer.Args, remoteServer.Namespace, mcp.WithClientExtraEnv(remoteServer.Env...))
				if err != nil {
					log.WithGroup("mcp").Error("Failed to launch stdio MCP server", "namespace", remoteServer.Namespace, "command", remoteServer.Command, "error", err)
					continue
				}
				client = c
				log.WithGroup("mcp").Info("Connected to stdio MCP server", "namespace", remoteServer.Namespace, "command", remoteServer.Command)
			} else {
				authProvider := CreateAuthProvider(remoteServer)
				if authProvider == nil {
					continue // Skip if auth provider creation failed
				}
				client = mcp.NewClient(remoteServer.URL, authProvider, remoteServer.Namespace)
			}

			// Opt the client into notifications: an HTTP client opens an SSE reader,
			// and the propagation hook (installed by Register*) re-emits upstream
			// listChanged events to our own clients. stdio clients always receive
			// notifications, so this is a harmless no-op for them.
			if remoteServer.Notifications {
				client.EnableNotifications()
			}

			// Determine tool visibility mode (default to "native" if not specified)
			visibility := strings.TrimSpace(remoteServer.ToolVisibility)
			if visibility == "" {
				visibility = "native"
			}
			// Normalize legacy "on-demand" to "discoverable"
			if visibility == "on-demand" {
				visibility = "discoverable"
			}
			log.WithGroup("mcp").Info("Processing remote server", "namespace", remoteServer.Namespace, "url", remoteServer.URL, "command", remoteServer.Command, "tool_visibility_config", remoteServer.ToolVisibility, "tool_visibility_resolved", visibility, "notifications", remoteServer.Notifications)

			// Register based on visibility setting
			var err error
			if visibility == "discoverable" {
				// Discoverable mode: tools only available via tool_search, not in tools/list
				err = server.RegisterRemoteServerDiscoverable(client)
				log.WithGroup("mcp").Info("Registered remote MCP server", "namespace", remoteServer.Namespace, "url", remoteServer.URL, "command", remoteServer.Command, "mode", "discoverable")
			} else {
				// Native mode: tools visible in tools/list
				err = server.RegisterRemoteServer(client)
				log.WithGroup("mcp").Info("Registered remote MCP server", "namespace", remoteServer.Namespace, "url", remoteServer.URL, "command", remoteServer.Command, "mode", "native")
			}

			if err != nil {
				log.WithGroup("mcp").Error("Failed to register remote MCP server", "namespace", remoteServer.Namespace, "url", remoteServer.URL, "command", remoteServer.Command, "visibility", visibility, "error", err)
				continue
			}

			// Test if we can list tools from the remote server (only if native mode)
			if visibility == "native" {
				tools := server.ListToolsWithContext(context.Background())
				remoteToolCount := 0
				for _, tool := range tools {
					if strings.Contains(tool.Name, remoteServer.Namespace+".") {
						remoteToolCount++
					}
				}
				if remoteToolCount == 0 {
					log.WithGroup("mcp").Warn("No tools loaded from remote MCP server (may be unreachable or have auth issues)", "namespace", remoteServer.Namespace, "url", remoteServer.URL, "command", remoteServer.Command)
				} else {
					log.WithGroup("mcp").Info("Remote server tool count", "namespace", remoteServer.Namespace, "count", remoteToolCount)
				}
			}
		}

		// Log total tools after registration
		totalTools := server.ListToolsWithContext(context.Background())
		log.WithGroup("mcp").Info("Total tools after remote registration", "count", len(totalTools))
	}

	// Log info
	log.WithGroup("mcp").Info("MCP server initialized")

	return server
}
