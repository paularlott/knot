package apiclient

import "context"

type RoleDetails struct {
	Id          string   `json:"role_id"`
	Name        string   `json:"name"`
	Permissions []uint16 `json:"permissions"`
}

type RoleInfo struct {
	Id   string `json:"role_id"`
	Name string `json:"name"`
}

type RoleInfoList struct {
	Count int        `json:"count"`
	Roles []RoleInfo `json:"roles"`
}

type RoleRequest struct {
	Name        string   `json:"name"`
	Permissions []uint16 `json:"permissions"`
}

type RoleResponse struct {
	Status bool   `json:"status"`
	Id     string `json:"role_id"`
}

func (c *ApiClient) GetRoles(ctx context.Context) (*RoleInfoList, int, error) {
	response := &RoleInfoList{}

	code, err := c.httpClient.Get(ctx, "/api/roles", response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) UpdateRole(ctx context.Context, roleId string, request *RoleRequest) (int, error) {
	code, err := c.httpClient.Put(ctx, "/api/roles/"+roleId, request, nil, 200)
	if err != nil {
		return code, err
	}

	return code, nil
}

func (c *ApiClient) CreateRole(ctx context.Context, request *RoleRequest) (string, int, error) {
	response := &RoleResponse{}

	code, err := c.httpClient.Post(ctx, "/api/roles", request, response, 201)
	if err != nil {
		return "", code, err
	}

	return response.Id, code, nil
}

func (c *ApiClient) DeleteRole(ctx context.Context, roleId string) (int, error) {
	return c.httpClient.Delete(ctx, "/api/roles/"+roleId, nil, nil, 200)
}

func (c *ApiClient) GetRole(ctx context.Context, roleId string) (*RoleDetails, int, error) {
	response := &RoleDetails{}

	code, err := c.httpClient.Get(ctx, "/api/roles/"+roleId, response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}
