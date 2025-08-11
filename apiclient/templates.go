package apiclient

import "context"

type CustomFieldDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type TemplateCreateRequest struct {
	Name             string               `json:"name"`
	Job              string               `json:"job"`
	Description      string               `json:"description"`
	Volumes          string               `json:"volumes"`
	Groups           []string             `json:"groups"`
	Platform         string               `json:"platform"`
	Active           bool                 `json:"active"`
	WithTerminal     bool                 `json:"with_terminal"`
	WithVSCodeTunnel bool                 `json:"with_vscode_tunnel"`
	WithCodeServer   bool                 `json:"with_code_server"`
	WithSSH          bool                 `json:"with_ssh"`
	WithRunCommand   bool                 `json:"with_run_command"`
	ScheduleEnabled  bool                 `json:"schedule_enabled"`
	AutoStart        bool                 `json:"auto_start"`
	Schedule         []TemplateDetailsDay `json:"schedule"`
	ComputeUnits     uint32               `json:"compute_units"`
	StorageUnits     uint32               `json:"storage_units"`
	Zones            []string             `json:"zones"`
	MaxUptime        uint32               `json:"max_uptime"`
	MaxUptimeUnit    string               `json:"max_uptime_unit"`
	IconURL          string               `json:"icon_url"`
	CustomFields     []CustomFieldDef     `json:"custom_fields"`
}

type TemplateUpdateRequest struct {
	Name             string               `json:"name"`
	Job              string               `json:"job"`
	Description      string               `json:"description"`
	Volumes          string               `json:"volumes"`
	Groups           []string             `json:"groups"`
	Active           bool                 `json:"active"`
	Platform         string               `json:"platform"`
	WithTerminal     bool                 `json:"with_terminal"`
	WithVSCodeTunnel bool                 `json:"with_vscode_tunnel"`
	WithCodeServer   bool                 `json:"with_code_server"`
	WithSSH          bool                 `json:"with_ssh"`
	WithRunCommand   bool                 `json:"with_run_command"`
	ScheduleEnabled  bool                 `json:"schedule_enabled"`
	AutoStart        bool                 `json:"auto_start"`
	Schedule         []TemplateDetailsDay `json:"schedule"`
	ComputeUnits     uint32               `json:"compute_units"`
	StorageUnits     uint32               `json:"storage_units"`
	Zones            []string             `json:"zones"`
	MaxUptime        uint32               `json:"max_uptime"`
	MaxUptimeUnit    string               `json:"max_uptime_unit"`
	IconURL          string               `json:"icon_url"`
	CustomFields     []CustomFieldDef     `json:"custom_fields"`
}

type TemplateCreateResponse struct {
	Status bool   `json:"status"`
	Id     string `json:"template_id"`
}

type TemplateInfo struct {
	Id              string               `json:"template_id"`
	Name            string               `json:"name"`
	Description     string               `json:"description"`
	Usage           int                  `json:"usage"`
	Deployed        int                  `json:"deployed"`
	Groups          []string             `json:"groups"`
	Platform        string               `json:"platform"`
	Active          bool                 `json:"active"`
	IsManaged       bool                 `json:"is_managed"`
	ScheduleEnabled bool                 `json:"schedule_enabled"`
	AutoStart       bool                 `json:"auto_start"`
	ComputeUnits    uint32               `json:"compute_units"`
	StorageUnits    uint32               `json:"storage_units"`
	Schedule        []TemplateDetailsDay `json:"schedule"`
	Zones           []string             `json:"zones"`
	MaxUptime       uint32               `json:"max_uptime"`
	MaxUptimeUnit   string               `json:"max_uptime_unit"`
	IconURL         string               `json:"icon_url"`
}

type TemplateList struct {
	Count     int            `json:"count"`
	Templates []TemplateInfo `json:"templates"`
}

type TemplateDetailsDay struct {
	Enabled bool   `json:"enabled"`
	From    string `json:"from"`
	To      string `json:"to"`
}

type TemplateDetails struct {
	Name             string               `json:"name"`
	Job              string               `json:"job"`
	Description      string               `json:"description"`
	Volumes          string               `json:"volumes"`
	Usage            int                  `json:"usage"`
	Hash             string               `json:"hash"`
	Deployed         int                  `json:"deployed"`
	Groups           []string             `json:"groups"`
	Platform         string               `json:"platform"`
	Active           bool                 `json:"active"`
	IsManaged        bool                 `json:"is_managed"`
	WithTerminal     bool                 `json:"with_terminal"`
	WithVSCodeTunnel bool                 `json:"with_vscode_tunnel"`
	WithCodeServer   bool                 `json:"with_code_server"`
	WithSSH          bool                 `json:"with_ssh"`
	WithRunCommand   bool                 `json:"with_run_command"`
	ComputeUnits     uint32               `json:"compute_units"`
	StorageUnits     uint32               `json:"storage_units"`
	ScheduleEnabled  bool                 `json:"schedule_enabled"`
	AutoStart        bool                 `json:"auto_start"`
	Schedule         []TemplateDetailsDay `json:"schedule"`
	Zones            []string             `json:"zones"`
	MaxUptime        uint32               `json:"max_uptime"`
	MaxUptimeUnit    string               `json:"max_uptime_unit"`
	IconURL          string               `json:"icon_url"`
	CustomFields     []CustomFieldDef     `json:"custom_fields"`
}

func (c *ApiClient) GetTemplates(ctx context.Context) (*TemplateList, int, error) {
	response := &TemplateList{}

	code, err := c.httpClient.Get(ctx, "/api/templates", response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}

func (c *ApiClient) UpdateTemplate(ctx context.Context, templateId string, request *TemplateUpdateRequest) (int, error) {
	return c.httpClient.Put(ctx, "/api/templates/"+templateId, &request, nil, 200)
}

func (c *ApiClient) CreateTemplate(ctx context.Context, request *TemplateCreateRequest) (string, int, error) {
	response := &TemplateCreateResponse{}

	code, err := c.httpClient.Post(ctx, "/api/templates", &request, &response, 201)
	if err != nil {
		return "", code, err
	}

	return response.Id, code, nil
}

func (c *ApiClient) DeleteTemplate(ctx context.Context, templateId string) (int, error) {
	return c.httpClient.Delete(ctx, "/api/templates/"+templateId, nil, nil, 200)
}

func (c *ApiClient) GetTemplate(ctx context.Context, templateId string) (*TemplateDetails, int, error) {
	response := &TemplateDetails{}

	code, err := c.httpClient.Get(ctx, "/api/templates/"+templateId, response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}
