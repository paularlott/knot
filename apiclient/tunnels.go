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
