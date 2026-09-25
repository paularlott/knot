package mcp

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"

	"github.com/paularlott/lmchatkit"
	"github.com/paularlott/mcp"
)

// Compile-time checks: remoteServerProvider is used as all three of these.
var (
	_ mcp.ToolProvider              = (*remoteServerProvider)(nil)
	_ mcp.ResourceProvider          = (*remoteServerProvider)(nil)
	_ lmchatkit.SourcedToolProvider = (*remoteServerProvider)(nil)
)

// remoteServerManager caches MCP clients per server config, keyed by server ID.
// stdio clients spawn a subprocess; HTTP clients open a connection. Both are
// expensive to create so we cache them. The cache is periodically pruned of
// clients whose configs no longer exist or are disabled.
type remoteServerManager struct {
	mu      sync.RWMutex
	clients map[string]*cachedClient // server config ID -> client
}

type cachedClient struct {
	client   *mcp.Client
	serverId string
	lastUsed time.Time
}

var remoteManager = &remoteServerManager{
	clients: make(map[string]*cachedClient),
}

// remoteServerProvider implements both mcp.ToolProvider and
// mcp.ResourceProvider for a user's configured remote MCP servers. It loads
// the user's enabled server configs from the database, creates/caches MCP
// clients, and exposes their tools and resources — the latter needed so an
// MCP Apps view (a tool linked to a ui:// resource on one of these servers)
// can actually be fetched and rendered, not just discovered and called.
type remoteServerProvider struct {
	user *model.User
}

func NewRemoteServerProvider(user *model.User) *remoteServerProvider {
	return &remoteServerProvider{user: user}
}

// enabledServers returns the user's non-deleted, enabled remote server
// configs — the common first step of GetTools, GetResources and ReadResource.
func (p *remoteServerProvider) enabledServers() ([]*model.MCPServer, error) {
	db := database.GetInstance()
	servers, err := db.GetMCPServersByUser(p.user.Id)
	if err != nil {
		return nil, err
	}
	enabled := make([]*model.MCPServer, 0, len(servers))
	for _, server := range servers {
		if !server.IsDeleted && server.Enabled {
			enabled = append(enabled, server)
		}
	}
	return enabled, nil
}

// clientFor initializes (creating/caching as needed) the MCP client for one
// server, logging and returning ok=false on any failure rather than an error
// — one unreachable server must not stop the others from being tried.
func (p *remoteServerProvider) clientFor(ctx context.Context, server *model.MCPServer) (client *mcp.Client, ok bool) {
	client, err := remoteManager.getOrCreateClient(server)
	if err != nil {
		log.WithGroup("mcp").Warn("Failed to create MCP client for user server",
			"namespace", server.Namespace, "user", p.user.Username, "error", err)
		return nil, false
	}
	if err := client.Initialize(ctx); err != nil {
		log.WithGroup("mcp").Warn("Failed to initialize MCP client for user server",
			"namespace", server.Namespace, "user", p.user.Username, "error", err)
		return nil, false
	}
	return client, true
}

// GetResources implements mcp.ResourceProvider, aggregating the resources
// (static and templates) exposed by each of the user's remote servers.
func (p *remoteServerProvider) GetResources(ctx context.Context) (*mcp.ProvidedResources, error) {
	servers, err := p.enabledServers()
	if err != nil {
		return nil, err
	}

	out := &mcp.ProvidedResources{}
	for _, server := range servers {
		client, ok := p.clientFor(ctx, server)
		if !ok {
			continue
		}
		if resources, err := client.ListResources(ctx); err == nil {
			out.Resources = append(out.Resources, resources...)
		}
		if templates, err := client.ListResourceTemplates(ctx); err == nil {
			out.Templates = append(out.Templates, templates...)
		}
	}
	return out, nil
}

// ReadResource implements mcp.ResourceProvider, trying each of the user's
// remote servers in turn since nothing about a bare uri says which server
// registered it. Best-effort per server (matching GetTools' own convention
// in this file): a server that errors or doesn't have the uri is skipped
// rather than aborting the whole lookup, so one unreachable server can't
// hide a resource that a different one actually serves.
func (p *remoteServerProvider) ReadResource(ctx context.Context, uri string) (*mcp.ResourceResponse, error) {
	servers, err := p.enabledServers()
	if err != nil {
		return nil, err
	}

	for _, server := range servers {
		client, ok := p.clientFor(ctx, server)
		if !ok {
			continue
		}
		resp, err := client.ReadResource(ctx, uri)
		if err != nil || resp == nil {
			continue
		}
		return resp, nil
	}
	return nil, mcp.ErrUnknownResource
}

func (p *remoteServerProvider) GetTools(ctx context.Context) ([]mcp.MCPTool, error) {
	servers, err := p.enabledServers()
	if err != nil {
		return nil, err
	}

	var tools []mcp.MCPTool

	for _, server := range servers {
		client, ok := p.clientFor(ctx, server)
		if !ok {
			continue
		}

		serverTools, err := client.ListTools(ctx)
		if err != nil {
			log.WithGroup("mcp").Warn("Failed to list tools from user MCP server",
				"namespace", server.Namespace, "user", p.user.Username, "error", err)
			continue
		}

		for _, tool := range serverTools {
			toolName := tool.Name
			// Strip namespace prefix if present (client adds it)
			if server.Namespace != "" {
				nsPrefix := server.Namespace + mcp.DefaultNamespaceSeparator
				toolName = strings.TrimPrefix(tool.Name, nsPrefix)
			}

			// Check disabled tools
			if !server.IsToolEnabled(toolName) {
				continue
			}

			visibility := mcp.ToolVisibilityNative
			if server.ToolVisibility == "ondemand" || server.ToolVisibility == "discoverable" {
				visibility = mcp.ToolVisibilityDiscoverable
			}

			tools = append(tools, mcp.MCPTool{
				Name:         tool.Name,
				Description:  tool.Description,
				InputSchema:  tool.InputSchema,
				OutputSchema: tool.OutputSchema,
				Meta:         tool.Meta,
				Icons:        tool.Icons,
				Visibility:   visibility,
			})
		}
	}

	return tools, nil
}

// bareNameFor reports whether name could belong to server, given its
// namespace, and the bare (un-namespaced) name to use when calling that
// server directly. Shared by ExecuteTool and ToolSource so they resolve a
// namespaced name to a server identically. Strict matching, no guessing: a
// namespaced server answers only its own ns__name, and a server with no
// namespace answers bare names only — a name carrying someone else's
// namespace never routes here.
func bareNameFor(server *model.MCPServer, name string) (bareName string, ok bool) {
	if server.Namespace == "" {
		if strings.Contains(name, mcp.DefaultNamespaceSeparator) {
			return "", false
		}
		return name, true
	}
	prefix := server.Namespace + mcp.DefaultNamespaceSeparator
	if strings.HasPrefix(name, prefix) {
		return strings.TrimPrefix(name, prefix), true
	}
	return "", false
}

func (p *remoteServerProvider) ExecuteTool(ctx context.Context, name string, params map[string]interface{}) (*mcp.ToolResponse, error) {
	// Dispatch routes by this provider's own listing (providerForTool in the
	// library), so only namespaced remote-tool names ever arrive here — the
	// old boot-loaded-mcptools first try was unreachable dead code under
	// that routing.
	// Try remote MCP servers
	db := database.GetInstance()
	servers, err := db.GetMCPServersByUser(p.user.Id)
	if err != nil {
		return nil, err
	}

	for _, server := range servers {
		if server.IsDeleted || !server.Enabled {
			continue
		}

		toolName, ok := bareNameFor(server, name)
		if !ok {
			continue
		}

		// Check disabled tools
		if !server.IsToolEnabled(toolName) {
			continue
		}

		client, err := remoteManager.getOrCreateClient(server)
		if err != nil {
			continue
		}

		if err := client.Initialize(ctx); err != nil {
			continue
		}

		resp, err := client.CallTool(ctx, toolName, params)
		if err != nil {
			// Tool not found on this server — try next
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("tool not found: %s", name)
}

// ToolSource and ReadResourceFromSource together implement
// lmchatkit.SourcedToolProvider: since this provider is attached per-request
// via mcp.WithToolProviders (not registered on the shared *mcp.Server),
// mcp.Server's own ToolSource/ReadResourceFrom have no visibility into it at
// all — without these, lmchatkit's /api/resources/read always 403'd for any
// tool from a user's remote server, which chat.js's hydrateAppResource
// silently treats as "no app renders" rather than an error. The server's DB
// ID is used as the opaque source identifier: stable and unique, unlike
// Namespace, which can be empty or shared across servers.

func (p *remoteServerProvider) ToolSource(ctx context.Context, name string) (string, bool) {
	servers, err := p.enabledServers()
	if err != nil {
		return "", false
	}
	for _, server := range servers {
		if _, ok := bareNameFor(server, name); ok {
			return server.Id, true
		}
	}
	return "", false
}

func (p *remoteServerProvider) ReadResourceFromSource(ctx context.Context, source, uri string) (*mcp.ResourceResponse, error) {
	servers, err := p.enabledServers()
	if err != nil {
		return nil, err
	}
	for _, server := range servers {
		if server.Id != source {
			continue
		}
		client, ok := p.clientFor(ctx, server)
		if !ok {
			return nil, mcp.ErrUnknownResource
		}
		return client.ReadResource(ctx, uri)
	}
	return nil, mcp.ErrUnknownResource
}

func (m *remoteServerManager) getOrCreateClient(server *model.MCPServer) (*mcp.Client, error) {
	m.mu.RLock()
	if cc, ok := m.clients[server.Id]; ok {
		m.mu.RUnlock()
		cc.lastUsed = time.Now()
		return cc.client, nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if cc, ok := m.clients[server.Id]; ok {
		cc.lastUsed = time.Now()
		return cc.client, nil
	}

	client, err := createClient(server)
	if err != nil {
		return nil, err
	}

	m.clients[server.Id] = &cachedClient{
		client:   client,
		serverId: server.Id,
		lastUsed: time.Now(),
	}

	return client, nil
}

func createClient(server *model.MCPServer) (*mcp.Client, error) {
	// stdio server
	if server.Command != "" {
		client, err := mcp.NewStdioClient(server.Command, server.Args, server.Namespace, mcp.WithClientExtraEnv(server.Env...))
		if err != nil {
			return nil, fmt.Errorf("failed to create stdio client for %s: %w", server.Namespace, err)
		}
		declareUIAppsSupport(client)
		return client, nil
	}

	// HTTP server
	var auth mcp.AuthProvider
	if server.AuthType == "oauth2" {
		auth = mcp.NewOAuth2RefreshTokenAuth(server.OAuthTokenURL, server.OAuthClientID, server.OAuthAccessToken, server.OAuthRefreshToken)
	} else if server.Token != "" {
		auth = mcp.NewBearerTokenAuth(server.Token)
	}

	normalizedURL := strings.TrimSuffix(server.URL, "/")
	client := mcp.NewClient(normalizedURL, auth, server.Namespace)

	// Notifications are always enabled — listChanged events keep tool caches fresh.
	client.EnableNotifications()
	declareUIAppsSupport(client)

	return client, nil
}

// declareUIAppsSupport advertises this client's own support for the MCP
// Apps extension (SEP-1865) to a remote server, mirroring llmrouter's own
// declareUIAppsSupport. Without this, a spec-conformant remote server that
// only attaches _meta.ui for clients that declared
// capabilities.extensions[io.modelcontextprotocol/ui] has no way to know
// knot can render one, and silently serves a plain-text-only tool instead —
// MCP Apps then quietly never works for that server, with no error anywhere
// to explain why.
func declareUIAppsSupport(client *mcp.Client) {
	client.DeclareExtension(mcp.UIAppsExtensionID, map[string]any{
		"mimeTypes": []string{mcp.UIAppMimeType},
	})
}

// ListRemoteServerTools connects to the remote MCP server and returns its tool list.
// Used by the API endpoint for the tools management UI.
func ListRemoteServerTools(server *model.MCPServer) ([]mcp.MCPTool, error) {
	client, err := remoteManager.getOrCreateClient(server)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.Initialize(ctx); err != nil {
		return nil, err
	}

	tools, err := client.ListTools(ctx)
	if err != nil {
		return nil, err
	}

	return tools, nil
}

// GetRemoteServerProtocolVersion connects to the remote MCP server (if not
// already connected) and returns the MCP protocol version it actually
// negotiated — e.g. "2025-06-18" for a legacy server, or the modern era's
// fixed revision. Used by the API endpoint for the server management UI.
func GetRemoteServerProtocolVersion(server *model.MCPServer) (string, error) {
	client, err := remoteManager.getOrCreateClient(server)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.Initialize(ctx); err != nil {
		return "", err
	}

	return client.ProtocolVersion(), nil
}
