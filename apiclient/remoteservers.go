package apiclient

type RegisterRemoteServerRequest struct {
	Url string `json:"url"`
}

type RegisterRemoteServerResponse struct {
	Status   bool   `json:"status"`
	ServerId string `json:"server_id"`
}

func (c *ApiClient) RegisterRemoteServer(url string) (string, error) {
	request := RegisterRemoteServerRequest{
		Url: url,
	}

	response := RegisterRemoteServerResponse{}

	_, err := c.httpClient.Post("/api/v1/remote/servers", &request, &response, 201)
	if err != nil {
		return "", err
	}

	return response.ServerId, nil
}

func (c *ApiClient) UpdateRemoteServer(serverId string) error {
	_, err := c.httpClient.Put("/api/v1/remote/servers/"+serverId, nil, nil, 200)
	return err
}

func (c *ApiClient) NotifyRemoteUserUpdate(userId string) error {
	_, err := c.httpClient.Post("/api/v1/remote/notify/users/"+userId, nil, nil, 200)
	return err
}

func (c *ApiClient) NotifyRemoteUserDelete(userId string) error {
	_, err := c.httpClient.Delete("/api/v1/remote/notify/users/"+userId, nil, nil, 200)
	return err
}
