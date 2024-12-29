package apiclient

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

type UserRoleRequest struct {
	Name        string   `json:"name"`
	Permissions []uint16 `json:"permissions"`
}

type RoleResponse struct {
	Status bool   `json:"status"`
	Id     string `json:"role_id"`
}

func (c *ApiClient) GetRoles() (*RoleInfoList, int, error) {
	response := &RoleInfoList{}

	code, err := c.httpClient.Get("/api/v1/roles", response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) UpdateRole(roleId string, roleName string, permissions []uint16) (int, error) {
	request := UserRoleRequest{
		Name:        roleName,
		Permissions: permissions,
	}

	code, err := c.httpClient.Put("/api/v1/roles/"+roleId, request, nil, 200)
	if err != nil {
		return code, err
	}

	return code, nil
}

func (c *ApiClient) CreateRole(roleName string, permissions []uint16) (string, int, error) {
	request := UserRoleRequest{
		Name:        roleName,
		Permissions: permissions,
	}

	response := &RoleResponse{}

	code, err := c.httpClient.Post("/api/v1/roles", request, response, 201)
	if err != nil {
		return "", code, err
	}

	return response.Id, code, nil
}

func (c *ApiClient) DeleteRole(roleId string) (int, error) {
	return c.httpClient.Delete("/api/v1/roles/"+roleId, nil, nil, 200)
}

func (c *ApiClient) GetRole(roleId string) (*RoleDetails, int, error) {
	response := &RoleDetails{}

	code, err := c.httpClient.Get("/api/v1/roles/"+roleId, response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}
