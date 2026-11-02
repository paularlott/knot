package apiclient

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/database/model"
)

type CustomFieldDef struct {
	Name        string   `json:"name" msgpack:"name"`
	Description string   `json:"description" msgpack:"description"`
	Type        string   `json:"type,omitempty" msgpack:"type,omitempty"`
	Handler     string   `json:"handler,omitempty" msgpack:"handler,omitempty"`
	Language    string   `json:"language,omitempty" msgpack:"language,omitempty"`
	Default     string   `json:"default,omitempty" msgpack:"default,omitempty"`
	Required    bool     `json:"required,omitempty" msgpack:"required,omitempty"`
	Options     []string `json:"options,omitempty" msgpack:"options,omitempty"`
	// ShowOnCreate shows an optional field on the create-space form without
	// opening its optional fields toggle.
	ShowOnCreate bool `json:"show_on_create,omitempty" msgpack:"show_on_create,omitempty"`
}

type TemplateCreateRequest struct {
	Name                     string                   `json:"name" msgpack:"name"`
	Job                      string                   `json:"job" msgpack:"job"`
	Description              string                   `json:"description" msgpack:"description"`
	Volumes                  string                   `json:"volumes" msgpack:"volumes"`
	Groups                   []string                 `json:"groups" msgpack:"groups"`
	Platform                 string                   `json:"platform" msgpack:"platform"`
	Active                   bool                     `json:"active" msgpack:"active"`
	WithTerminal             bool                     `json:"with_terminal" msgpack:"with_terminal"`
	WithVSCodeTunnel         bool                     `json:"with_vscode_tunnel" msgpack:"with_vscode_tunnel"`
	WithCodeServer           bool                     `json:"with_code_server" msgpack:"with_code_server"`
	WithSSH                  bool                     `json:"with_ssh" msgpack:"with_ssh"`
	WithRunCommand           bool                     `json:"with_run_command" msgpack:"with_run_command"`
	WithVNC                  bool                     `json:"with_vnc" msgpack:"with_vnc"`
	AllowNodeMigration       bool                     `json:"allow_node_migration" msgpack:"allow_node_migration"`
	StartupScriptId          string                   `json:"startup_script_id" msgpack:"startup_script_id"`
	ShutdownScriptId         string                   `json:"shutdown_script_id" msgpack:"shutdown_script_id"`
	ScheduleEnabled          bool                     `json:"schedule_enabled" msgpack:"schedule_enabled"`
	AutoStart                bool                     `json:"auto_start" msgpack:"auto_start"`
	Schedule                 []TemplateDetailsDay     `json:"schedule" msgpack:"schedule"`
	ComputeUnits             uint32                   `json:"compute_units" msgpack:"compute_units"`
	StorageUnits             uint32                   `json:"storage_units" msgpack:"storage_units"`
	Zones                    []string                 `json:"zones" msgpack:"zones"`
	MaxUptime                uint32                   `json:"max_uptime" msgpack:"max_uptime"`
	MaxUptimeUnit            string                   `json:"max_uptime_unit" msgpack:"max_uptime_unit"`
	IdleTimeout              uint32                   `json:"idle_timeout" msgpack:"idle_timeout"`
	IdleTimeoutUnit          string                   `json:"idle_timeout_unit" msgpack:"idle_timeout_unit"`
	IconURL                  string                   `json:"icon_url" msgpack:"icon_url"`
	CustomFields             []CustomFieldDef         `json:"custom_fields" msgpack:"custom_fields"`
	HealthCheckType          string                   `json:"health_check_type" msgpack:"health_check_type"`
	HealthCheckConfig        string                   `json:"health_check_config" msgpack:"health_check_config"`
	HealthCheckSkipSSLVerify bool                     `json:"health_check_skip_ssl_verify" msgpack:"health_check_skip_ssl_verify"`
	HealthCheckTimeout       uint32                   `json:"health_check_timeout" msgpack:"health_check_timeout"`
	HealthCheckInterval      uint32                   `json:"health_check_interval" msgpack:"health_check_interval"`
	HealthCheckMaxFailures   uint32                   `json:"health_check_max_failures" msgpack:"health_check_max_failures"`
	HealthCheckAutoRestart   bool                     `json:"health_check_auto_restart" msgpack:"health_check_auto_restart"`
	DisableUserActivity      bool                     `json:"disable_user_activity" msgpack:"disable_user_activity"`
	DisableFileStorage       bool                     `json:"disable_file_storage" msgpack:"disable_file_storage"`
	DisableUserSecrets       bool                     `json:"disable_user_secrets" msgpack:"disable_user_secrets"`
	Ports                    []model.TemplatePort     `json:"ports" msgpack:"ports"`
	PortForwards             []model.PortForwardEntry `json:"port_forwards" msgpack:"port_forwards"`
	Jobs                     []model.SpaceJob         `json:"jobs" msgpack:"jobs"`
}

type TemplateUpdateRequest struct {
	Name                     string                   `json:"name" msgpack:"name"`
	Job                      string                   `json:"job" msgpack:"job"`
	Description              string                   `json:"description" msgpack:"description"`
	Volumes                  string                   `json:"volumes" msgpack:"volumes"`
	Groups                   []string                 `json:"groups" msgpack:"groups"`
	Active                   bool                     `json:"active" msgpack:"active"`
	Platform                 string                   `json:"platform" msgpack:"platform"`
	WithTerminal             bool                     `json:"with_terminal" msgpack:"with_terminal"`
	WithVSCodeTunnel         bool                     `json:"with_vscode_tunnel" msgpack:"with_vscode_tunnel"`
	WithCodeServer           bool                     `json:"with_code_server" msgpack:"with_code_server"`
	WithSSH                  bool                     `json:"with_ssh" msgpack:"with_ssh"`
	WithRunCommand           bool                     `json:"with_run_command" msgpack:"with_run_command"`
	WithVNC                  bool                     `json:"with_vnc" msgpack:"with_vnc"`
	AllowNodeMigration       bool                     `json:"allow_node_migration" msgpack:"allow_node_migration"`
	StartupScriptId          string                   `json:"startup_script_id" msgpack:"startup_script_id"`
	ShutdownScriptId         string                   `json:"shutdown_script_id" msgpack:"shutdown_script_id"`
	ScheduleEnabled          bool                     `json:"schedule_enabled" msgpack:"schedule_enabled"`
	AutoStart                bool                     `json:"auto_start" msgpack:"auto_start"`
	Schedule                 []TemplateDetailsDay     `json:"schedule" msgpack:"schedule"`
	ComputeUnits             uint32                   `json:"compute_units" msgpack:"compute_units"`
	StorageUnits             uint32                   `json:"storage_units" msgpack:"storage_units"`
	Zones                    []string                 `json:"zones" msgpack:"zones"`
	MaxUptime                uint32                   `json:"max_uptime" msgpack:"max_uptime"`
	MaxUptimeUnit            string                   `json:"max_uptime_unit" msgpack:"max_uptime_unit"`
	IdleTimeout              uint32                   `json:"idle_timeout" msgpack:"idle_timeout"`
	IdleTimeoutUnit          string                   `json:"idle_timeout_unit" msgpack:"idle_timeout_unit"`
	IconURL                  string                   `json:"icon_url" msgpack:"icon_url"`
	CustomFields             []CustomFieldDef         `json:"custom_fields" msgpack:"custom_fields"`
	HealthCheckType          string                   `json:"health_check_type" msgpack:"health_check_type"`
	HealthCheckConfig        string                   `json:"health_check_config" msgpack:"health_check_config"`
	HealthCheckSkipSSLVerify bool                     `json:"health_check_skip_ssl_verify" msgpack:"health_check_skip_ssl_verify"`
	HealthCheckTimeout       uint32                   `json:"health_check_timeout" msgpack:"health_check_timeout"`
	HealthCheckInterval      uint32                   `json:"health_check_interval" msgpack:"health_check_interval"`
	HealthCheckMaxFailures   uint32                   `json:"health_check_max_failures" msgpack:"health_check_max_failures"`
	HealthCheckAutoRestart   bool                     `json:"health_check_auto_restart" msgpack:"health_check_auto_restart"`
	DisableUserActivity      bool                     `json:"disable_user_activity" msgpack:"disable_user_activity"`
	DisableFileStorage       bool                     `json:"disable_file_storage" msgpack:"disable_file_storage"`
	DisableUserSecrets       bool                     `json:"disable_user_secrets" msgpack:"disable_user_secrets"`
	Ports                    []model.TemplatePort     `json:"ports" msgpack:"ports"`
	PortForwards             []model.PortForwardEntry `json:"port_forwards" msgpack:"port_forwards"`
	Jobs                     []model.SpaceJob         `json:"jobs" msgpack:"jobs"`
}

type TemplateCreateResponse struct {
	Status bool   `json:"status" msgpack:"status"`
	Id     string `json:"template_id" msgpack:"template_id"`
}

type TemplateInfo struct {
	Id                 string                   `json:"template_id" msgpack:"template_id"`
	Name               string                   `json:"name" msgpack:"name"`
	Description        string                   `json:"description" msgpack:"description"`
	Usage              int                      `json:"usage" msgpack:"usage"`
	Deployed           int                      `json:"deployed" msgpack:"deployed"`
	Groups             []string                 `json:"groups" msgpack:"groups"`
	Platform           string                   `json:"platform" msgpack:"platform"`
	Active             bool                     `json:"active" msgpack:"active"`
	IsManaged          bool                     `json:"is_managed" msgpack:"is_managed"`
	AllowNodeMigration bool                     `json:"allow_node_migration" msgpack:"allow_node_migration"`
	WithVNC            bool                     `json:"with_vnc" msgpack:"with_vnc"`
	ScheduleEnabled    bool                     `json:"schedule_enabled" msgpack:"schedule_enabled"`
	AutoStart          bool                     `json:"auto_start" msgpack:"auto_start"`
	ComputeUnits       uint32                   `json:"compute_units" msgpack:"compute_units"`
	StorageUnits       uint32                   `json:"storage_units" msgpack:"storage_units"`
	Schedule           []TemplateDetailsDay     `json:"schedule" msgpack:"schedule"`
	Zones              []string                 `json:"zones" msgpack:"zones"`
	MaxUptime          uint32                   `json:"max_uptime" msgpack:"max_uptime"`
	MaxUptimeUnit      string                   `json:"max_uptime_unit" msgpack:"max_uptime_unit"`
	IdleTimeout        uint32                   `json:"idle_timeout" msgpack:"idle_timeout"`
	IdleTimeoutUnit    string                   `json:"idle_timeout_unit" msgpack:"idle_timeout_unit"`
	IconURL            string                   `json:"icon_url" msgpack:"icon_url"`
	Ports              []model.TemplatePort     `json:"ports" msgpack:"ports"`
	PortForwards       []model.PortForwardEntry `json:"port_forwards" msgpack:"port_forwards"`
	Jobs               []model.SpaceJob         `json:"jobs" msgpack:"jobs"`
	CustomFields       []CustomFieldDef         `json:"custom_fields" msgpack:"custom_fields"`
	KvmNetworkMode     string                   `json:"kvm_network_mode,omitempty" msgpack:"kvm_network_mode,omitempty"`
	KvmNetworkCidr     string                   `json:"kvm_network_cidr,omitempty" msgpack:"kvm_network_cidr,omitempty"`
	KvmIPRangeStart    string                   `json:"kvm_ip_range_start,omitempty" msgpack:"kvm_ip_range_start,omitempty"`
	KvmIPRangeEnd      string                   `json:"kvm_ip_range_end,omitempty" msgpack:"kvm_ip_range_end,omitempty"`
	KvmGateway         string                   `json:"kvm_gateway,omitempty" msgpack:"kvm_gateway,omitempty"`
	KvmBridge          string                   `json:"kvm_bridge,omitempty" msgpack:"kvm_bridge,omitempty"`
	// RuntimeAvailable reports whether any node in the zone currently
	// offers the runtime the template needs (always true for manual and
	// Nomad templates).
	RuntimeAvailable bool `json:"runtime_available" msgpack:"runtime_available"`
}

type TemplateList struct {
	Count     int            `json:"count" msgpack:"count"`
	Templates []TemplateInfo `json:"templates" msgpack:"templates"`
}

type TemplateDetailsDay struct {
	Enabled bool   `json:"enabled" msgpack:"enabled"`
	From    string `json:"from" msgpack:"from"`
	To      string `json:"to" msgpack:"to"`
}

type TemplateDetails struct {
	TemplateId               string                   `json:"template_id" msgpack:"template_id"`
	Name                     string                   `json:"name" msgpack:"name"`
	Job                      string                   `json:"job" msgpack:"job"`
	Description              string                   `json:"description" msgpack:"description"`
	Volumes                  string                   `json:"volumes" msgpack:"volumes"`
	Usage                    int                      `json:"usage" msgpack:"usage"`
	Hash                     string                   `json:"hash" msgpack:"hash"`
	Deployed                 int                      `json:"deployed" msgpack:"deployed"`
	Groups                   []string                 `json:"groups" msgpack:"groups"`
	Platform                 string                   `json:"platform" msgpack:"platform"`
	Active                   bool                     `json:"active" msgpack:"active"`
	IsManaged                bool                     `json:"is_managed" msgpack:"is_managed"`
	WithTerminal             bool                     `json:"with_terminal" msgpack:"with_terminal"`
	WithVSCodeTunnel         bool                     `json:"with_vscode_tunnel" msgpack:"with_vscode_tunnel"`
	WithCodeServer           bool                     `json:"with_code_server" msgpack:"with_code_server"`
	WithSSH                  bool                     `json:"with_ssh" msgpack:"with_ssh"`
	WithRunCommand           bool                     `json:"with_run_command" msgpack:"with_run_command"`
	WithVNC                  bool                     `json:"with_vnc" msgpack:"with_vnc"`
	AllowNodeMigration       bool                     `json:"allow_node_migration" msgpack:"allow_node_migration"`
	StartupScriptId          string                   `json:"startup_script_id" msgpack:"startup_script_id"`
	ShutdownScriptId         string                   `json:"shutdown_script_id" msgpack:"shutdown_script_id"`
	ComputeUnits             uint32                   `json:"compute_units" msgpack:"compute_units"`
	StorageUnits             uint32                   `json:"storage_units" msgpack:"storage_units"`
	ScheduleEnabled          bool                     `json:"schedule_enabled" msgpack:"schedule_enabled"`
	AutoStart                bool                     `json:"auto_start" msgpack:"auto_start"`
	Schedule                 []TemplateDetailsDay     `json:"schedule" msgpack:"schedule"`
	Zones                    []string                 `json:"zones" msgpack:"zones"`
	MaxUptime                uint32                   `json:"max_uptime" msgpack:"max_uptime"`
	MaxUptimeUnit            string                   `json:"max_uptime_unit" msgpack:"max_uptime_unit"`
	IdleTimeout              uint32                   `json:"idle_timeout" msgpack:"idle_timeout"`
	IdleTimeoutUnit          string                   `json:"idle_timeout_unit" msgpack:"idle_timeout_unit"`
	IconURL                  string                   `json:"icon_url" msgpack:"icon_url"`
	CustomFields             []CustomFieldDef         `json:"custom_fields" msgpack:"custom_fields"`
	HealthCheckType          string                   `json:"health_check_type" msgpack:"health_check_type"`
	HealthCheckConfig        string                   `json:"health_check_config" msgpack:"health_check_config"`
	HealthCheckSkipSSLVerify bool                     `json:"health_check_skip_ssl_verify" msgpack:"health_check_skip_ssl_verify"`
	HealthCheckTimeout       uint32                   `json:"health_check_timeout" msgpack:"health_check_timeout"`
	HealthCheckInterval      uint32                   `json:"health_check_interval" msgpack:"health_check_interval"`
	HealthCheckMaxFailures   uint32                   `json:"health_check_max_failures" msgpack:"health_check_max_failures"`
	HealthCheckAutoRestart   bool                     `json:"health_check_auto_restart" msgpack:"health_check_auto_restart"`
	DisableUserActivity      bool                     `json:"disable_user_activity" msgpack:"disable_user_activity"`
	DisableFileStorage       bool                     `json:"disable_file_storage" msgpack:"disable_file_storage"`
	DisableUserSecrets       bool                     `json:"disable_user_secrets" msgpack:"disable_user_secrets"`
	Ports                    []model.TemplatePort     `json:"ports" msgpack:"ports"`
	PortForwards             []model.PortForwardEntry `json:"port_forwards" msgpack:"port_forwards"`
	Jobs                     []model.SpaceJob         `json:"jobs" msgpack:"jobs"`
	KvmNetworkMode           string                   `json:"kvm_network_mode,omitempty" msgpack:"kvm_network_mode,omitempty"`
	KvmNetworkCidr           string                   `json:"kvm_network_cidr,omitempty" msgpack:"kvm_network_cidr,omitempty"`
	KvmIPRangeStart          string                   `json:"kvm_ip_range_start,omitempty" msgpack:"kvm_ip_range_start,omitempty"`
	KvmIPRangeEnd            string                   `json:"kvm_ip_range_end,omitempty" msgpack:"kvm_ip_range_end,omitempty"`
	KvmGateway               string                   `json:"kvm_gateway,omitempty" msgpack:"kvm_gateway,omitempty"`
	KvmBridge                string                   `json:"kvm_bridge,omitempty" msgpack:"kvm_bridge,omitempty"`
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

func (c *ApiClient) GetTemplateByName(ctx context.Context, name string) (*TemplateDetails, error) {
	templates, _, err := c.GetTemplates(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range templates.Templates {
		if t.Name == name {
			template, _, err := c.GetTemplate(ctx, t.Id)
			return template, err
		}
	}
	return nil, fmt.Errorf("template not found: %s", name)
}

// ExportTemplate fetches a template as portable YAML text.
func (c *ApiClient) ExportTemplate(ctx context.Context, templateId string) (string, int, error) {
	var buf string
	code, err := c.httpClient.Get(ctx, "/api/templates/"+templateId+"/export", &buf)
	if err != nil {
		return "", code, err
	}
	return buf, code, nil
}

// ImportTemplate sends a portable YAML export to the server and creates or
// updates a template. Returns the template ID.
func (c *ApiClient) ImportTemplate(ctx context.Context, yamlText string) (string, int, error) {
	resp := &TemplateCreateResponse{}
	code, err := c.httpClient.Post(ctx, "/api/templates/import", yamlText, resp, 0)
	if err != nil {
		return "", code, err
	}
	return resp.Id, code, nil
}
