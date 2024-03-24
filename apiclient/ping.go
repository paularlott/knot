package apiclient

import "errors"

type PingResponse struct {
	Status  bool   `json:"status"`
	Version string `json:"version"`
}

func (c *ApiClient) Ping() (string, error) {
	ping := PingResponse{}
	statusCode, err := c.httpClient.Get("/api/v1/ping", &ping)
	if statusCode != 200 {
		return "", errors.New("invalid status code")
	}
	return ping.Version, err
}
