package apiclient

import "context"

type PoolRequest struct {
	Name            string `json:"name"`
	TemplateId      string `json:"template_id"`
	StartupScriptId string `json:"startup_script_id"`
	DesiredCount    int    `json:"desired_count"`
	Active          bool   `json:"active"`
}

type PoolSetSizeRequest struct {
	DesiredCount int `json:"desired_count"`
}

type PoolUpdateRequest struct {
	Name            *string `json:"name,omitempty"`
	TemplateId      *string `json:"template_id,omitempty"`
	StartupScriptId *string `json:"startup_script_id,omitempty"`
	DesiredCount    *int    `json:"desired_count,omitempty"`
	Active          *bool   `json:"active,omitempty"`
}

type PoolUtilization struct {
	CombinedRPS      float64 `json:"combined_rps"`
	MethodRPS        float64 `json:"method_rps"`
	HTTPRPS          float64 `json:"http_rps"`
	TCPRPS           float64 `json:"tcp_rps"`
	MethodInflight   int     `json:"method_inflight"`
	AvgCPUPercent    float64 `json:"avg_cpu_percent"`
	AvgMemoryPercent float64 `json:"avg_memory_percent"`
}

type PoolMemberInfo struct {
	Id             string  `json:"space_id"`
	Name           string  `json:"name"`
	State          string  `json:"state"`
	CombinedRPS    float64 `json:"combined_rps"`
	MethodRPS      float64 `json:"method_rps"`
	HTTPRPS        float64 `json:"http_rps"`
	TCPRPS         float64 `json:"tcp_rps"`
	MethodInflight int     `json:"method_inflight"`
	CPUPercent     float64 `json:"cpu_percent"`
	MemoryPercent  float64 `json:"memory_percent"`
	Healthy        bool    `json:"healthy"`
	IsPending      bool    `json:"is_pending"`
	IsDeleting     bool    `json:"is_deleting"`
	IsDeployed     bool    `json:"is_deployed"`
}

type PoolInfo struct {
	Id              string           `json:"pool_id"`
	Name            string           `json:"name"`
	TemplateId      string           `json:"template_id"`
	StartupScriptId string           `json:"startup_script_id"`
	DesiredCount    int              `json:"desired_count"`
	AliveMembers    int              `json:"alive_members"`
	Active          bool             `json:"active"`
	Utilization     PoolUtilization  `json:"utilization"`
	Members         []PoolMemberInfo `json:"members"`
}

type PoolList struct {
	Count int        `json:"count"`
	Pools []PoolInfo `json:"pools"`
}

type PoolCreateResponse struct {
	Status  bool   `json:"status"`
	Id      string `json:"pool_id"`
	Message string `json:"message,omitempty"`
}

func (c *ApiClient) GetPool(ctx context.Context, idOrName string) (*PoolInfo, int, error) {
	response := &PoolInfo{}
	code, err := c.httpClient.Get(ctx, "/api/pools/"+idOrName, response)
	return response, code, err
}

func (c *ApiClient) GetPools(ctx context.Context) (*PoolList, int, error) {
	response := &PoolList{}
	code, err := c.httpClient.Get(ctx, "/api/pools", response)
	return response, code, err
}

func (c *ApiClient) CreatePool(ctx context.Context, request *PoolRequest) (*PoolCreateResponse, int, error) {
	response := &PoolCreateResponse{}
	code, err := c.httpClient.Post(ctx, "/api/pools", request, response, 201)
	return response, code, err
}

func (c *ApiClient) UpdatePool(ctx context.Context, idOrName string, request *PoolRequest) (int, error) {
	return c.httpClient.Put(ctx, "/api/pools/"+idOrName, request, nil, 200)
}

func (c *ApiClient) PatchPool(ctx context.Context, idOrName string, request *PoolUpdateRequest) (int, error) {
	return c.httpClient.Put(ctx, "/api/pools/"+idOrName, request, nil, 200)
}

func (c *ApiClient) DeletePool(ctx context.Context, idOrName string) (int, error) {
	return c.httpClient.Delete(ctx, "/api/pools/"+idOrName, nil, nil, 200)
}

func (c *ApiClient) SetPoolSize(ctx context.Context, idOrName string, desiredCount int) (int, error) {
	return c.httpClient.Post(ctx, "/api/pools/"+idOrName+"/size", &PoolSetSizeRequest{DesiredCount: desiredCount}, nil, 200)
}

func (c *ApiClient) StartPool(ctx context.Context, idOrName string) (int, error) {
	return c.httpClient.Post(ctx, "/api/pools/"+idOrName+"/start", nil, nil, 200)
}

func (c *ApiClient) StopPool(ctx context.Context, idOrName string) (int, error) {
	return c.httpClient.Post(ctx, "/api/pools/"+idOrName+"/stop", nil, nil, 200)
}
