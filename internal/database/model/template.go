package model

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"time"
	_ "time/tzdata"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/config"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const (
	PlatformManual = "manual"
	PlatformDocker = "docker"
	PlatformPodman = "podman"
	PlatformNomad  = "nomad"

	LeafNodeZone = "<leaf-node>"
)

// Template object
type Template struct {
	Id               string                 `json:"template_id" db:"template_id,pk"`
	Name             string                 `json:"name" db:"name"`
	Description      string                 `json:"description" db:"description"`
	Hash             string                 `json:"hash" db:"hash"`
	Platform         string                 `json:"platform" db:"platform"`
	IconURL          string                 `json:"icon_url" db:"icon_url"`
	Job              string                 `json:"job" db:"job"`
	Volumes          string                 `json:"volumes" db:"volumes"`
	Groups           []string               `json:"groups" db:"groups,json"`
	Active           bool                   `json:"active" db:"active"`
	WithTerminal     bool                   `json:"with_terminal" db:"with_terminal"`
	WithVSCodeTunnel bool                   `json:"with_vscode_tunnel" db:"with_vscode_tunnel"`
	WithCodeServer   bool                   `json:"with_code_server" db:"with_code_server"`
	WithSSH          bool                   `json:"with_ssh" db:"with_ssh"`
	WithRunCommand   bool                   `json:"with_run_command" db:"with_run_command"`
	ComputeUnits     uint32                 `json:"compute_units" db:"compute_units"`
	StorageUnits     uint32                 `json:"storage_units" db:"storage_units"`
	ScheduleEnabled  bool                   `json:"schedule_enabled" db:"schedule_enabled"`
	AutoStart        bool                   `json:"auto_start" db:"auto_start"`
	IsDeleted        bool                   `json:"is_deleted" db:"is_deleted"`
	IsManaged        bool                   `json:"is_managed" db:"is_managed"`
	Schedule         []TemplateScheduleDays `json:"schedule" db:"schedule,json"`
	Zones            []string               `json:"zones" db:"zones,json"`
	CustomFields     []TemplateCustomField  `json:"custom_fields" db:"custom_fields,json"`
	MaxUptime        uint32                 `json:"max_uptime" db:"max_uptime"`
	MaxUptimeUnit    string                 `json:"max_uptime_unit" db:"max_uptime_unit"`
	CreatedUserId    string                 `json:"created_user_id" db:"created_user_id"`
	CreatedAt        time.Time              `json:"created_at" db:"created_at"`
	UpdatedUserId    string                 `json:"updated_user_id" db:"updated_user_id"`
	UpdatedAt        hlc.Timestamp          `json:"updated_at" db:"updated_at"`
}

type TemplateScheduleDays struct {
	Enabled bool   `json:"enabled"`
	From    string `json:"from"`
	To      string `json:"to"`
}

type TemplateCustomField struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func NewTemplate(
	name string,
	description string,
	job string,
	volumes string,
	userId string,
	groups []string,
	platform string,
	withTerminal bool,
	withVSCodeTunnel bool,
	withCodeServer bool,
	withSSH bool,
	withRunCommand bool,
	computeUnits uint32,
	storageUnits uint32,
	scheduleEnabled bool,
	schedule *[]TemplateScheduleDays,
	zones []string,
	autoStart bool,
	active bool,
	maxUptime uint32,
	maxUptimeUnit string,
	iconURL string,
	customFields []TemplateCustomField,
) *Template {
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal().Msg(err.Error())
	}

	template := &Template{
		Id:               id.String(),
		Name:             name,
		Description:      description,
		Job:              job,
		Volumes:          volumes,
		Groups:           groups,
		Zones:            zones,
		CreatedUserId:    userId,
		Platform:         platform,
		WithTerminal:     withTerminal,
		WithVSCodeTunnel: withVSCodeTunnel,
		WithCodeServer:   withCodeServer,
		WithSSH:          withSSH,
		WithRunCommand:   withRunCommand,
		ComputeUnits:     computeUnits,
		StorageUnits:     storageUnits,
		CreatedAt:        time.Now().UTC(),
		UpdatedUserId:    userId,
		UpdatedAt:        hlc.Now(),
		IconURL:          iconURL,
		Active:           active,
		MaxUptime:        maxUptime,
		MaxUptimeUnit:    maxUptimeUnit,
		CustomFields:     customFields,
	}
	template.UpdateHash()

	if !scheduleEnabled || schedule == nil {
		template.ScheduleEnabled = false
		template.Schedule = nil
	} else {
		template.ScheduleEnabled = true
		template.Schedule = *schedule
		template.AutoStart = autoStart
	}

	return template
}

func (template *Template) GetVolumes(space *Space, user *User, variables map[string]interface{}) (*CSIVolumes, error) {
	return LoadVolumesFromYaml(template.Volumes, template, space, user, variables)
}

func (template *Template) UpdateHash() {
	hash := md5.Sum([]byte(template.Job + template.Volumes + template.Platform + fmt.Sprintf("%t%t%t%t%t%v", template.WithTerminal, template.WithVSCodeTunnel, template.WithCodeServer, template.WithSSH, template.WithRunCommand, template.CustomFields)))
	template.Hash = hex.EncodeToString(hash[:])
}

func (template *Template) AllowedBySchedule() bool {
	if !template.ScheduleEnabled {
		return true
	}

	// Get the timezone
	cfg := config.GetServerConfig()
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		log.Error().Msgf("Error loading timezone: %s", err)
		return false
	}

	// Get the current time and adjust for the server timezone
	now := time.Now().In(loc)

	// Get the day of the week
	dayOfWeek := int(now.Weekday())

	// Get the day schedule, if it doesn't exist then return false
	if len(template.Schedule) <= int(dayOfWeek) {
		return false
	}
	daySchedule := template.Schedule[dayOfWeek]
	if !daySchedule.Enabled {
		return false
	}

	from, err := time.Parse("3:04pm", daySchedule.From)
	if err != nil {
		log.Error().Msgf("Error parsing schedule from time: %s", err)
		return false
	}
	to, err := time.Parse("3:04pm", daySchedule.To)
	if err != nil {
		log.Error().Msgf("Error parsing schedule to time: %s", err)
		return false
	}

	// Test if now is between from and to times
	if now.Hour() >= from.Hour() && now.Hour() <= to.Hour() {
		if now.Hour() == from.Hour() && now.Minute() < from.Minute() {
			return false
		} else if now.Hour() == to.Hour() && now.Minute() > to.Minute() {
			return false
		}

		return true
	}

	return false
}

func (template *Template) IsManual() bool {
	return template.Platform == PlatformManual
}

func (template *Template) IsLocalContainer() bool {
	return template.Platform == PlatformDocker || template.Platform == PlatformPodman
}

// IsValidForZone determines whether the template is valid for deployment in the specified zone.
// The function evaluates zone restrictions based on the template's Zones configuration.
// If no zones are specified, the template is considered valid for all zones.
// Zone names prefixed with '!' are treated as exclusions (negated zones).
// The function first checks for exclusions, then checks for explicit inclusions.
//
// zone is the target zone name to validate against the template's zone restrictions.
//
// Returns true if the template can be deployed in the specified zone, false otherwise.
func (template *Template) IsValidForZone(zone string) bool {
    // If no zones specified, template is valid for all zones
    if len(template.Zones) == 0 {
        return true
    }

    // Check for negated zones first
    for _, z := range template.Zones {
        if z[0] == '!' && z[1:] == zone {
            return false
        }
    }

    // Check for positive zones
    for _, z := range template.Zones {
        if z[0] != '!' && z == zone {
            return true
        }
    }

    return false
}
