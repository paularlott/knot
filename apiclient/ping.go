package apiclient

import (
	"context"
	"errors"
)

type PingResponse struct {
	Status  bool   `json:"status"`
	Version string `json:"version"`
	Zone    string `json:"zone"`
}

func (c *ApiClient) Ping(ctx context.Context) (*PingResponse, error) {
	ping := &PingResponse{}
	statusCode, err := c.httpClient.Get(ctx, "/api/ping", ping)
	if statusCode > 0 {
		if statusCode == 401 {
			return nil, errors.New("unauthorized")
		} else if statusCode != 200 {
			return nil, errors.New("invalid status code")
		}
	}

	return ping, err
}
