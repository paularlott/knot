package apiclient

import (
	"context"
	"time"
)

type PoolRequest struct {
	Name            string `json:"name"`
	TemplateId      string `json:"template_id"`
	StartupScriptId string `json:"startup_script_id"`
	DesiredCount    int    `json:"desired_count"`
	Active          bool   `json:"active"`
	// LeaseMaxTime is in seconds: 0 = leases disabled (default), -1 = no
	// timeout, >0 = max duration of one acquire/extend.
	LeaseMaxTime int `json:"lease_max_time"`
	// LeaseMaxExtensions: 0 = extending forbidden (default), -1 = unlimited,
	// >0 = max extensions per lease.
	LeaseMaxExtensions int `json:"lease_max_extensions"`
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
	LeaseMaxTime    *int    `json:"lease_max_time,omitempty"`
	// LeaseMaxExtensions: 0 = extending forbidden, -1 = unlimited, >0 = max
	// extensions per lease. The pointer cannot distinguish "set to 0" from
	// "absent" — patch semantics treat absent as unchanged and 0 as
	// "disable extending".
	LeaseMaxExtensions *int `json:"lease_max_extensions,omitempty"`
}

type PoolLeaseAcquireRequest struct {
	// DurationSeconds: 0 = the pool's maximum (-1 pool: never expires),
	// -1 = never-expiring lease (unlimited pools only), >0 = bounded by the
	// pool maximum.
	DurationSeconds int `json:"duration_seconds"`
	// WaitSeconds optionally long-polls for a free member (max 300).
	WaitSeconds int `json:"wait_seconds"`
}

type PoolLeaseExtendRequest struct {
	DurationSeconds int `json:"duration_seconds"`
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
	TCPRPS         float64 `json:"tcprps"`
	MethodInflight int     `json:"method_inflight"`
	CPUPercent     float64 `json:"cpu_percent"`
	MemoryPercent  float64 `json:"memory_percent"`
	Healthy        bool    `json:"healthy"`
	IsPending      bool    `json:"is_pending"`
	IsDeleting     bool    `json:"is_deleting"`
	IsDeployed     bool    `json:"is_deployed"`
	// Lease state: "" = free, "active" = exclusively leased,
	// "draining" = lease ended, waiting for in-flight work to finish.
	LeaseState     string     `json:"lease_state,omitempty"`
	LeaseHolder    string     `json:"lease_holder,omitempty"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"` // null = never expires
}

type PoolInfo struct {
	Id                 string           `json:"pool_id"`
	Name               string           `json:"name"`
	TemplateId         string           `json:"template_id"`
	StartupScriptId    string           `json:"startup_script_id"`
	DesiredCount       int              `json:"desired_count"`
	AliveMembers       int              `json:"alive_members"`
	Active             bool             `json:"active"`
	LeaseMaxTime       int              `json:"lease_max_time"`
	LeaseMaxExtensions int              `json:"lease_max_extensions"`
	Utilization        PoolUtilization  `json:"utilization"`
	Members            []PoolMemberInfo `json:"members"`
}

type PoolList struct {
	Count int        `json:"count"`
	Pools []PoolInfo `json:"pools"`
}

// LeaseInfo describes one exclusive member lease. ExpiresAt is null for a
// never-expiring lease; MaxExtensions is -1 for unlimited.
type LeaseInfo struct {
	LeaseId        string     `json:"lease_id"`
	PoolId         string     `json:"pool_id"`
	PoolName       string     `json:"pool_name"`
	SpaceId        string     `json:"space_id"`
	SpaceName      string     `json:"space_name"`
	UserId         string     `json:"user_id"`
	Username       string     `json:"username"`
	ExpiresAt      *time.Time `json:"expires_at"`
	ExtensionsUsed int        `json:"extensions_used"`
	MaxExtensions  int        `json:"max_extensions"`
	State          string     `json:"state"` // active | draining
}

type PoolLeaseList struct {
	Count  int         `json:"count"`
	Leases []LeaseInfo `json:"leases"`
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

func (c *ApiClient) AcquirePoolLease(ctx context.Context, idOrName string, request *PoolLeaseAcquireRequest) (*LeaseInfo, int, error) {
	response := &LeaseInfo{}
	// The server long-polls up to wait_seconds before answering, so the
	// default 10s client timeout would cut the wait short. Raise it for
	// this call and restore the package default afterwards.
	if request.WaitSeconds > 0 {
		c.httpClient.SetTimeout(time.Duration(request.WaitSeconds+15) * time.Second)
		defer c.httpClient.SetTimeout(10 * time.Second)
	}
	code, err := c.httpClient.Post(ctx, "/api/pools/"+idOrName+"/acquire", request, response, 200)
	return response, code, err
}

func (c *ApiClient) ExtendPoolLease(ctx context.Context, idOrName, leaseId string, request *PoolLeaseExtendRequest) (*LeaseInfo, int, error) {
	response := &LeaseInfo{}
	code, err := c.httpClient.Post(ctx, "/api/pools/"+idOrName+"/leases/"+leaseId+"/extend", request, response, 200)
	return response, code, err
}

func (c *ApiClient) ReleasePoolLease(ctx context.Context, idOrName, leaseId string) (*LeaseInfo, int, error) {
	response := &LeaseInfo{}
	code, err := c.httpClient.Delete(ctx, "/api/pools/"+idOrName+"/leases/"+leaseId, nil, response, 200)
	return response, code, err
}

func (c *ApiClient) GetPoolLeases(ctx context.Context, idOrName string) (*PoolLeaseList, int, error) {
	response := &PoolLeaseList{}
	code, err := c.httpClient.Get(ctx, "/api/pools/"+idOrName+"/leases", response)
	return response, code, err
}
