package apiclient

type TunnelInfo struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

func (c *ApiClient) GetTunnels() ([]TunnelInfo, int, error) {
	response := []TunnelInfo{}

	code, err := c.httpClient.Get("/api/tunnels", &response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) GetTunnelDomain() (string, int, error) {
	response := ""

	code, err := c.httpClient.Get("/api/tunnels/domain", &response)
	if err != nil {
		return "", code, err
	}

	return response, code, nil
}
