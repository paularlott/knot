package apiclient

import "context"

type TunnelInfo struct {
	Name    string `json:"name" msgpack:"name"`
	Address string `json:"address" msgpack:"address"`
}

type TunnelServerInfo struct {
	Domain        string   `json:"domain" msgpack:"domain"`
	TunnelServers []string `json:"tunnel_servers" msgpack:"tunnel_servers"`
}

func (c *ApiClient) GetTunnels(ctx context.Context) ([]TunnelInfo, int, error) {
	response := []TunnelInfo{}

	code, err := c.httpClient.Get(ctx, "/api/tunnels", &response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) GetTunnelServerInfo(ctx context.Context) (*TunnelServerInfo, int, error) {
	response := &TunnelServerInfo{}

	code, err := c.httpClient.Get(ctx, "/api/tunnels/server-info", &response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

// --- Remote web-tunnel management (drives a space's agent-owned tunnels) ---

type SpaceTunnelStartRequest struct {
	Protocol string `json:"protocol" msgpack:"protocol"`
	Port     uint16 `json:"port" msgpack:"port"`
	Name     string `json:"name" msgpack:"name"`

	// Server/Token target a knot server other than the one that owns the
	// space; both must be given together, and both empty means the space's
	// own server. ServerTlsSkipVerify carries the TLS choice for the
	// connection to that server.
	Server              string `json:"server,omitempty" msgpack:"server,omitempty"`
	Token               string `json:"token,omitempty" msgpack:"token,omitempty"`
	ServerTlsSkipVerify bool   `json:"server_tls_skip_verify" msgpack:"server_tls_skip_verify"`
}

type SpaceTunnelStartResponse struct {
	Success bool   `json:"success" msgpack:"success"`
	Error   string `json:"error,omitempty" msgpack:"error,omitempty"`
	URL     string `json:"url,omitempty" msgpack:"url,omitempty"`
}

type SpaceTunnelInfo struct {
	Port     uint16 `json:"port" msgpack:"port"`
	Protocol string `json:"protocol" msgpack:"protocol"`
	Name     string `json:"name" msgpack:"name"`
	URL      string `json:"url" msgpack:"url"`
}

type SpaceTunnelListResponse struct {
	Tunnels []SpaceTunnelInfo `json:"tunnels" msgpack:"tunnels"`
}

type SpaceTunnelStopRequest struct {
	Name string `json:"name" msgpack:"name"`
}

// StartSpaceTunnel starts an agent-owned web tunnel inside a space.
func (c *ApiClient) StartSpaceTunnel(ctx context.Context, spaceId string, request *SpaceTunnelStartRequest) (*SpaceTunnelStartResponse, int, error) {
	response := &SpaceTunnelStartResponse{}
	code, err := c.httpClient.Post(ctx, "/space-io/"+spaceId+"/tunnel/start", request, response, 200)
	if err != nil {
		return nil, code, err
	}
	return response, code, nil
}

// ListSpaceTunnels lists the agent-owned web tunnels in a space.
func (c *ApiClient) ListSpaceTunnels(ctx context.Context, spaceId string) (*SpaceTunnelListResponse, int, error) {
	response := &SpaceTunnelListResponse{}
	code, err := c.httpClient.Get(ctx, "/space-io/"+spaceId+"/tunnel/list", &response)
	if err != nil {
		return nil, code, err
	}
	return response, code, nil
}

// StopSpaceTunnel stops an agent-owned web tunnel in a space by name.
func (c *ApiClient) StopSpaceTunnel(ctx context.Context, spaceId string, request *SpaceTunnelStopRequest) (int, error) {
	return c.httpClient.Post(ctx, "/space-io/"+spaceId+"/tunnel/stop", request, nil, 200)
}
