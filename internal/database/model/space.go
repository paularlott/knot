package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/paularlott/knot/internal/sshd"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type SpaceVolume struct {
	Id        string `json:"id"`
	Namespace string `json:"Namespace"`
}

// Value implements the driver.Valuer interface.
func (sv SpaceVolume) Value() (driver.Value, error) {
	return json.Marshal(sv)
}

// Scan implements the sql.Scanner interface.
func (sv *SpaceVolume) Scan(value interface{}) error {
	log.Warn().Msg("Scan")
	b, ok := value.([]byte)
	if !ok {
		return nil
	}

	return json.Unmarshal(b, &sv)
}

// VolumeDataMap is a custom type that implements the sql.Scanner and driver.Valuer interfaces
type VolumeDataMap map[string]SpaceVolume

// Value implements the driver.Valuer interface.
func (v VolumeDataMap) Value() (driver.Value, error) {
	j, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return driver.Value([]byte(j)), nil
}

// Scan implements the sql.Scanner interface.
func (v *VolumeDataMap) Scan(value interface{}) error {
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to unmarshal JSONB value: %v", value)
	}

	return json.Unmarshal(bytes, v)
}

// Space object
type Space struct {
	Id               string        `json:"space_id" db:"space_id,pk" msgpack:"space_id"`
	ParentSpaceId    string        `json:"parent_space_id" db:"parent_space_id" msgpack:"parent_space_id"`
	UserId           string        `json:"user_id" db:"user_id" msgpack:"user_id"`
	TemplateId       string        `json:"template_id" db:"template_id" msgpack:"template_id"`
	SharedWithUserId string        `json:"shared_with_user_id" db:"shared_with_user_id" msgpack:"shared_with_user_id"`
	Name             string        `json:"name" db:"name" msgpack:"name"`
	Description      string        `json:"description" db:"description" msgpack:"description"`
	Note             string        `json:"note" db:"note" msgpack:"note"`
	Location         string        `json:"location" db:"location" msgpack:"location"`
	Shell            string        `json:"shell" db:"shell" msgpack:"shell"`
	TemplateHash     string        `json:"template_hash" db:"template_hash" msgpack:"template_hash"`
	NomadNamespace   string        `json:"nomad_namespace" db:"nomad_namespace" msgpack:"nomad_namespace"`
	ContainerId      string        `json:"container_id" db:"container_id" msgpack:"container_id"`
	VolumeData       VolumeDataMap `json:"volume_data" db:"volume_data" msgpack:"volume_data"`
	SSHHostSigner    string        `json:"ssh_host_signer" db:"ssh_host_signer" msgpack:"ssh_host_signer"`
	IsDeployed       bool          `json:"is_deployed" db:"is_deployed" msgpack:"is_deployed"`
	IsPending        bool          `json:"is_pending" db:"is_pending" msgpack:"is_pending"`    // Flags if the space is pending a state change, starting or stopping
	IsDeleting       bool          `json:"is_deleting" db:"is_deleting" msgpack:"is_deleting"` // Flags if the space is pending a state change, starting or stopping
	IsDeleted        bool          `json:"is_deleted" db:"is_deleted" msgpack:"is_deleted"`
	AltNames         []string      `json:"alt_names" msgpack:"alt_names"`
	StartedAt        time.Time     `json:"started_at" db:"started_at" msgpack:"started_at"`
	CreatedAt        time.Time     `json:"created_at" db:"created_at" msgpack:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at" db:"updated_at" msgpack:"updated_at"`
}

func NewSpace(name string, description string, userId string, templateId string, shell string, altNames *[]string, location string) *Space {
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal().Msg(err.Error())
	}

	// Create a host key for the space
	ed25519, err := sshd.GenerateEd25519PrivateKey()
	if err != nil {
		log.Fatal().Msg(err.Error())
	}

	now := time.Now().UTC()

	space := &Space{
		Id:               id.String(),
		UserId:           userId,
		TemplateId:       templateId,
		Name:             name,
		Description:      description,
		AltNames:         *altNames,
		Shell:            shell,
		TemplateHash:     "",
		IsDeployed:       false,
		IsPending:        false,
		IsDeleting:       false,
		VolumeData:       make(map[string]SpaceVolume),
		StartedAt:        now,
		CreatedAt:        now,
		UpdatedAt:        now,
		Location:         location,
		SSHHostSigner:    ed25519,
		SharedWithUserId: "",
	}

	return space
}

func (s *Space) MaxUptimeReached(template *Template) bool {
	if template.MaxUptimeUnit == "disabled" {
		return false
	}

	if template.MaxUptime == 0 {
		return true
	}

	var maxUptime time.Duration
	switch template.MaxUptimeUnit {
	case "minute":
		maxUptime = time.Duration(template.MaxUptime) * time.Minute
	case "hour":
		maxUptime = time.Duration(template.MaxUptime) * time.Hour
	case "day":
		maxUptime = time.Duration(template.MaxUptime) * 24 * time.Hour
	default:
		maxUptime = time.Duration(template.MaxUptime) * time.Hour // fallback to hour
	}

	if s.StartedAt.Add(maxUptime).Before(time.Now().UTC()) {
		return true
	}

	return false
}
