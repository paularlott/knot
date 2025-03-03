package model

import (
	"time"

	"github.com/paularlott/knot/internal/sshd"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type SpaceVolume struct {
	Id        string `json:"id"`
	Namespace string `json:"Namespace"`
}

// Space object
type Space struct {
	Id               string                 `json:"space_id" db:"space_id,pk"`
	ParentSpaceId    string                 `json:"parent_space_id" db:"parent_space_id"`
	UserId           string                 `json:"user_id" db:"user_id"`
	TemplateId       string                 `json:"template_id" db:"template_id"`
	SharedWithUserId string                 `json:"shared_with_user_id" db:"shared_with_user_id"`
	Name             string                 `json:"name" db:"name"`
	Location         string                 `json:"location" db:"location"`
	Shell            string                 `json:"shell" db:"shell"`
	TemplateHash     string                 `json:"template_hash" db:"template_hash"`
	NomadNamespace   string                 `json:"nomad_namespace" db:"nomad_namespace"`
	ContainerId      string                 `json:"container_id" db:"container_id"`
	VolumeData       map[string]SpaceVolume `json:"volume_data" db:"volume_data,json"`
	SSHHostSigner    string                 `json:"ssh_host_signer" db:"ssh_host_signer"`
	IsDeployed       bool                   `json:"is_deployed" db:"is_deployed"`
	IsPending        bool                   `json:"is_pending" db:"is_pending"` // Flags if the space is pending a state change, starting or stopping
	IsDeleting       bool                   `json:"is_deleting" db:"is_deleting"`
	AltNames         []string               `json:"alt_names"`
	CreatedAt        NullTime               `json:"created_at" db:"created_at"`
	UpdatedAt        NullTime               `json:"updated_at" db:"updated_at"`
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
		CreatedAt:        NullTime{Time: &now},
		UpdatedAt:        NullTime{Time: &now},
		Location:         "",
		SSHHostSigner:    ed25519,
		SharedWithUserId: "",
	}

	return space
}
