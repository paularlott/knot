package apiclient

import (
	"context"
)

type Icon struct {
	Description string `json:"description"`
	Source      string `json:"source"`
	URL         string `json:"url"`
}

type IconsResponse struct {
	Icons []Icon `json:"icons" msgpack:"icons"`
}

func (c *ApiClient) GetIcons(ctx context.Context) ([]Icon, int, error) {
	var icons []Icon
	statusCode, err := c.httpClient.Get(ctx, "/api/icons", &icons)
	if err != nil {
		return nil, statusCode, err
	}
	return icons, statusCode, nil
}
