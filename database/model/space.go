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
	Id               string        `json:"space_id" db:"space_id,pk"`
	ParentSpaceId    string        `json:"parent_space_id" db:"parent_space_id"`
	UserId           string        `json:"user_id" db:"user_id"`
	TemplateId       string        `json:"template_id" db:"template_id"`
	SharedWithUserId string        `json:"shared_with_user_id" db:"shared_with_user_id"`
	Name             string        `json:"name" db:"name"`
	Location         string        `json:"location" db:"location"`
	Shell            string        `json:"shell" db:"shell"`
	TemplateHash     string        `json:"template_hash" db:"template_hash"`
	NomadNamespace   string        `json:"nomad_namespace" db:"nomad_namespace"`
	ContainerId      string        `json:"container_id" db:"container_id"`
	VolumeData       VolumeDataMap `json:"volume_data" db:"volume_data"`
	SSHHostSigner    string        `json:"ssh_host_signer" db:"ssh_host_signer"`
	IsDeployed       bool          `json:"is_deployed" db:"is_deployed"`
	IsPending        bool          `json:"is_pending" db:"is_pending"` // Flags if the space is pending a state change, starting or stopping
	IsDeleting       bool          `json:"is_deleting" db:"is_deleting"`
	AltNames         []string      `json:"alt_names"`
	CreatedAt        time.Time     `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at" db:"updated_at"`
}

func NewSpace(name string, userId string, templateId string, shell string, altNames *[]string) *Space {
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
		AltNames:         *altNames,
		Shell:            shell,
		TemplateHash:     "",
		IsDeployed:       false,
		IsPending:        false,
		IsDeleting:       false,
		VolumeData:       make(map[string]SpaceVolume),
		CreatedAt:        now,
		UpdatedAt:        now,
		Location:         "",
		SSHHostSigner:    ed25519,
		SharedWithUserId: "",
	}

	return space
}
