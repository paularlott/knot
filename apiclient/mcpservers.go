package apiclient

import "context"

type MCPServerInfo struct {
	Id                string   `json:"mcp_server_id"`
	UserId            string   `json:"user_id"`
	Namespace         string   `json:"namespace"`
	URL               string   `json:"url"`
	Command           string   `json:"command"`
	Args              []string `json:"args"`
	Enabled           bool     `json:"enabled"`
	ToolVisibility    string   `json:"tool_visibility"`
	DisabledTools     []string `json:"disabled_tools"`
	RemoteSearch      bool     `json:"remote_search"`
}

type MCPServerDetails struct {
	Id                string   `json:"mcp_server_id"`
	UserId            string   `json:"user_id"`
	Namespace         string   `json:"namespace"`
	URL               string   `json:"url"`
	Command           string   `json:"command"`
	Args              []string `json:"args"`
	AuthType          string   `json:"auth_type"`
	Token             string   `json:"token"`
	OAuthClientID     string   `json:"oauth_client_id"`
	OAuthTokenURL     string   `json:"oauth_token_url"`
	OAuthAccessToken  string   `json:"oauth_access_token"`
	OAuthRefreshToken string   `json:"oauth_refresh_token"`
	Enabled           bool     `json:"enabled"`
	ToolVisibility    string   `json:"tool_visibility"`
	DisabledTools     []string `json:"disabled_tools"`
	RemoteSearch      bool     `json:"remote_search"`
}

type MCPServerList struct {
	Count  int              `json:"count"`
	Servers []MCPServerInfo `json:"servers"`
}

type MCPServerCreateRequest struct {
	Namespace         string   `json:"namespace"`
	URL               string   `json:"url"`
	Command           string   `json:"command"`
	Args              []string `json:"args"`
	AuthType          string   `json:"auth_type"`
	Token             string   `json:"token"`
	OAuthClientID     string   `json:"oauth_client_id"`
	OAuthTokenURL     string   `json:"oauth_token_url"`
	OAuthAccessToken  string   `json:"oauth_access_token"`
	OAuthRefreshToken string   `json:"oauth_refresh_token"`
	Enabled           bool     `json:"enabled"`
	ToolVisibility    string   `json:"tool_visibility"`
	DisabledTools     []string `json:"disabled_tools"`
	RemoteSearch      bool     `json:"remote_search"`
}

type MCPServerUpdateRequest struct {
	Namespace         string   `json:"namespace"`
	URL               string   `json:"url"`
	Command           string   `json:"command"`
	Args              []string `json:"args"`
	AuthType          string   `json:"auth_type"`
	Token             string   `json:"token"`
	OAuthClientID     string   `json:"oauth_client_id"`
	OAuthTokenURL     string   `json:"oauth_token_url"`
	OAuthAccessToken  string   `json:"oauth_access_token"`
	OAuthRefreshToken string   `json:"oauth_refresh_token"`
	Enabled           bool     `json:"enabled"`
	ToolVisibility    string   `json:"tool_visibility"`
	DisabledTools     []string `json:"disabled_tools"`
	RemoteSearch      bool     `json:"remote_search"`
}

type MCPServerCreateResponse struct {
	Status bool   `json:"status"`
	Id     string `json:"mcp_server_id"`
}

type MCPServerToggleToolRequest struct {
	ToolName string `json:"tool_name"`
	Enabled  bool   `json:"enabled"`
}

func (c *ApiClient) GetMCPServers(ctx context.Context, userId string) (*MCPServerList, error) {
	var list MCPServerList
	path := "/api/mcp-servers"
	if userId != "" {
		path += "?user_id=" + userId
	}
	_, err := c.httpClient.Get(ctx, path, &list)
	return &list, err
}

func (c *ApiClient) GetMCPServer(ctx context.Context, id string) (*MCPServerDetails, error) {
	var server MCPServerDetails
	_, err := c.httpClient.Get(ctx, "/api/mcp-servers/"+id, &server)
	return &server, err
}

func (c *ApiClient) CreateMCPServer(ctx context.Context, req *MCPServerCreateRequest) (*MCPServerCreateResponse, error) {
	var resp MCPServerCreateResponse
	_, err := c.httpClient.Post(ctx, "/api/mcp-servers", req, &resp, 201)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *ApiClient) UpdateMCPServer(ctx context.Context, id string, req *MCPServerUpdateRequest) error {
	_, err := c.httpClient.Put(ctx, "/api/mcp-servers/"+id, req, nil, 200)
	return err
}

func (c *ApiClient) DeleteMCPServer(ctx context.Context, id string) error {
	_, err := c.httpClient.Delete(ctx, "/api/mcp-servers/"+id, nil, nil, 200)
	return err
}

func (c *ApiClient) ToggleMCPServerTool(ctx context.Context, id string, req *MCPServerToggleToolRequest) error {
	_, err := c.httpClient.Post(ctx, "/api/mcp-servers/"+id+"/toggle-tool", req, nil, 200)
	return err
}
