package model

import (
	"time"

	"github.com/google/uuid"
)

// Space object
type Space struct {
	Id string `json:"space_id"`
  UserId string `json:"user_id"`
  TemplateId string `json:"template_id"`
  Name string `json:"name"`
  AgentURL string `json:"agent_url"`
  Shell string `json:"shell"`
  CreatedAt time.Time `json:"created_at"`
  UpdatedAt time.Time `json:"updated_at"`
}

func NewSpace(name string, userId string, agentURL string, templateId string, shell string) *Space {
  space := &Space{
    Id: uuid.New().String(),
    UserId: userId,
    TemplateId: templateId,
    Name: name,
    AgentURL: agentURL,
    Shell: shell,
    CreatedAt: time.Now().UTC(),
    UpdatedAt: time.Now().UTC(),
  }

  return space
}
