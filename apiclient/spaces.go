package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/database/model"
)

type SpaceRequest struct {
	Name            string               `json:"name"`
	Description     string               `json:"description"`
	TemplateId      string               `json:"template_id"`
	Shell           string               `json:"shell"`
	UserId          string               `json:"user_id"`
	AltNames        []model.AltNameEntry `json:"alt_names"`
	IconURL         string               `json:"icon_url"`
	CustomFields    []CustomFieldValue   `json:"custom_fields"`
	SelectedNodeId  string               `json:"selected_node_id,omitempty"`
	StartupScriptId string               `json:"startup_script_id,omitempty"`
	DependsOn       []string             `json:"depends_on"`
	Stack           string               `json:"stack"`
	StackPrefix     string               `json:"stack_prefix"`
}

type CreateSpaceResponse struct {
	Status  bool   `json:"status"`
	SpaceID string `json:"space_id"`
}

type SpaceTransferRequest struct {
	UserId string `json:"user_id"`
}

type SpaceShareUpdateRequest struct {
	Shares []string `json:"shares,omitempty"`
}

type SpaceInfo struct {
	Id                      string               `json:"space_id"`
	Name                    string               `json:"name"`
	Description             string               `json:"description"`
	Note                    string               `json:"note"`
	TemplateName            string               `json:"template_name"`
	TemplateId              string               `json:"template_id"`
	PoolId                  string               `json:"pool_id"`
	PoolName                string               `json:"pool_name"`
	Zone                    string               `json:"zone"`
	Username                string               `json:"username"`
	UserId                  string               `json:"user_id"`
	Platform                string               `json:"platform"`
	Shares                  []string             `json:"shares"`
	DependsOn               []string             `json:"depends_on"`
	HasCodeServer           bool                 `json:"has_code_server"`
	HasSSH                  bool                 `json:"has_ssh"`
	HasHttpVNC              bool                 `json:"has_http_vnc"`
	HasTerminal             bool                 `json:"has_terminal"`
	HasState                bool                 `json:"has_state"`
	IsDeployed              bool                 `json:"is_deployed"`
	IsPending               bool                 `json:"is_pending"`
	IsDeleting              bool                 `json:"is_deleting"`
	TcpPorts                map[string]string    `json:"tcp_ports"`
	HttpPorts               map[string]string    `json:"http_ports"`
	UpdateAvailable         bool                 `json:"update_available"`
	IsRemote                bool                 `json:"is_remote"`
	HasVSCodeTunnel         bool                 `json:"has_vscode_tunnel"`
	VSCodeTunnel            string               `json:"vscode_tunnel_name"`
	StartedAt               time.Time            `json:"started_at"`
	IconURL                 string               `json:"icon_url"`
	Healthy                 bool                 `json:"healthy"`
	HealthKnown             bool                 `json:"health_known"`
	NodeHostname            string               `json:"node_hostname"`
	Stack                   string               `json:"stack"`
	StackPrefix             string               `json:"stack_prefix"`
	ResourceUsage           *SpaceResourceUsage  `json:"resource_usage,omitempty"`
	AltNames                []model.AltNameEntry `json:"alt_names"`
	CustomFields            []CustomFieldValue   `json:"custom_fields"`
	TemplateHasSSH          bool                 `json:"template_has_ssh"`
	TemplateHasTerminal     bool                 `json:"template_has_terminal"`
	TemplateHasCodeServer   bool                 `json:"template_has_code_server"`
	TemplateHasVSCodeTunnel bool                 `json:"template_has_vscode_tunnel"`
}

type SpaceInfoList struct {
	Count  int         `json:"count"`
	Spaces []SpaceInfo `json:"spaces"`
}

type CustomFieldValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type SetCustomFieldRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type GetCustomFieldResponse struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type SpaceDefinition struct {
	SpaceId            string                       `json:"space_id"`
	UserId             string                       `json:"user_id"`
	TemplateId         string                       `json:"template_id"`
	Shares             []string                     `json:"shares"`
	DependsOn          []string                     `json:"depends_on"`
	Name               string                       `json:"name"`
	Description        string                       `json:"description"`
	Note               string                       `json:"note"`
	TemplateName       string                       `json:"template_name"`
	PoolId             string                       `json:"pool_id"`
	PoolName           string                       `json:"pool_name"`
	Username           string                       `json:"username"`
	Platform           string                       `json:"platform"`
	Shell              string                       `json:"shell"`
	Zone               string                       `json:"zone"`
	AltNames           []model.AltNameEntry         `json:"alt_names"`
	IsDeployed         bool                         `json:"is_deployed"`
	IsPending          bool                         `json:"is_pending"`
	IsDeleting         bool                         `json:"is_deleting"`
	HasEverStarted     bool                         `json:"has_ever_started"`
	VolumeData         map[string]model.SpaceVolume `json:"volume_data"`
	StartedAt          time.Time                    `json:"started_at"`
	CreatedAt          time.Time                    `json:"created_at"`
	CreatedAtFormatted string                       `json:"created_at_formatted"`
	IconURL            string                       `json:"icon_url"`
	CustomFields       []CustomFieldValue           `json:"custom_fields"`
	StartupScriptId    string                       `json:"startup_script_id"`
	HasCodeServer      bool                         `json:"has_code_server"`
	HasSSH             bool                         `json:"has_ssh"`
	HasTerminal        bool                         `json:"has_terminal"`
	HasHttpVNC         bool                         `json:"has_http_vnc"`
	HasState           bool                         `json:"has_state"`
	TcpPorts           map[string]string            `json:"tcp_ports"`
	HttpPorts          map[string]string            `json:"http_ports"`
	UpdateAvailable    bool                         `json:"update_available"`
	HasVSCodeTunnel    bool                         `json:"has_vscode_tunnel"`
	VSCodeTunnel       string                       `json:"vscode_tunnel_name"`
	Healthy            bool                         `json:"healthy"`
	HealthKnown        bool                         `json:"health_known"`
	IsRemote           bool                         `json:"is_remote"`
	NodeId             string                       `json:"node_id"`
	NodeHostname       string                       `json:"node_hostname"`
	Stack              string                       `json:"stack"`
	StackPrefix        string                       `json:"stack_prefix"`
	ResourceUsage      *SpaceResourceUsage          `json:"resource_usage,omitempty"`
}

type SpaceResourceUsage struct {
	CPUPercent       float64 `json:"cpu_percent"`
	MemoryUsedBytes  uint64  `json:"memory_used_bytes"`
	MemoryLimitBytes uint64  `json:"memory_limit_bytes"`
	DiskUsedBytes    uint64  `json:"disk_used_bytes"`
	DiskLimitBytes   uint64  `json:"disk_limit_bytes"`
}

type SpaceActivityUsage struct {
	WriteCount     uint32     `json:"write_count"`
	CreateCount    uint32     `json:"create_count"`
	DeleteCount    uint32     `json:"delete_count"`
	RenameCount    uint32     `json:"rename_count"`
	DistinctPaths  uint32     `json:"distinct_paths"`
	LastActivityAt *time.Time `json:"last_activity_at,omitempty"`
}

type SpaceUsagePoint struct {
	BucketStart   time.Time           `json:"bucket_start"`
	BucketKind    string              `json:"bucket_kind,omitempty"`
	IsLive        bool                `json:"is_live,omitempty"`
	ResourceUsage *SpaceResourceUsage `json:"resource_usage,omitempty"`
	ActivityUsage *SpaceActivityUsage `json:"activity_usage,omitempty"`
}

type SpaceUsageHistoryResponse struct {
	SpaceId    string            `json:"space_id"`
	Range      string            `json:"range,omitempty"`
	BucketKind string            `json:"bucket_kind,omitempty"`
	Points     []SpaceUsagePoint `json:"points"`
}

type RunCommandRequest struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Timeout int      `json:"timeout"`
	Workdir string   `json:"workdir,omitempty"`
}

type CopyFileRequest struct {
	SourcePath string `json:"source_path"`
	DestPath   string `json:"dest_path"`
	Content    []byte `json:"content,omitempty"`
	Direction  string `json:"direction"` // "to_space" or "from_space"
	Workdir    string `json:"workdir,omitempty"`
}

type PortForwardRequest struct {
	LocalPort  uint16 `json:"local_port"`
	Space      string `json:"space"`
	RemotePort uint16 `json:"remote_port"`
	Persistent bool   `json:"persistent"`
	Force      bool   `json:"force"`
}

type PortListResponse struct {
	Forwards []PortForwardInfo `json:"forwards"`
}

type PortForwardInfo struct {
	LocalPort   uint16 `json:"local_port"`
	Space       string `json:"space"`
	RemotePort  uint16 `json:"remote_port"`
	Persistent  bool   `json:"persistent"`
	Mode        string `json:"mode"` // "direct" or "relay"
	LatencyMs   int    `json:"latency_ms"`
	JitterMs    int    `json:"jitter_ms"`
	BandwidthKB int    `json:"bandwidth_kb"`
	TimeoutMs   int    `json:"timeout_ms"`
	Down        bool   `json:"down"`
}

type PortStopRequest struct {
	LocalPort uint16 `json:"local_port"`
}

type PortThrottleRequest struct {
	LocalPort   uint16 `json:"local_port"`
	LatencyMs   int    `json:"latency_ms"`
	JitterMs    int    `json:"jitter_ms"`
	BandwidthKB int    `json:"bandwidth_kb"`
	TimeoutMs   int    `json:"timeout_ms"`
	Down        bool   `json:"down"`
	Reset       bool   `json:"reset"`
}

type PortApplyRequest struct {
	Forwards []PortForwardRequest `json:"forwards"`
}

type PortApplyResponse struct {
	Applied []PortForwardInfo `json:"applied"`
	Stopped []PortForwardInfo `json:"stopped"`
	Errors  []string          `json:"errors,omitempty"`
}

func (c *ApiClient) GetSpaces(ctx context.Context, userId string, allZones bool) (*SpaceInfoList, int, error) {
	response := &SpaceInfoList{}

	url := "/api/spaces?user_id=" + userId
	if allZones {
		url += "&all_zones=true"
	}
	code, err := c.httpClient.Get(ctx, url, &response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) GetSpace(ctx context.Context, spaceId string) (*SpaceDefinition, int, error) {
	response := &SpaceDefinition{}

	code, err := c.httpClient.Get(ctx, "/api/spaces/"+spaceId, &response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

// StackExists reports whether a stack name is already in use for the
// authenticated user (i.e. at least one non-deleted space has that Stack).
func (c *ApiClient) StackExists(ctx context.Context, stackName string) (bool, error) {
	var response struct {
		Exists bool `json:"exists"`
	}
	code, err := c.httpClient.Get(ctx, "/api/stacks/"+stackName+"/exists", &response)
	if err != nil {
		return false, err
	}
	if code != 200 {
		return false, fmt.Errorf("unexpected status %d checking stack existence", code)
	}
	return response.Exists, nil
}

func (c *ApiClient) UpdateSpace(ctx context.Context, spaceId string, space *SpaceRequest) (int, error) {
	code, err := c.httpClient.Put(ctx, "/api/spaces/"+spaceId, space, nil, 200)
	if err != nil {
		return code, err
	}

	return code, nil
}

func (c *ApiClient) SetSpaceCustomField(ctx context.Context, spaceId string, fieldName string, fieldValue string) (int, error) {
	request := &SetCustomFieldRequest{
		Name:  fieldName,
		Value: fieldValue,
	}

	code, err := c.httpClient.Put(ctx, "/api/spaces/"+spaceId+"/custom-field", request, nil, 200)
	if err != nil {
		return code, err
	}

	return code, nil
}

func (c *ApiClient) GetSpaceCustomField(ctx context.Context, spaceId string, fieldName string) (*GetCustomFieldResponse, int, error) {
	response := &GetCustomFieldResponse{}

	code, err := c.httpClient.Get(ctx, "/api/spaces/"+spaceId+"/custom-field/"+fieldName, response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) CreateSpace(ctx context.Context, space *SpaceRequest) (string, int, error) {
	response := &CreateSpaceResponse{}

	code, err := c.httpClient.Post(ctx, "/api/spaces", space, response, 201)
	if err != nil {
		return "", code, err
	}

	return response.SpaceID, code, nil
}

func (c *ApiClient) DeleteSpace(ctx context.Context, spaceId string) (int, error) {
	return c.httpClient.Delete(ctx, "/api/spaces/"+spaceId, nil, nil, 200)
}

func (c *ApiClient) StartSpace(ctx context.Context, spaceId string) (int, error) {
	return c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/start", nil, nil, 200)
}

func (c *ApiClient) StopSpace(ctx context.Context, spaceId string) (int, error) {
	return c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/stop", nil, nil, 200)
}

func (c *ApiClient) RestartSpace(ctx context.Context, spaceId string) (int, error) {
	return c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/restart", nil, nil, 200)
}

func stackAction(c *ApiClient, ctx context.Context, stackName, action string) (int, error) {
	// Stack operations block synchronously on the server (up to 120s per tier).
	// Disable the default 10s client timeout for this call.
	c.httpClient.SetTimeout(0)
	defer c.httpClient.SetTimeout(10 * time.Second)

	code, err := c.httpClient.PostJSON(ctx, "/api/spaces/stacks/"+stackName+"/"+action, nil, nil, 202)
	if err != nil {
		if idx := strings.Index(err.Error(), "{"); idx != -1 {
			var body struct {
				Error string `json:"error"`
			}
			if jsonErr := json.Unmarshal([]byte(err.Error()[idx:]), &body); jsonErr == nil && body.Error != "" {
				return code, fmt.Errorf("%s", body.Error)
			}
		}
	}
	return code, err
}

func (c *ApiClient) StartStack(ctx context.Context, stackName string) (int, error) {
	return stackAction(c, ctx, stackName, "start")
}

func (c *ApiClient) StopStack(ctx context.Context, stackName string) (int, error) {
	return stackAction(c, ctx, stackName, "stop")
}

func (c *ApiClient) RestartStack(ctx context.Context, stackName string) (int, error) {
	return stackAction(c, ctx, stackName, "restart")
}

// DeleteStack deletes every space in the named stack. The server validates that
// every space is stoppable before mutating anything, so the call is all-or-
// nothing. Returns when each space has been marked as deleting (actual teardown
// continues asynchronously on the server).
func (c *ApiClient) DeleteStack(ctx context.Context, stackName string) (int, error) {
	// Stack operations block synchronously on the server (up to 120s per tier).
	// Disable the default 10s client timeout for this call.
	c.httpClient.SetTimeout(0)
	defer c.httpClient.SetTimeout(10 * time.Second)

	code, err := c.httpClient.Delete(ctx, "/api/stacks/"+stackName, nil, nil, 202)
	if err != nil {
		if idx := strings.Index(err.Error(), "{"); idx != -1 {
			var body struct {
				Error string `json:"error"`
			}
			if jsonErr := json.Unmarshal([]byte(err.Error()[idx:]), &body); jsonErr == nil && body.Error != "" {
				return code, fmt.Errorf("%s", body.Error)
			}
		}
	}
	return code, err
}

func (c *ApiClient) TransferSpace(ctx context.Context, spaceId string, userId string) (int, error) {
	request := &SpaceTransferRequest{
		UserId: userId,
	}

	return c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/transfer", request, nil, 200)
}

func (c *ApiClient) AddShare(ctx context.Context, spaceId string, userId string) (int, error) {
	request := &SpaceShareUpdateRequest{
		Shares: []string{userId},
	}

	return c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/share", request, nil, 200)
}

func (c *ApiClient) AddShares(ctx context.Context, spaceId string, shares []string) (int, error) {
	request := &SpaceShareUpdateRequest{
		Shares: shares,
	}

	return c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/share", request, nil, 200)
}

func (c *ApiClient) RemoveShare(ctx context.Context, spaceId string) (int, error) {
	return c.httpClient.Delete(ctx, "/api/spaces/"+spaceId+"/share", nil, nil, 200)
}

func (c *ApiClient) ForwardPort(ctx context.Context, spaceId string, request *PortForwardRequest) (int, error) {
	return c.httpClient.Post(ctx, "/space-io/"+spaceId+"/port/forward", request, nil, 200)
}

func (c *ApiClient) ListPorts(ctx context.Context, spaceId string) (*PortListResponse, int, error) {
	response := &PortListResponse{}
	code, err := c.httpClient.Get(ctx, "/space-io/"+spaceId+"/port/list", &response)
	if err != nil {
		return nil, code, err
	}
	return response, code, nil
}

func (c *ApiClient) StopPort(ctx context.Context, spaceId string, request *PortStopRequest) (int, error) {
	return c.httpClient.Post(ctx, "/space-io/"+spaceId+"/port/stop", request, nil, 200)
}

func (c *ApiClient) ThrottlePort(ctx context.Context, spaceId string, request *PortThrottleRequest) (int, error) {
	return c.httpClient.Post(ctx, "/space-io/"+spaceId+"/port/throttle", request, nil, 200)
}

func (c *ApiClient) ApplyPorts(ctx context.Context, spaceId string, request *PortApplyRequest) (*PortApplyResponse, int, error) {
	response := &PortApplyResponse{}
	code, err := c.httpClient.Post(ctx, "/space-io/"+spaceId+"/port/apply", request, response, 200)
	if err != nil {
		return nil, code, err
	}
	return response, code, nil
}

func (c *ApiClient) GetSpaceByName(ctx context.Context, spaceName string) (*SpaceDefinition, error) {
	spaces, _, err := c.GetSpaces(ctx, "", false)
	if err != nil {
		return nil, err
	}
	for _, s := range spaces.Spaces {
		if s.Name == spaceName {
			space, _, err := c.GetSpace(ctx, s.Id)
			return space, err
		}
	}
	return nil, fmt.Errorf("space not found")
}

func (c *ApiClient) RunCommand(ctx context.Context, spaceId string, request *RunCommandRequest) (string, error) {
	var response struct {
		Output  string `json:"output"`
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}

	_, err := c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/run-command", request, &response, 200)
	if err != nil {
		return "", err
	}

	if !response.Success {
		return response.Output, fmt.Errorf("%s", response.Error)
	}

	return response.Output, nil
}

func (c *ApiClient) ReadSpaceFile(ctx context.Context, spaceId string, filePath string) (string, error) {
	content, _, err := c.ReadSpaceFileRange(ctx, spaceId, filePath, 0, 0)
	return content, err
}

// ReadSpaceFileRange reads a file or a 1-based line range from a space. offset
// is the 1-based starting line (0 = from the beginning); limit is the maximum
// number of lines to return (0 = no limit). Returns the content, the total
// number of lines in the file, and any error.
func (c *ApiClient) ReadSpaceFileRange(ctx context.Context, spaceId string, filePath string, offset, limit int) (string, int, error) {
	var request struct {
		Path   string `json:"path"`
		Offset int    `json:"offset,omitempty"`
		Limit  int    `json:"limit,omitempty"`
	}
	request.Path = filePath
	request.Offset = offset
	request.Limit = limit

	var response struct {
		Success    bool   `json:"success"`
		Content    string `json:"content"`
		Size       int    `json:"size"`
		TotalLines int    `json:"total_lines"`
		Error      string `json:"error"`
	}

	_, err := c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/files/read", request, &response, 200)
	if err != nil {
		return "", 0, err
	}

	if !response.Success {
		return "", 0, fmt.Errorf("%s", response.Error)
	}

	return response.Content, response.TotalLines, nil
}

func (c *ApiClient) WriteSpaceFile(ctx context.Context, spaceId string, filePath string, content string) error {
	return c.WriteSpaceFileMode(ctx, spaceId, filePath, content, "")
}

// WriteSpaceFileMode writes content with a mode: "overwrite" (default),
// "append", or "prepend".
func (c *ApiClient) WriteSpaceFileMode(ctx context.Context, spaceId string, filePath string, content string, mode string) error {
	return c.WriteSpaceFileOpts(ctx, spaceId, filePath, content, mode, 0, 0)
}

// WriteSpaceFileOpts is the full-form upload used by sync tools. mtimeNs and
// filePerm are optional (0 = leave alone); when set, the agent applies them
// after the write so the destination matches the source's metadata.
//
// The apiclient uses msgpack for all requests by default (set in
// NewClient/NewMuxClient), so the Content string carries bytes verbatim —
// binary-safe. JSON encoding would have replaced non-UTF-8 bytes with U+FFFD,
// changing the byte count and making mirror re-upload the same files forever.
func (c *ApiClient) WriteSpaceFileOpts(ctx context.Context, spaceId string, filePath string, content string, mode string, mtimeNs int64, filePerm uint32) error {
	var request struct {
		Path     string `json:"path" msgpack:"path"`
		Content  string `json:"content" msgpack:"content"`
		Mode     string `json:"mode,omitempty" msgpack:"mode,omitempty"`
		MtimeNs  int64  `json:"mtime_ns,omitempty" msgpack:"mtime_ns,omitempty"`
		FilePerm uint32 `json:"file_perm,omitempty" msgpack:"file_perm,omitempty"`
	}
	request.Path = filePath
	request.Content = content
	request.Mode = mode
	request.MtimeNs = mtimeNs
	request.FilePerm = filePerm

	var response struct {
		Success      bool   `json:"success" msgpack:"success"`
		BytesWritten int    `json:"bytes_written" msgpack:"bytes_written"`
		Error        string `json:"error" msgpack:"error"`
	}

	_, err := c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/files/write", request, &response, 200)
	if err != nil {
		return err
	}

	if !response.Success {
		return fmt.Errorf("%s", response.Error)
	}

	return nil
}

// GrepMatch is a single grep match.
type GrepMatch struct {
	File string `json:"file" msgpack:"file"`
	Line int    `json:"line" msgpack:"line"`
	Text string `json:"text" msgpack:"text"`
}

// Grep searches file contents in a running space. Returns matching lines.
func (c *ApiClient) Grep(ctx context.Context, spaceId string, req GrepRequest) (*GrepResponse, error) {
	var resp GrepResponse
	_, err := c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/files/grep", req, &resp, 200)
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}
	return &resp, nil
}

// GrepRequest mirrors msg.GrepMessage for the HTTP API.
type GrepRequest struct {
	Pattern     string `json:"pattern" msgpack:"pattern"`
	Path        string `json:"path" msgpack:"path"`
	Literal     bool   `json:"literal,omitempty" msgpack:"literal,omitempty"`
	Recursive   bool   `json:"recursive,omitempty" msgpack:"recursive,omitempty"`
	IgnoreCase  bool   `json:"ignore_case,omitempty" msgpack:"ignore_case,omitempty"`
	Glob        string `json:"glob,omitempty" msgpack:"glob,omitempty"`
	FollowLinks bool   `json:"follow_links,omitempty" msgpack:"follow_links,omitempty"`
	MaxSize     int64  `json:"max_size,omitempty" msgpack:"max_size,omitempty"`
	Workdir     string `json:"workdir,omitempty" msgpack:"workdir,omitempty"`
}

type GrepResponse struct {
	Success bool        `json:"success" msgpack:"success"`
	Error   string      `json:"error,omitempty" msgpack:"error,omitempty"`
	Matches []GrepMatch `json:"matches,omitempty" msgpack:"matches,omitempty"`
}

// FindRequest mirrors msg.FindMessage for the HTTP API.
type FindRequest struct {
	Path            string   `json:"path" msgpack:"path"`
	Recursive       bool     `json:"recursive" msgpack:"recursive"`
	Type            string   `json:"type,omitempty" msgpack:"type,omitempty"`
	Name            string   `json:"name,omitempty" msgpack:"name,omitempty"`
	IncludeHidden   bool     `json:"include_hidden,omitempty" msgpack:"include_hidden,omitempty"`
	IncludeMetadata bool     `json:"include_metadata,omitempty" msgpack:"include_metadata,omitempty"`
	IncludeHash     bool     `json:"include_hash,omitempty" msgpack:"include_hash,omitempty"`
	IncludeSymlinks bool     `json:"include_symlinks,omitempty" msgpack:"include_symlinks,omitempty"`
	FollowLinks     bool     `json:"follow_links,omitempty" msgpack:"follow_links,omitempty"`
	MaxDepth        int      `json:"max_depth,omitempty" msgpack:"max_depth,omitempty"`
	MtimeMin        *float64 `json:"mtime_min,omitempty" msgpack:"mtime_min,omitempty"`
	MtimeMax        *float64 `json:"mtime_max,omitempty" msgpack:"mtime_max,omitempty"`
	SizeMin         *int64   `json:"size_min,omitempty" msgpack:"size_min,omitempty"`
	SizeMax         *int64   `json:"size_max,omitempty" msgpack:"size_max,omitempty"`
	Workdir         string   `json:"workdir,omitempty" msgpack:"workdir,omitempty"`
}

// FindResponse is the agent's reply. INVARIANT: exactly one of Paths or
// Entries is populated per response, never both. The agent picks based on
// FindRequest.IncludeMetadata — false (default) populates Paths (cheap, no
// per-entry stat); true populates Entries (every match stat'd). Pick the
// field that matches the flag you set.
type FindResponse struct {
	Success bool        `json:"success" msgpack:"success"`
	Error   string      `json:"error,omitempty" msgpack:"error,omitempty"`
	Paths   []string    `json:"paths,omitempty" msgpack:"paths,omitempty"`
	Entries []FindEntry `json:"entries,omitempty" msgpack:"entries,omitempty"`
}

// FindEntry mirrors msg.FindEntry for the HTTP API.
type FindEntry struct {
	Path       string  `json:"path" msgpack:"path"`
	Size       int64   `json:"size" msgpack:"size"`
	Mtime      float64 `json:"mtime" msgpack:"mtime"`
	IsDir      bool    `json:"is_dir" msgpack:"is_dir"`
	Hash       uint64  `json:"hash,omitempty" msgpack:"hash,omitempty"`
	LinkTarget string  `json:"link_target,omitempty" msgpack:"link_target,omitempty"`
	FilePerm   int     `json:"file_perm,omitempty" msgpack:"file_perm,omitempty"`
}

// Find finds files/directories in a running space.
func (c *ApiClient) Find(ctx context.Context, spaceId string, req FindRequest) (*FindResponse, error) {
	var resp FindResponse
	_, err := c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/files/find", req, &resp, 200)
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}
	return &resp, nil
}

// SedRequest mirrors msg.SedMessage for the HTTP API.
type SedRequest struct {
	Mode        string `json:"mode" msgpack:"mode"` // "replace", "replace_pattern", "extract"
	Pattern     string `json:"pattern" msgpack:"pattern"`
	Replacement string `json:"replacement,omitempty" msgpack:"replacement,omitempty"`
	Path        string `json:"path" msgpack:"path"`
	Recursive   bool   `json:"recursive,omitempty" msgpack:"recursive,omitempty"`
	IgnoreCase  bool   `json:"ignore_case,omitempty" msgpack:"ignore_case,omitempty"`
	Glob        string `json:"glob,omitempty" msgpack:"glob,omitempty"`
	FollowLinks bool   `json:"follow_links,omitempty" msgpack:"follow_links,omitempty"`
	MaxSize     int64  `json:"max_size,omitempty" msgpack:"max_size,omitempty"`
	Workdir     string `json:"workdir,omitempty" msgpack:"workdir,omitempty"`
}

type ExtractMatch struct {
	File   string   `json:"file" msgpack:"file"`
	Line   int      `json:"line" msgpack:"line"`
	Text   string   `json:"text" msgpack:"text"`
	Groups []string `json:"groups,omitempty" msgpack:"groups,omitempty"`
}

type SedResponse struct {
	Success       bool           `json:"success" msgpack:"success"`
	Error         string         `json:"error,omitempty" msgpack:"error,omitempty"`
	Mode          string         `json:"mode,omitempty" msgpack:"mode,omitempty"`
	FilesModified int64          `json:"files_modified,omitempty" msgpack:"files_modified,omitempty"`
	Matches       []ExtractMatch `json:"matches,omitempty" msgpack:"matches,omitempty"`
}

// Sed performs an in-place edit or extraction in a running space.
func (c *ApiClient) Sed(ctx context.Context, spaceId string, req SedRequest) (*SedResponse, error) {
	var resp SedResponse
	_, err := c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/files/sed", req, &resp, 200)
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}
	return &resp, nil
}

// EditFileRequest mirrors msg.EditFileMessage for the HTTP API.
type EditFileRequest struct {
	Path    string `json:"path" msgpack:"path"`
	Search  string `json:"search" msgpack:"search"`
	Replace string `json:"replace" msgpack:"replace"`
	Workdir string `json:"workdir,omitempty" msgpack:"workdir,omitempty"`
}

type EditFileResponse struct {
	Success      bool   `json:"success" msgpack:"success"`
	Error        string `json:"error,omitempty" msgpack:"error,omitempty"`
	BytesWritten int    `json:"bytes_written,omitempty" msgpack:"bytes_written,omitempty"`
}

// EditFile performs a targeted search-and-replace on a single file. The search
// text must appear exactly once; fails if 0 or >1 matches.
func (c *ApiClient) EditFile(ctx context.Context, spaceId string, req EditFileRequest) (*EditFileResponse, error) {
	var resp EditFileResponse
	_, err := c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/files/edit", req, &resp, 200)
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}
	return &resp, nil
}

// DeleteFileRequest mirrors msg.DeleteFileMessage for the HTTP API.
type DeleteFileRequest struct {
	Path      string `json:"path" msgpack:"path"`
	Recursive bool   `json:"recursive,omitempty" msgpack:"recursive,omitempty"`
	Workdir   string `json:"workdir,omitempty" msgpack:"workdir,omitempty"`
}

// CreateSymlinkSpaceFile creates a symlink at filePath pointing to target.
// Uses the same /files/write endpoint with SymlinkTarget set instead of
// Content. The agent removes any existing file at the destination first.
func (c *ApiClient) CreateSymlinkSpaceFile(ctx context.Context, spaceId, filePath, target string) error {
	var request struct {
		Path          string `json:"path" msgpack:"path"`
		SymlinkTarget string `json:"symlink_target" msgpack:"symlink_target"`
	}
	request.Path = filePath
	request.SymlinkTarget = target

	var response struct {
		Success      bool   `json:"success" msgpack:"success"`
		BytesWritten int    `json:"bytes_written" msgpack:"bytes_written"`
		Error        string `json:"error" msgpack:"error"`
	}

	_, err := c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/files/write", request, &response, 200)
	if err != nil {
		return err
	}
	if !response.Success {
		return fmt.Errorf("%s", response.Error)
	}
	return nil
}

type DeleteFileResponse struct {
	Success bool   `json:"success" msgpack:"success"`
	Error   string `json:"error,omitempty" msgpack:"error,omitempty"`
	Removed int    `json:"removed,omitempty" msgpack:"removed,omitempty"`
}

// DeleteSpaceFile removes a file or directory from a running space. Recursive
// uses os.RemoveAll semantics; non-recursive on a non-empty directory fails.
// Missing paths are treated as success (idempotent) — important for sync tools
// whose delete list was computed against a slightly stale snapshot.
func (c *ApiClient) DeleteSpaceFile(ctx context.Context, spaceId string, req DeleteFileRequest) (*DeleteFileResponse, error) {
	var resp DeleteFileResponse
	_, err := c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/files/delete", req, &resp, 200)
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("%s", resp.Error)
	}
	return &resp, nil
}
