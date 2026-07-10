package apiclient

import "context"

type TunnelInfo struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

type TunnelServerInfo struct {
	Domain        string   `json:"domain"`
	TunnelServers []string `json:"tunnel_servers"`
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
	Protocol string `json:"protocol"`
	Port     uint16 `json:"port"`
	Name     string `json:"name"`
}

type SpaceTunnelStartResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	URL     string `json:"url,omitempty"`
}

type SpaceTunnelInfo struct {
	Port     uint16 `json:"port"`
	Protocol string `json:"protocol"`
	Name     string `json:"name"`
	URL      string `json:"url"`
}

type SpaceTunnelListResponse struct {
	Tunnels []SpaceTunnelInfo `json:"tunnels"`
}

type SpaceTunnelStopRequest struct {
	Name string `json:"name"`
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
