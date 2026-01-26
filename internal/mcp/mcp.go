package mcp

import (
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
		// Mode is determined from X-MCP-Tool-Mode header or tool_mode query parameter
		// In discovery mode, only tool_search/execute_tool are visible (all others are searchable)
		unifiedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// The authentication middleware has already run and set the user in the context
			user := r.Context().Value("user").(*model.User)

			// Add script tools as request-scoped provider
			ctx := r.Context()
			if user != nil && (user.HasPermission(model.PermissionExecuteScripts) || user.HasPermission(model.PermissionExecuteOwnScripts)) {
				nativeProvider := NewScriptToolsProvider(user)
				onDemandProvider := NewOnDemandScriptToolsProvider(user)
				ctx = mcp.WithToolProviders(ctx, nativeProvider)
				ctx = mcp.WithOnDemandToolProviders(ctx, onDemandProvider)
			}

			// Handle the MCP request - mode is determined from header by MCP server
			server.HandleRequest(w, r.WithContext(ctx))
		})

		// Apply authentication middleware - unified endpoint
		// Use X-MCP-Tool-Mode: discovery header for discovery mode
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
			// Create auth provider
			authProvider := CreateAuthProvider(remoteServer)
			if authProvider == nil {
				continue // Skip if auth provider creation failed
			}

			// Determine tool visibility mode (default to "native" if not specified)
			visibility := strings.TrimSpace(remoteServer.ToolVisibility)
			if visibility == "" {
				visibility = "native"
			}
			log.WithGroup("mcp").Info("Processing remote server", "namespace", remoteServer.Namespace, "url", remoteServer.URL, "tool_visibility_config", remoteServer.ToolVisibility, "tool_visibility_resolved", visibility)

			// Create MCP client for remote server
			client := mcp.NewClient(remoteServer.URL, authProvider, remoteServer.Namespace)

			// Register based on visibility setting
			var err error
			if visibility == "ondemand" {
				// OnDemand mode: tools only available via tool_search, not in tools/list
				err = server.RegisterRemoteServerOnDemand(client)
				log.WithGroup("mcp").Info("Registered remote MCP server", "namespace", remoteServer.Namespace, "url", remoteServer.URL, "mode", "ondemand")
			} else {
				// Native mode: tools visible in tools/list
				err = server.RegisterRemoteServer(client)
				log.WithGroup("mcp").Info("Registered remote MCP server", "namespace", remoteServer.Namespace, "url", remoteServer.URL, "mode", "native")
			}

			if err != nil {
				log.WithGroup("mcp").Error("Failed to register remote MCP server", "namespace", remoteServer.Namespace, "url", remoteServer.URL, "visibility", visibility, "error", err)
				continue
			}

			// Test if we can list tools from the remote server (only if native mode)
			if visibility == "native" {
				tools := server.ListTools()
				remoteToolCount := 0
				for _, tool := range tools {
					if strings.Contains(tool.Name, remoteServer.Namespace+"/") {
						remoteToolCount++
					}
				}
				if remoteToolCount == 0 {
					log.WithGroup("mcp").Warn("No tools loaded from remote MCP server (may be unreachable or have auth issues)", "namespace", remoteServer.Namespace, "url", remoteServer.URL)
				} else {
					log.WithGroup("mcp").Info("Remote server tool count", "namespace", remoteServer.Namespace, "count", remoteToolCount)
				}
			}
		}

		// Log total tools after registration
		totalTools := server.ListTools()
		log.WithGroup("mcp").Info("Total tools after remote registration", "count", len(totalTools))
	}

	// Log info
	log.WithGroup("mcp").Info("MCP server initialized")

	return server
}
