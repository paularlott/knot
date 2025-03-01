package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/paularlott/knot/internal/sshd"
	"github.com/rs/zerolog/log"
)

type SpaceVolume struct {
	Id        string `json:"id"`
	Namespace string `json:"Namespace"`
}

// Space object
type Space struct {
	Id               string                 `json:"space_id"`
	UserId           string                 `json:"user_id"`
	TemplateId       string                 `json:"template_id"`
	Name             string                 `json:"name"`
	Shell            string                 `json:"shell"`
	Location         string                 `json:"location"`
	TemplateHash     string                 `json:"template_hash"`
	NomadNamespace   string                 `json:"nomad_namespace"`
	ContainerId      string                 `json:"container_id"`
	VolumeData       map[string]SpaceVolume `json:"volume_data"`
	IsDeployed       bool                   `json:"is_deployed"`
	IsPending        bool                   `json:"is_pending"` // Flags if the space is pending a state change, starting or stopping
	IsDeleting       bool                   `json:"is_deleting"`
	AltNames         []string               `json:"alt_names"`
	SSHHostSigner    string                 `json:"ssh_host_signer"`
	SharedWithUserId string                 `json:"shared_with_user_id"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
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
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
		Location:         "",
		SSHHostSigner:    ed25519,
		SharedWithUserId: "",
	}

	return space
}
