package apiclient

import "context"

type PermissionInfo struct {
	Id          int    `json:"id" msgpack:"id"`
	Name        string `json:"name" msgpack:"name"`
	Group       string `json:"group" msgpack:"group"`
	Description string `json:"description,omitempty" msgpack:"description,omitempty"`
}

type PermissionInfoList struct {
	Count             int                    `json:"count" msgpack:"count"`
	Permissions       []PermissionInfo       `json:"permissions" msgpack:"permissions"`
	PluginPermissions []PluginPermissionInfo `json:"plugin_permissions,omitempty" msgpack:"plugin_permissions,omitempty"`
}

func (c *ApiClient) GetPermissions(ctx context.Context) (*PermissionInfoList, int, error) {
	response := &PermissionInfoList{}

	code, err := c.httpClient.Get(ctx, "/api/permissions", response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}
