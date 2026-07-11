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
	"github.com/paularlott/knot/internal/mcptools"

	"github.com/paularlott/mcp"
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

// remoteServerProvider implements mcp.ToolProvider for a user's configured
// remote MCP servers. It loads the user's enabled server configs from the
// database, creates/caches MCP clients, and exposes their tools.
type remoteServerProvider struct {
	user *model.User
}

func NewRemoteServerProvider(user *model.User) mcp.ToolProvider {
	return &remoteServerProvider{user: user}
}

func (p *remoteServerProvider) GetTools(ctx context.Context) ([]mcp.MCPTool, error) {
	db := database.GetInstance()
	servers, err := db.GetMCPServersByUser(p.user.Id)
	if err != nil {
		return nil, err
	}

	var tools []mcp.MCPTool

	for _, server := range servers {
		if server.IsDeleted || !server.Enabled {
			continue
		}

		client, err := remoteManager.getOrCreateClient(server)
		if err != nil {
			log.WithGroup("mcp").Warn("Failed to create MCP client for user server",
				"namespace", server.Namespace, "user", p.user.Username, "error", err)
			continue
		}

		if err := client.Initialize(ctx); err != nil {
			log.WithGroup("mcp").Warn("Failed to initialize MCP client for user server",
				"namespace", server.Namespace, "user", p.user.Username, "error", err)
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
				Name:        tool.Name,
				Description: tool.Description,
				InputSchema: tool.InputSchema,
				Keywords:    tool.Keywords,
				Visibility:  visibility,
			})
		}
	}

	return tools, nil
}

func (p *remoteServerProvider) ExecuteTool(ctx context.Context, name string, params map[string]interface{}) (*mcp.ToolResponse, error) {
	// Try boot-loaded tools first
	toolResult, toolErr := mcptools.ExecuteTool(name, params, p.user)
	if toolErr == nil {
		return mcp.NewToolResponseAuto(toolResult), nil
	}
	if _, exists := mcptools.GetTool(name); exists {
		return nil, toolErr
	}

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

		// Determine the namespaced tool name
		nsPrefix := ""
		if server.Namespace != "" {
			nsPrefix = server.Namespace + mcp.DefaultNamespaceSeparator
		}

		var toolName string
		if strings.HasPrefix(name, nsPrefix) {
			toolName = strings.TrimPrefix(name, nsPrefix)
		} else if name == server.Namespace || !strings.Contains(name, mcp.DefaultNamespaceSeparator) {
			// No namespace prefix — try matching directly
			toolName = name
		} else {
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
		client, err := mcp.NewStdioClient(server.Command, server.Args, server.Namespace)
		if err != nil {
			return nil, fmt.Errorf("failed to create stdio client for %s: %w", server.Namespace, err)
		}
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

	return client, nil
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
