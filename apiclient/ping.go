package apiclient

import (
	"context"
)

type PingResponse struct {
	Status  bool   `json:"status"`
	Version string `json:"version"`
	Zone    string `json:"zone"`
}

func (c *ApiClient) Ping(ctx context.Context) (*PingResponse, error) {
	ping := &PingResponse{}
	statusCode, err := c.httpClient.Get(ctx, "/api/ping", ping)
	if statusCode > 0 && statusCode != 200 {
		if he := AsHTTPError(err); he != nil {
			return nil, he
		}
		return nil, newStatusError(statusCode, "GET", "/api/ping")
	}

	return ping, err
}
