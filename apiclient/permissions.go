package apiclient

import "context"

type PermissionInfo struct {
	Id    int    `json:"id"`
	Name  string `json:"name"`
	Group string `json:"group"`
}

type PermissionInfoList struct {
	Count       int              `json:"count"`
	Permissions []PermissionInfo `json:"permissions"`
}

func (c *ApiClient) GetPermissions(ctx context.Context) (*PermissionInfoList, int, error) {
	response := &PermissionInfoList{}

	code, err := c.httpClient.Get(ctx, "/api/permissions", response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}
