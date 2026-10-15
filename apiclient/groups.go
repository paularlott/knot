package apiclient

import "context"

type GroupInfo struct {
	Id            string `json:"group_id" msgpack:"group_id"`
	Name          string `json:"name" msgpack:"name"`
	MaxSpaces     uint32 `json:"max_spaces" msgpack:"max_spaces"`
	ComputeUnits  uint32 `json:"compute_units" msgpack:"compute_units"`
	StorageUnits  uint32 `json:"storage_units" msgpack:"storage_units"`
	MaxTunnels    uint32 `json:"max_tunnels" msgpack:"max_tunnels"`
	FileStorageMB uint32 `json:"file_storage_mb" msgpack:"file_storage_mb"`
	MaxBuckets    uint32 `json:"max_buckets" msgpack:"max_buckets"`
}

type GroupInfoList struct {
	Count  int         `json:"count" msgpack:"count"`
	Groups []GroupInfo `json:"groups" msgpack:"groups"`
}

type GroupRequest struct {
	Name          string `json:"name" msgpack:"name"`
	MaxSpaces     uint32 `json:"max_spaces" msgpack:"max_spaces"`
	ComputeUnits  uint32 `json:"compute_units" msgpack:"compute_units"`
	StorageUnits  uint32 `json:"storage_units" msgpack:"storage_units"`
	MaxTunnels    uint32 `json:"max_tunnels" msgpack:"max_tunnels"`
	FileStorageMB uint32 `json:"file_storage_mb" msgpack:"file_storage_mb"`
	MaxBuckets    uint32 `json:"max_buckets" msgpack:"max_buckets"`
}

type GroupResponse struct {
	Status bool   `json:"status" msgpack:"status"`
	Id     string `json:"group_id" msgpack:"group_id"`
}

func (c *ApiClient) GetGroups(ctx context.Context) (*GroupInfoList, int, error) {
	response := &GroupInfoList{}

	code, err := c.httpClient.Get(ctx, "/api/groups", response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) UpdateGroup(ctx context.Context, groupId string, request *GroupRequest) (int, error) {
	code, err := c.httpClient.Put(ctx, "/api/groups/"+groupId, request, nil, 200)
	if err != nil {
		return code, err
	}

	return code, nil
}

func (c *ApiClient) CreateGroup(ctx context.Context, request *GroupRequest) (string, int, error) {
	response := &GroupResponse{}

	code, err := c.httpClient.Post(ctx, "/api/groups", request, response, 201)
	if err != nil {
		return "", code, err
	}

	return response.Id, code, nil
}

func (c *ApiClient) DeleteGroup(ctx context.Context, groupId string) (int, error) {
	return c.httpClient.Delete(ctx, "/api/groups/"+groupId, nil, nil, 200)
}

func (c *ApiClient) GetGroup(ctx context.Context, groupId string) (*GroupInfo, int, error) {
	response := &GroupInfo{}

	code, err := c.httpClient.Get(ctx, "/api/groups/"+groupId, response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}
