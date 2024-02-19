package model

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type SpaceVolume struct {
  Id string `json:"id"`
  Namespace string `json:"Namespace"`
}

// Space object
type Space struct {
	Id string `json:"space_id"`
  UserId string `json:"user_id"`
  TemplateId string `json:"template_id"`
  Name string `json:"name"`
  AgentURL string `json:"agent_url"`
  Shell string `json:"shell"`
  TemplateHash string `json:"template_hash"`
  NomadNamespace string `json:"nomad_namespace"`
  NomadJobId string `json:"nomad_job_id"`
  VolumeData map[string]SpaceVolume `json:"volume_data"`
  VolumeSizes map[string]int64 `json:"volume_sizes"`
  IsDeployed bool `json:"is_deployed"`
  CreatedAt time.Time `json:"created_at"`
  UpdatedAt time.Time `json:"updated_at"`
}

func NewSpace(name string, userId string, agentURL string, templateId string, shell string, volSizes *map[string]int64) *Space {
  id, err := uuid.NewV7()
  if err != nil {
    log.Fatal().Msg(err.Error())
  }

  space := &Space{
    Id: id.String(),
    UserId: userId,
    TemplateId: templateId,
    Name: name,
    AgentURL: agentURL,
    Shell: shell,
    TemplateHash: "",
    IsDeployed: false,
    VolumeData: make(map[string]SpaceVolume),
    VolumeSizes: *volSizes,
    CreatedAt: time.Now().UTC(),
    UpdatedAt: time.Now().UTC(),
  }

  return space
}

func (space *Space) GetAgentURL() string {
  if space.AgentURL == "" {
    if viper.GetBool("server.tls.agent_use_tls") {
      return fmt.Sprintf("srv+https://knot-%s.service.consul", space.Id)
    } else {
      return fmt.Sprintf("srv+http://knot-%s.service.consul", space.Id)
    }
  } else {
    return space.AgentURL
  }
}
