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

// newKnotMCPServer builds one knot MCP server instance: name, version,
// instructions and the MCP Apps (SEP-1865) capability declaration, which
// script tools can carry a _meta.ui link (see ScriptToolsProvider). Doesn't
// gate whether _meta.ui is attached to a tool descriptor — that happens
// unconditionally, per this library's own guidance for an HTTP, multi-tenant
// server — purely so the server's declared capabilities accurately reflect
// what it offers.
func newKnotMCPServer() *mcp.Server {
	server := mcp.NewServer("knot-mcp-server", build.Version)
	server.SetInstructions(`These tools manage spaces, templates, and other resources.

All tools are directly callable on the /mcp endpoint.`)
	server.DeclareExtension(mcp.UIAppsExtensionID, map[string]any{
		"mimeTypes": []string{mcp.UIAppMimeType},
	})
	return server
}

// InitializeMCPServer builds knot's two MCP server instances and registers
// the public /mcp HTTP endpoint.
//
// The endpoint server serves ONLY knot's own tools: script and method tool
// providers, attached per request. Remote MCP servers — operator-configured
// (server.mcp.remote_servers) and user-configured alike — are deliberately
// NOT federated through the public endpoint: an external MCP client that
// wants another server's tools connects to that server directly, and knot's
// name and tool list describe knot alone. The returned server is the
// internal one used by knot's own AI surfaces (web chat, OpenAI-compatible
// endpoints, scriptling's knot.mcp), which keep remote servers registered —
// those consumers have no way to attach to servers themselves.
func InitializeMCPServer(routes *http.ServeMux, enableWebEndpoint bool, mcpConfig *config.MCPConfig) *mcp.Server {
	// Debug: Log what we actually received
	if mcpConfig != nil && len(mcpConfig.RemoteServers) > 0 {
		for i, rs := range mcpConfig.RemoteServers {
			log.WithGroup("mcp").Info("RemoteServer config", "index", i, "namespace", rs.Namespace, "url", rs.URL, "tool_visibility", rs.ToolVisibility)
		}
	}

	// The public /mcp endpoint's server: knot's own tools only.
	endpointServer := newKnotMCPServer()

	// The internal server for knot's own AI surfaces: same natives, plus
	// remote server federation (registered below).
	server := newKnotMCPServer()

	if enableWebEndpoint {
		// Create unified handler for /mcp endpoint
		// Mode is determined from X-MCP-Show-All header or show_all query parameter
		unifiedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// The authentication middleware has already run and set the user in the context
			user := r.Context().Value("user").(*model.User)

			// Add request-scoped tool providers (order preserved: scripts then
			// methods). No remote server provider here — the public endpoint
			// exposes knot's own tools only (see InitializeMCPServer's comment).
			var providers []mcp.ToolProvider
			if user != nil && (user.HasPermission(model.PermissionExecuteScripts) || user.HasPermission(model.PermissionExecuteOwnScripts)) {
				providers = append(providers, NewScriptToolsProvider(user))
			}
			if user != nil {
				providers = append(providers, NewMethodToolsProvider(user))
			}

			// Attach providers and apply show-all mode (X-MCP-Show-All / ?show_all) in one step
			ctx := mcp.WithShowAllFromRequest(r.Context(), r, providers...)

			// Handle the MCP request
			endpointServer.HandleRequest(w, r.WithContext(ctx))
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
	// Register remote MCP servers if configured — on the internal server only
	// (web chat, OpenAI endpoints, scriptling), never on the public /mcp
	// endpoint.
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

			// Advertise this client's own support for the MCP Apps extension
			// to the remote server (see declareUIAppsSupport in remote_servers.go).
			declareUIAppsSupport(client)

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
					if strings.Contains(tool.Name, remoteServer.Namespace+mcp.DefaultNamespaceSeparator) {
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
