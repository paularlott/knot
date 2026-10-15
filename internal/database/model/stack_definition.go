package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/log"
)

type StackComponent struct {
	Name            string             `json:"name" db:"name" msgpack:"name"`
	TemplateId      string             `json:"template_id" db:"template_id" msgpack:"template_id"`
	Description     string             `json:"description" db:"description" msgpack:"description"`
	Shell           string             `json:"shell" db:"shell" msgpack:"shell"`
	StartupScriptId string             `json:"startup_script_id" db:"startup_script_id" msgpack:"startup_script_id"`
	DependsOn       []string           `json:"depends_on" db:"depends_on,json" msgpack:"depends_on"`
	CustomFields    []StackCustomField `json:"custom_fields" db:"custom_fields,json" msgpack:"custom_fields"`
	PortForwards    []StackPortForward `json:"port_forwards" db:"port_forwards,json" msgpack:"port_forwards"`
}

type StackCustomField struct {
	Name  string `json:"name" db:"name" msgpack:"name"`
	Value string `json:"value" db:"value" msgpack:"value"`
}

type StackPortForward struct {
	ToSpace    string `json:"to_space" db:"to_space" msgpack:"to_space"`
	LocalPort  int    `json:"local_port" db:"local_port" msgpack:"local_port"`
	RemotePort int    `json:"remote_port" db:"remote_port" msgpack:"remote_port"`
}

type StackDefinition struct {
	Id            string           `json:"stack_definition_id" db:"stack_definition_id,pk" msgpack:"stack_definition_id"`
	UserId        string           `json:"user_id" db:"user_id" msgpack:"user_id"`
	Name          string           `json:"name" db:"name" msgpack:"name"`
	Description   string           `json:"description" db:"description" msgpack:"description"`
	IconURL       string           `json:"icon_url" db:"icon_url" msgpack:"icon_url"`
	Groups        []string         `json:"groups" db:"groups,json" msgpack:"groups"`
	Zones         []string         `json:"zones" db:"zones,json" msgpack:"zones"`
	Active        bool             `json:"active" db:"active" msgpack:"active"`
	IsDeleted     bool             `json:"is_deleted" db:"is_deleted" msgpack:"is_deleted"`
	IsManaged     bool             `json:"is_managed" db:"is_managed" msgpack:"is_managed"`
	Components    []StackComponent `json:"components" db:"components,json" msgpack:"components"`
	CreatedUserId string           `json:"created_user_id" db:"created_user_id" msgpack:"created_user_id"`
	CreatedAt     time.Time        `json:"created_at" db:"created_at" msgpack:"created_at"`
	UpdatedUserId string           `json:"updated_user_id" db:"updated_user_id" msgpack:"updated_user_id"`
	UpdatedAt     hlc.Timestamp    `json:"updated_at" db:"updated_at" msgpack:"updated_at"`
}

func NewStackDefinition(
	name string,
	description string,
	iconUrl string,
	groups []string,
	zones []string,
	active bool,
	components []StackComponent,
	ownerUserId string,
	createdUserId string,
) *StackDefinition {
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal(err.Error())
	}

	return &StackDefinition{
		Id:            id.String(),
		UserId:        ownerUserId,
		Name:          name,
		Description:   description,
		IconURL:       iconUrl,
		Groups:        groups,
		Zones:         zones,
		Active:        active,
		Components:    components,
		CreatedUserId: createdUserId,
		CreatedAt:     time.Now().UTC(),
		UpdatedUserId: createdUserId,
		UpdatedAt:     hlc.Now(),
	}
}

// IsValidForZone determines whether the stack definition is valid for the specified zone.
// Follows the same !-prefix negation logic as Script.IsValidForZone.
func (sd *StackDefinition) IsValidForZone(zone string) bool {
	if len(sd.Zones) == 0 {
		return true
	}

	for _, z := range sd.Zones {
		if len(z) > 0 && z[0] == '!' && z[1:] == zone {
			return false
		}
	}

	for _, z := range sd.Zones {
		if len(z) > 0 && z[0] != '!' && z == zone {
			return true
		}
	}

	return false
}

// IsGlobal returns true if the definition is a system/global definition (UserId is empty)
func (sd *StackDefinition) IsGlobal() bool {
	return sd.UserId == ""
}
