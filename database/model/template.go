package model

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"time"
	_ "time/tzdata"

	"github.com/google/uuid"
	"github.com/paularlott/knot/internal/origin_leaf/server_info"
	"github.com/rs/zerolog/log"
)

// Template object
type Template struct {
	Id               string                 `json:"template_id" db:"template_id,pk"`
	Name             string                 `json:"name" db:"name"`
	Description      string                 `json:"description" db:"description"`
	Hash             string                 `json:"hash" db:"hash"`
	Job              string                 `json:"job" db:"job"`
	Volumes          string                 `json:"volumes" db:"volumes"`
	Groups           []string               `json:"groups" db:"groups,json"`
	LocalContainer   bool                   `json:"local_container" db:"local_container"`
	IsManual         bool                   `json:"is_manual" db:"is_manual"`
	WithTerminal     bool                   `json:"with_terminal" db:"with_terminal"`
	WithVSCodeTunnel bool                   `json:"with_vscode_tunnel" db:"with_vscode_tunnel"`
	WithCodeServer   bool                   `json:"with_code_server" db:"with_code_server"`
	WithSSH          bool                   `json:"with_ssh" db:"with_ssh"`
	ComputeUnits     uint32                 `json:"compute_units" db:"compute_units"`
	StorageUnits     uint32                 `json:"storage_units" db:"storage_units"`
	ScheduleEnabled  bool                   `json:"schedule_enabled" db:"schedule_enabled"`
	IsDeleted        bool                   `json:"is_deleted" db:"is_deleted"`
	Schedule         []TemplateScheduleDays `json:"schedule" db:"schedule,json"`
	Locations        []string               `json:"locations" db:"locations,json"`
	CreatedUserId    string                 `json:"created_user_id" db:"created_user_id"`
	CreatedAt        time.Time              `json:"created_at" db:"created_at"`
	UpdatedUserId    string                 `json:"updated_user_id" db:"updated_user_id"`
	UpdatedAt        time.Time              `json:"updated_at" db:"updated_at"`
}

type TemplateScheduleDays struct {
	Enabled bool   `json:"enabled"`
	From    string `json:"from"`
	To      string `json:"to"`
}

func NewTemplate(name string, description string, job string, volumes string, userId string, groups []string, localContainer bool, isManual bool, withTerminal bool, withVSCodeTunnel bool, withCodeServer bool, withSSH bool, computeUnits uint32, storageUnits uint32, scheduleEnabled bool, schedule *[]TemplateScheduleDays, locations []string) *Template {
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
		Locations:        locations,
		CreatedUserId:    userId,
		LocalContainer:   localContainer,
		IsManual:         isManual,
		WithTerminal:     withTerminal,
		WithVSCodeTunnel: withVSCodeTunnel,
		WithCodeServer:   withCodeServer,
		WithSSH:          withSSH,
		ComputeUnits:     computeUnits,
		StorageUnits:     storageUnits,
		CreatedAt:        time.Now().UTC(),
		UpdatedUserId:    userId,
		UpdatedAt:        time.Now().UTC(),
	}
	template.UpdateHash()

	if !scheduleEnabled || schedule == nil {
		template.ScheduleEnabled = false
		template.Schedule = nil
	} else {
		template.ScheduleEnabled = true
		template.Schedule = *schedule
	}

	return template
}

func (template *Template) GetVolumes(space *Space, user *User, variables *map[string]interface{}) (*CSIVolumes, error) {
	return LoadVolumesFromYaml(template.Volumes, template, space, user, variables)
}

func (template *Template) UpdateHash() {
	hash := md5.Sum([]byte(template.Job + template.Volumes + fmt.Sprintf("%t%t%t%t", template.WithTerminal, template.WithVSCodeTunnel, template.WithCodeServer, template.WithSSH)))
	template.Hash = hex.EncodeToString(hash[:])
}

func (template *Template) AllowedBySchedule() bool {
	if !template.ScheduleEnabled {
		return true
	}

	// Get the timezone
	loc, err := time.LoadLocation(server_info.Timezone)
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
