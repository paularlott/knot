package apiclient

import (
	"context"
)

type Icon struct {
	Description string `json:"description" msgpack:"description"`
	Source      string `json:"source" msgpack:"source"`
	URL         string `json:"url" msgpack:"url"`
}

type IconsResponse struct {
	Icons []Icon `json:"icons" msgpack:"icons"`
}

func (c *ApiClient) GetIcons(ctx context.Context) (*IconsResponse, int, error) {
	var response IconsResponse
	statusCode, err := c.httpClient.Get(ctx, "api/icons", &response)
	return &response, statusCode, err
}
