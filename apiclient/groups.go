package apiclient

type GroupInfo struct {
	Id           string `json:"group_id"`
	Name         string `json:"name"`
	MaxSpaces    uint32 `json:"max_spaces"`
	ComputeUnits uint32 `json:"compute_units"`
	StorageUnits uint32 `json:"storage_units"`
	MaxTunnels   uint32 `json:"max_tunnels"`
}

type GroupInfoList struct {
	Count  int         `json:"count"`
	Groups []GroupInfo `json:"groups"`
}

type UserGroupRequest struct {
	Name         string `json:"name"`
	MaxSpaces    uint32 `json:"max_spaces"`
	ComputeUnits uint32 `json:"compute_units"`
	StorageUnits uint32 `json:"storage_units"`
	MaxTunnels   uint32 `json:"max_tunnels"`
}

type GroupResponse struct {
	Status bool   `json:"status"`
	Id     string `json:"group_id"`
}

func (c *ApiClient) GetGroups() (*GroupInfoList, int, error) {
	response := &GroupInfoList{}

	code, err := c.httpClient.Get("/api/groups", response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) UpdateGroup(groupId string, groupName string, maxSpaces uint32, computeUnits uint32, storageUnits uint32, maxTunnels uint32) (int, error) {
	request := UserGroupRequest{
		Name:         groupName,
		MaxSpaces:    maxSpaces,
		ComputeUnits: computeUnits,
		StorageUnits: storageUnits,
		MaxTunnels:   maxTunnels,
	}

	code, err := c.httpClient.Put("/api/groups/"+groupId, request, nil, 200)
	if err != nil {
		return code, err
	}

	return code, nil
}

func (c *ApiClient) CreateGroup(groupName string, maxSpaces uint32, computeUnits uint32, storageUnits uint32, maxTunnels uint32) (string, int, error) {
	request := UserGroupRequest{
		Name:         groupName,
		MaxSpaces:    maxSpaces,
		ComputeUnits: computeUnits,
		StorageUnits: storageUnits,
		MaxTunnels:   maxTunnels,
	}

	response := &GroupResponse{}

	code, err := c.httpClient.Post("/api/groups", request, response, 201)
	if err != nil {
		return "", code, err
	}

	return response.Id, code, nil
}

func (c *ApiClient) DeleteGroup(groupId string) (int, error) {
	return c.httpClient.Delete("/api/groups/"+groupId, nil, nil, 200)
}

func (c *ApiClient) GetGroup(groupId string) (*GroupInfo, int, error) {
	response := &GroupInfo{}

	code, err := c.httpClient.Get("/api/groups/"+groupId, response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}
