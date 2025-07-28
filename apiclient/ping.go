package apiclient

import (
	"context"
	"errors"
)

type PingResponse struct {
	Status  bool   `json:"status"`
	Version string `json:"version"`
}

func (c *ApiClient) Ping(ctx context.Context) (string, error) {
	ping := PingResponse{}
	statusCode, err := c.httpClient.Get(ctx, "/api/ping", &ping)
	if statusCode > 0 {
		if statusCode == 401 {
			return "", errors.New("unauthorized")
		} else if statusCode != 200 {
			return "", errors.New("invalid status code")
		}
	}

	return ping.Version, err
}
