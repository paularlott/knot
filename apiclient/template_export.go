package apiclient

import (
	"fmt"

	"github.com/paularlott/knot/internal/database/model"
	"gopkg.in/yaml.v3"
)

// TemplateExport is the portable YAML representation of a template. It can be
// exported via `knot template export` and imported via `knot template import`.
// Scripts are referenced by name (not UUID) so exports are portable across
// instances. Template variables (${{ .X }}) in job/volumes are preserved
// verbatim — they resolve at deploy time.
type TemplateExport struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Platform    string `yaml:"platform"`
	IconURL     string `yaml:"icon_url,omitempty"`
	Active      bool   `yaml:"active"`

	ComputeUnits uint32 `yaml:"compute_units"`
	StorageUnits uint32 `yaml:"storage_units,omitempty"`

	Features TemplateExportFeatures `yaml:"features,omitempty"`

	Groups []string `yaml:"groups,omitempty"`
	Zones  []string `yaml:"zones,omitempty"`

	MaxUptime     uint32 `yaml:"max_uptime,omitempty"`
	MaxUptimeUnit string `yaml:"max_uptime_unit,omitempty"`

	ScheduleEnabled bool                         `yaml:"schedule_enabled,omitempty"`
	Schedule        []TemplateExportScheduleDay  `yaml:"schedule,omitempty"`
	AutoStart       bool                         `yaml:"auto_start,omitempty"`

	CustomFields []TemplateExportCustomField `yaml:"custom_fields,omitempty"`

	StartupScript  string `yaml:"startup_script,omitempty"`
	ShutdownScript string `yaml:"shutdown_script,omitempty"`

	HealthCheckType             string `yaml:"health_check_type,omitempty"`
	HealthCheckConfig           string `yaml:"health_check_config,omitempty"`
	HealthCheckSkipSSLVerify    bool   `yaml:"health_check_skip_ssl_verify,omitempty"`
	HealthCheckTimeout          uint32 `yaml:"health_check_timeout,omitempty"`
	HealthCheckInterval         uint32 `yaml:"health_check_interval,omitempty"`
	HealthCheckMaxFailures      uint32 `yaml:"health_check_max_failures,omitempty"`
	HealthCheckAutoRestart      bool   `yaml:"health_check_auto_restart,omitempty"`

	DisableUserActivity bool           `yaml:"disable_user_activity,omitempty"`
	Ports               []model.TemplatePort `yaml:"ports,omitempty"`

	Job     string `yaml:"job,omitempty"`
	Volumes string `yaml:"volumes,omitempty"`
}

type TemplateExportFeatures struct {
	WithTerminal      bool `yaml:"with_terminal,omitempty"`
	WithVSCodeTunnel  bool `yaml:"with_vscode_tunnel,omitempty"`
	WithCodeServer    bool `yaml:"with_code_server,omitempty"`
	WithSSH           bool `yaml:"with_ssh,omitempty"`
	WithRunCommand    bool `yaml:"with_run_command,omitempty"`
	AllowNodeMigration bool `yaml:"allow_node_migration,omitempty"`
}

type TemplateExportScheduleDay struct {
	Enabled bool   `yaml:"enabled,omitempty"`
	From    string `yaml:"from,omitempty"`
	To      string `yaml:"to,omitempty"`
}

type TemplateExportCustomField struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
}

// MarshalYAML implements yaml.Marshaler to emit job/volumes as block scalars
// (literal `|`) for readability in version control.
func (e TemplateExport) MarshalYAML() ([]byte, error) {
	type alias struct {
		TemplateExport `yaml:",inline"`
	}
	// Use the inline alias so yaml.v3 applies our tags, then force the
	// job/volumes fields to literal block style via a custom node.
	out, err := yaml.Marshal(alias{TemplateExport: e})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// String returns the YAML representation, terminated with a trailing newline.
func (e TemplateExport) String() (string, error) {
	data, err := yaml.Marshal(e)
	if err != nil {
		return "", fmt.Errorf("marshal template export: %w", err)
	}
	return string(data), nil
}

// ParseTemplateExport decodes YAML bytes into a TemplateExport.
func ParseTemplateExport(data []byte) (*TemplateExport, error) {
	var exp TemplateExport
	if err := yaml.Unmarshal(data, &exp); err != nil {
		return nil, fmt.Errorf("parse template export YAML: %w", err)
	}
	return &exp, nil
}

// ToCreateRequest converts the export format into a TemplateCreateRequest for
// the existing create API. Script names should already be resolved to IDs by
// the caller.
func (e *TemplateExport) ToCreateRequest() *TemplateCreateRequest {
	req := &TemplateCreateRequest{
		Name:                       e.Name,
		Description:                e.Description,
		Job:                        e.Job,
		Volumes:                    e.Volumes,
		Groups:                     defaultSlice(e.Groups),
		Platform:                   e.Platform,
		IconURL:                    e.IconURL,
		Active:                     e.Active,
		WithTerminal:               e.Features.WithTerminal,
		WithVSCodeTunnel:           e.Features.WithVSCodeTunnel,
		WithCodeServer:             e.Features.WithCodeServer,
		WithSSH:                    e.Features.WithSSH,
		WithRunCommand:             e.Features.WithRunCommand,
		AllowNodeMigration:         e.Features.AllowNodeMigration,
		StartupScriptId:            e.StartupScript,
		ShutdownScriptId:           e.ShutdownScript,
		ComputeUnits:               e.ComputeUnits,
		StorageUnits:               e.StorageUnits,
		ScheduleEnabled:            e.ScheduleEnabled,
		AutoStart:                  e.AutoStart,
		Zones:                      defaultSlice(e.Zones),
		MaxUptime:                  e.MaxUptime,
		MaxUptimeUnit:              e.MaxUptimeUnit,
		HealthCheckType:            e.HealthCheckType,
		HealthCheckConfig:          e.HealthCheckConfig,
		HealthCheckSkipSSLVerify:   e.HealthCheckSkipSSLVerify,
		HealthCheckTimeout:         e.HealthCheckTimeout,
		HealthCheckInterval:        e.HealthCheckInterval,
		HealthCheckMaxFailures:     e.HealthCheckMaxFailures,
		HealthCheckAutoRestart:     e.HealthCheckAutoRestart,
		DisableUserActivity:        e.DisableUserActivity,
		Ports:                      defaultPorts(e.Ports),
	}
	req.CustomFields = defaultCustomFields(e.CustomFields)
	if len(e.Schedule) > 0 {
		req.Schedule = make([]TemplateDetailsDay, len(e.Schedule))
		for i, s := range e.Schedule {
			req.Schedule[i] = TemplateDetailsDay{Enabled: s.Enabled, From: s.From, To: s.To}
		}
	}
	return req
}

// ExportFromDetails converts a TemplateDetails (API response) into the portable
// export format. The caller must resolve script IDs to names before calling
// this (set StartupScript/ShutdownScript on the returned struct).
func ExportFromDetails(d *TemplateDetails) *TemplateExport {
	exp := &TemplateExport{
		Name:                        d.Name,
		Description:                 d.Description,
		Platform:                    d.Platform,
		IconURL:                     d.IconURL,
		Active:                      d.Active,
		ComputeUnits:                d.ComputeUnits,
		StorageUnits:                d.StorageUnits,
		Groups:                      d.Groups,
		Zones:                       d.Zones,
		MaxUptime:                   d.MaxUptime,
		MaxUptimeUnit:               d.MaxUptimeUnit,
		ScheduleEnabled:             d.ScheduleEnabled,
		AutoStart:                   d.AutoStart,
		StartupScript:               d.StartupScriptId,
		ShutdownScript:              d.ShutdownScriptId,
		HealthCheckType:             d.HealthCheckType,
		HealthCheckConfig:           d.HealthCheckConfig,
		HealthCheckSkipSSLVerify:    d.HealthCheckSkipSSLVerify,
		HealthCheckTimeout:          d.HealthCheckTimeout,
		HealthCheckInterval:         d.HealthCheckInterval,
		HealthCheckMaxFailures:      d.HealthCheckMaxFailures,
		HealthCheckAutoRestart:      d.HealthCheckAutoRestart,
		DisableUserActivity:         d.DisableUserActivity,
		Ports:                       d.Ports,
		Job:                         d.Job,
		Volumes:                     d.Volumes,
		Features: TemplateExportFeatures{
			WithTerminal:       d.WithTerminal,
			WithVSCodeTunnel:   d.WithVSCodeTunnel,
			WithCodeServer:     d.WithCodeServer,
			WithSSH:            d.WithSSH,
			WithRunCommand:     d.WithRunCommand,
			AllowNodeMigration: d.AllowNodeMigration,
		},
	}
	if len(d.CustomFields) > 0 {
		exp.CustomFields = make([]TemplateExportCustomField, len(d.CustomFields))
		for i, cf := range d.CustomFields {
			exp.CustomFields[i] = TemplateExportCustomField{Name: cf.Name, Description: cf.Description}
		}
	}
	if len(d.Schedule) > 0 {
		exp.Schedule = make([]TemplateExportScheduleDay, len(d.Schedule))
		for i, s := range d.Schedule {
			exp.Schedule[i] = TemplateExportScheduleDay{Enabled: s.Enabled, From: s.From, To: s.To}
		}
	}
	return exp
}

// defaultSlice returns s if non-nil, otherwise an empty slice. Prevents null
// in JSON when YAML omits the field.
func defaultSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func defaultPorts(p []model.TemplatePort) []model.TemplatePort {
	if p == nil {
		return []model.TemplatePort{}
	}
	return p
}

func defaultCustomFields(cf []TemplateExportCustomField) []CustomFieldDef {
	if len(cf) == 0 {
		return []CustomFieldDef{}
	}
	out := make([]CustomFieldDef, len(cf))
	for i, c := range cf {
		out[i] = CustomFieldDef{Name: c.Name, Description: c.Description}
	}
	return out
}
